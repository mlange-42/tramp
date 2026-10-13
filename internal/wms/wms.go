// Package wms implements a minimal client for fetching map images from WMS and XYZ tile services.
package wms

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	// Register decoders for the image formats WMS servers commonly deliver.
	_ "image/jpeg"
	_ "image/png"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/httpx"
)

// Layer types.
const (
	// TypeWMS is a WMS service. It is the default for an empty [Layer.Type].
	TypeWMS = "wms"
	// TypeXYZ is a tile service with a URL template containing {z}, {x} and {y}.
	TypeXYZ = "xyz"
	// TypeNone is an empty layer that is not fetched, e.g. for showing only overlays.
	TypeNone = "none"
)

// Layer describes a map layer to request.
type Layer struct {
	// Name is a human-readable name for the layer.
	Name string `yaml:"name"`
	// Description is shown as a tooltip in the list of layers. May be empty.
	Description string `yaml:"description,omitempty"`
	// Type is [TypeWMS] (the default if empty), [TypeXYZ] or [TypeNone].
	Type string `yaml:"type,omitempty"`
	// URL is the service endpoint, without request parameters.
	// For XYZ layers, it is a template like "https://tile.example.com/{z}/{x}/{y}.png".
	URL string `yaml:"url,omitempty"`
	// Layers is the comma-separated list of WMS layer names. Not used for XYZ layers.
	Layers string `yaml:"layers,omitempty"`
	// Styles is the comma-separated list of styles. May be empty.
	Styles string `yaml:"styles,omitempty"`
	// Format is the image MIME type, e.g. "image/png". Not used for XYZ layers.
	Format string `yaml:"format,omitempty"`
	// Version is the WMS version, "1.3.0" or "1.1.1". Not used for XYZ layers.
	Version string `yaml:"version,omitempty"`
	// Enabled can turn the layer off without removing it from the configuration file. Nil means enabled.
	Enabled *bool `yaml:"enabled,omitempty"`
	// MetaTiles is the number of tiles per side requested in one image from a WMS, like 4 for 4×4 tiles.
	// Larger images are much faster for services that need long to render each image. Zero or one means single tiles.
	MetaTiles int `yaml:"meta_tiles,omitempty"`
	// MaxZoom is the highest tile level the service provides. Zero means no limit.
	// The map is magnified beyond that.
	MaxZoom int `yaml:"max_zoom,omitempty"`
	// Capabilities is the address of WMS capabilities that list the layer, if not the service URL.
	// Some servers offer smaller documents for single layers, which load faster.
	// The capabilities are read for the valid times of overlays that change over time.
	Capabilities string `yaml:"capabilities,omitempty"`
	// Time is the TIME parameter of requests, for layers that change over time.
	// It is set by the application, so it is not part of the configuration file.
	Time string `yaml:"-"`
	// Reference is the reference time of the data, like the start of a model run, requested as DIM_REFERENCE_TIME.
	// Empty for the server's default, usually the latest. It is set by the application.
	Reference string `yaml:"-"`
	// Transparent requests a transparent background, for overlays.
	// It is set automatically for overlays, so it is not part of the configuration file.
	Transparent bool `yaml:"-"`
	// Attribution is the full credit of the layer, listed with the data sources.
	Attribution string `yaml:"attribution,omitempty"`
	// AttributionShort is the short credit shown on the map, with parts separated by " · ", like
	// "© OpenStreetMap · CyclOSM". Parts shared with other layers are shown once. If empty, Attribution is shown.
	AttributionShort string `yaml:"attribution_short,omitempty"`
	// AttributionURL links to the terms or the credits of the data, for the list of data sources.
	AttributionURL string `yaml:"attribution_url,omitempty"`
}

// IsXYZ reports whether the layer is an XYZ tile service.
func (l *Layer) IsXYZ() bool {
	return strings.EqualFold(l.Type, TypeXYZ)
}

// IsEnabled reports whether the layer is enabled, see [Layer.Enabled].
func (l *Layer) IsEnabled() bool {
	return l.Enabled == nil || *l.Enabled
}

// MetaSize returns the number of tiles per side requested in one image at a tile level,
// see [Layer.MetaTiles]. It is 1 for XYZ layers, and at most the number of tiles of the level.
func (l *Layer) MetaSize(z int) int {
	if l.IsXYZ() || l.MetaTiles <= 1 {
		return 1
	}
	return min(l.MetaTiles, geo.TileCount(z))
}

// MetaTile returns the first tile of the meta tile containing key, and its size in tiles per side.
func (l *Layer) MetaTile(key geo.TileKey) (geo.TileKey, int) {
	n := l.MetaSize(key.Z)
	return geo.TileKey{Z: key.Z, X: key.X / n * n, Y: key.Y / n * n}, n
}

// MetaURL returns the request URL of the n×n tiles starting at the given tile, see [Layer.MetaTile].
func (l *Layer) MetaURL(first geo.TileKey, n int) string {
	if n <= 1 {
		return l.TileURL(first)
	}
	last := geo.TileKey{Z: first.Z, X: first.X + n - 1, Y: first.Y + n - 1}
	return l.MapURL(first.Bounds().Union(last.Bounds()), n*geo.TileSize, n*geo.TileSize)
}

// IsNone reports whether the layer is empty and has nothing to fetch.
func (l *Layer) IsNone() bool {
	return strings.EqualFold(l.Type, TypeNone)
}

// TileURL returns the request URL for a tile.
func (l *Layer) TileURL(key geo.TileKey) string {
	if !l.IsXYZ() {
		return l.MapURL(key.Bounds(), geo.TileSize, geo.TileSize)
	}
	return strings.NewReplacer(
		"{z}", strconv.Itoa(key.Z),
		"{x}", strconv.Itoa(key.X),
		"{y}", strconv.Itoa(key.Y),
	).Replace(l.URL)
}

// MapURL returns the GetMap request URL for the given Web Mercator extent and pixel size.
func (l *Layer) MapURL(bbox geo.Rect, width, height int) string {
	version := l.Version
	if version == "" {
		version = "1.3.0"
	}
	format := l.Format
	if format == "" {
		format = "image/png"
	}

	q := url.Values{}
	q.Set("SERVICE", "WMS")
	q.Set("VERSION", version)
	q.Set("REQUEST", "GetMap")
	q.Set("LAYERS", l.Layers)
	q.Set("STYLES", l.Styles)
	q.Set("FORMAT", format)
	q.Set("WIDTH", strconv.Itoa(width))
	q.Set("HEIGHT", strconv.Itoa(height))
	if l.Transparent {
		q.Set("TRANSPARENT", "TRUE")
	}
	if l.Time != "" {
		q.Set("TIME", l.Time)
	}
	if l.Reference != "" {
		q.Set("DIM_REFERENCE_TIME", l.Reference)
	}
	// EPSG:3857 has easting/northing axis order in both versions.
	if version == "1.1.1" {
		q.Set("SRS", "EPSG:3857")
	} else {
		q.Set("CRS", "EPSG:3857")
	}
	q.Set("BBOX", fmt.Sprintf("%f,%f,%f,%f", bbox.Min.X, bbox.Min.Y, bbox.Max.X, bbox.Max.Y))

	sep := "?"
	if strings.Contains(l.URL, "?") {
		sep = "&"
	}
	return l.URL + sep + q.Encode()
}

// Client fetches map images from WMS and tile servers.
type Client struct {
	httpx.Client
}

// GetMap fetches and decodes the map image for the given layer and extent.
func (c *Client) GetMap(ctx context.Context, layer *Layer, bbox geo.Rect, width, height int) (image.Image, error) {
	d, err := c.fetch(ctx, layer.MapURL(bbox, width, height), time.Time{})
	if err != nil {
		return nil, err
	}
	return Decode(d.Body)
}

// GetTile fetches and decodes a tile image of the given layer.
func (c *Client) GetTile(ctx context.Context, layer *Layer, key geo.TileKey) (image.Image, error) {
	d, err := c.GetTileData(ctx, layer, key, time.Time{})
	if err != nil {
		return nil, err
	}
	return Decode(d.Body)
}

// ErrServiceException is returned if the server reports an error instead of an image,
// like for a time without data.
var ErrServiceException = errors.New("wms: service exception")

// exceptionText returns the message of a service exception report, or the whole body if it has none.
func exceptionText(body []byte) []byte {
	s := string(body)
	// Not the enclosing ServiceExceptionReport.
	start := strings.Index(s, "<ServiceException>")
	if start < 0 {
		start = strings.Index(s, "<ServiceException ")
	}
	if start < 0 {
		return body
	}
	s = s[start:]
	if i := strings.IndexByte(s, '>'); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.Index(s, "</ServiceException>"); i >= 0 {
		s = s[:i]
	}
	return []byte(strings.Join(strings.Fields(s), " "))
}

// ErrNotModified is returned by [Client.GetTileData] if the tile has not changed since the given time.
var ErrNotModified = errors.New("wms: not modified")

// Data is an image as delivered by the server, not decoded.
type Data struct {
	Body []byte
	// NoStore is whether the server forbids storing the image.
	NoStore bool
}

// GetTileData fetches a tile image of the given layer without decoding it.
// If since is not zero, the server is asked whether the tile changed since then,
// and [ErrNotModified] is returned if not.
func (c *Client) GetTileData(ctx context.Context, layer *Layer, key geo.TileKey, since time.Time) (Data, error) {
	return c.fetch(ctx, layer.TileURL(key), since)
}

// GetData fetches an image from a request URL without decoding it, like [Client.GetTileData].
func (c *Client) GetData(ctx context.Context, u string, since time.Time) (Data, error) {
	return c.fetch(ctx, u, since)
}

// Decode decodes an image as delivered by the server.
func Decode(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("wms: decoding image: %w", err)
	}
	return img, nil
}

func (c *Client) fetch(ctx context.Context, u string, since time.Time) (Data, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Data{}, err
	}
	if !since.IsZero() {
		req.Header.Set("If-Modified-Since", since.UTC().Format(http.TimeFormat))
	}
	resp, err := c.Do(req)
	if err != nil {
		return Data{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotModified && !since.IsZero() {
		return Data{}, ErrNotModified
	}

	// Limit reads in case a misconfigured server sends something huge.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return Data{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Data{}, fmt.Errorf("wms: HTTP %d: %s", resp.StatusCode, excerpt(body))
	}
	// Servers report errors as XML service exceptions, often with status 200.
	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "xml") || strings.HasPrefix(ct, "text/") {
		return Data{}, fmt.Errorf("%w: %s", ErrServiceException, excerpt(exceptionText(body)))
	}
	noStore := strings.Contains(strings.ToLower(resp.Header.Get("Cache-Control")), "no-store")
	return Data{Body: body, NoStore: noStore}, nil
}

func excerpt(b []byte) string {
	const maxLen = 300
	s := strings.TrimSpace(string(b))
	if len(s) > maxLen {
		s = s[:maxLen] + "..."
	}
	return s
}

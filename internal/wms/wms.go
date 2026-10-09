// Package wms implements a minimal client for fetching map images from WMS and XYZ tile services.
package wms

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	// Register decoders for the image formats WMS servers commonly deliver.
	_ "image/jpeg"
	_ "image/png"

	"github.com/mlange-42/tramp/internal/geo"
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
	// MaxZoom is the highest tile level the service provides. Zero means no limit.
	// The map is magnified beyond that.
	MaxZoom int `yaml:"max_zoom,omitempty"`
	// Transparent requests a transparent background, for overlays.
	// It is set automatically for overlays, so it is not part of the configuration file.
	Transparent bool `yaml:"-"`
	// Attribution is shown on the map.
	Attribution string `yaml:"attribution,omitempty"`
}

// IsXYZ reports whether the layer is an XYZ tile service.
func (l *Layer) IsXYZ() bool {
	return strings.EqualFold(l.Type, TypeXYZ)
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
	HTTP      *http.Client
	UserAgent string
}

// GetMap fetches and decodes the map image for the given layer and extent.
func (c *Client) GetMap(ctx context.Context, layer *Layer, bbox geo.Rect, width, height int) (image.Image, error) {
	return c.get(ctx, layer.MapURL(bbox, width, height))
}

// GetTile fetches and decodes a tile image of the given layer.
func (c *Client) GetTile(ctx context.Context, layer *Layer, key geo.TileKey) (image.Image, error) {
	return c.get(ctx, layer.TileURL(key))
}

func (c *Client) get(ctx context.Context, u string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Limit reads in case a misconfigured server sends something huge.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wms: HTTP %d: %s", resp.StatusCode, excerpt(body))
	}
	// Servers report errors as XML service exceptions, often with status 200.
	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "xml") || strings.HasPrefix(ct, "text/") {
		return nil, fmt.Errorf("wms: service exception: %s", excerpt(body))
	}

	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("wms: decoding image: %w", err)
	}
	return img, nil
}

func excerpt(b []byte) string {
	const maxLen = 300
	s := strings.TrimSpace(string(b))
	if len(s) > maxLen {
		s = s[:maxLen] + "..."
	}
	return s
}

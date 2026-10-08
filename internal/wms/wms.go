// Package wms implements a minimal WMS client for fetching map images.
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

// Layer describes a WMS layer to request.
type Layer struct {
	// Name is a human-readable name for the layer.
	Name string
	// URL is the service endpoint, without request parameters.
	URL string
	// Layers is the comma-separated list of WMS layer names.
	Layers string
	// Styles is the comma-separated list of styles. May be empty.
	Styles string
	// Format is the image MIME type, e.g. "image/png".
	Format string
	// Version is the WMS version, "1.3.0" or "1.1.1".
	Version string
	// Transparent requests a transparent background, for overlays.
	Transparent bool
	// Attribution is shown on the map.
	Attribution string
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

// Client fetches map images from WMS servers.
type Client struct {
	HTTP      *http.Client
	UserAgent string
}

// GetMap fetches and decodes the map image for the given layer and extent.
func (c *Client) GetMap(ctx context.Context, layer *Layer, bbox geo.Rect, width, height int) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, layer.MapURL(bbox, width, height), nil)
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

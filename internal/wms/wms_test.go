package wms

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
)

func TestMapURL(t *testing.T) {
	l := Layer{URL: "https://example.com/wms?map=x", Layers: "a,b", Version: "1.3.0"}
	u, err := url.Parse(l.MapURL(geo.Rect{Min: geo.Point{X: 1, Y: 2}, Max: geo.Point{X: 3, Y: 4}}, 256, 128))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	checks := map[string]string{
		"map": "x", "REQUEST": "GetMap", "LAYERS": "a,b", "CRS": "EPSG:3857",
		"WIDTH": "256", "HEIGHT": "128", "FORMAT": "image/png",
		"BBOX": "1.000000,2.000000,3.000000,4.000000",
	}
	for k, v := range checks {
		if got := q.Get(k); got != v {
			t.Errorf("%s: expected %q, got %q", k, v, got)
		}
	}
	if q.Has("TRANSPARENT") {
		t.Error("TRANSPARENT should only be set for transparent layers")
	}

	l.Version = "1.1.1"
	l.Transparent = true
	u, _ = url.Parse(l.MapURL(geo.Rect{}, 1, 1))
	if u.Query().Get("SRS") != "EPSG:3857" || u.Query().Has("CRS") {
		t.Errorf("WMS 1.1.1 should use SRS, got %s", u.RawQuery)
	}
	if u.Query().Get("TRANSPARENT") != "TRUE" {
		t.Errorf("expected TRANSPARENT=TRUE, got %s", u.RawQuery)
	}
}

func TestTileURL(t *testing.T) {
	l := Layer{Type: TypeXYZ, URL: "https://example.com/{z}/{x}/{y}.png"}
	if got := l.TileURL(geo.TileKey{Z: 3, X: 4, Y: 5}); got != "https://example.com/3/4/5.png" {
		t.Errorf("unexpected XYZ URL %q", got)
	}

	l = Layer{URL: "https://example.com/wms", Layers: "a"}
	u, err := url.Parse(l.TileURL(geo.TileKey{Z: 0}))
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("REQUEST") != "GetMap" || u.Query().Get("WIDTH") != "256" {
		t.Errorf("expected a WMS GetMap request, got %s", u.RawQuery)
	}
}

func TestGetMap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("LAYERS") == "bad" {
			w.Header().Set("Content-Type", "application/vnd.ogc.se_xml")
			_, _ = w.Write([]byte("<ServiceExceptionReport>Layer not found</ServiceExceptionReport>"))
			return
		}
		img := image.NewRGBA(image.Rect(0, 0, 4, 4))
		img.Set(1, 1, color.RGBA{255, 0, 0, 255})
		w.Header().Set("Content-Type", "image/png")
		_ = png.Encode(w, img)
	}))
	defer srv.Close()

	c := Client{}
	img, err := c.GetMap(context.Background(), &Layer{URL: srv.URL, Layers: "good"}, geo.Rect{}, 4, 4)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 4 {
		t.Errorf("unexpected image size %v", img.Bounds())
	}

	_, err = c.GetMap(context.Background(), &Layer{URL: srv.URL, Layers: "bad"}, geo.Rect{}, 4, 4)
	if err == nil || !strings.Contains(err.Error(), "Layer not found") {
		t.Errorf("expected service exception, got %v", err)
	}
}

func TestGetTileData(t *testing.T) {
	modified := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if since, err := http.ParseTime(r.Header.Get("If-Modified-Since")); err == nil && !modified.After(since) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if r.URL.Path == "/1/0/0.png" {
			w.Header().Set("Cache-Control", "private, no-store")
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png"))
	}))
	defer srv.Close()

	c := Client{}
	l := &Layer{Type: TypeXYZ, URL: srv.URL + "/{z}/{x}/{y}.png"}
	d, err := c.GetTileData(context.Background(), l, geo.TileKey{}, time.Time{})
	if err != nil || string(d.Body) != "png" || d.NoStore {
		t.Errorf("got %+v, %v", d, err)
	}
	d, err = c.GetTileData(context.Background(), l, geo.TileKey{}, modified.Add(-time.Hour))
	if err != nil || string(d.Body) != "png" {
		t.Errorf("got %+v, %v for a modified tile", d, err)
	}
	if _, err := c.GetTileData(context.Background(), l, geo.TileKey{}, modified); !errors.Is(err, ErrNotModified) {
		t.Errorf("got %v for an unmodified tile", err)
	}
	if d, err := c.GetTileData(context.Background(), l, geo.TileKey{Z: 1}, time.Time{}); err != nil || !d.NoStore {
		t.Errorf("got %+v, %v for a tile not to be stored", d, err)
	}
}

func TestMetaTiles(t *testing.T) {
	l := &Layer{URL: "https://example.com/wms", Layers: "a", MetaTiles: 4}
	first, n := l.MetaTile(geo.TileKey{Z: 5, X: 13, Y: 6})
	if n != 4 || first != (geo.TileKey{Z: 5, X: 12, Y: 4}) {
		t.Errorf("unexpected meta tile %v %d", first, n)
	}
	// Not larger than the level.
	if first, n := l.MetaTile(geo.TileKey{Z: 1, X: 1, Y: 1}); n != 2 || first != (geo.TileKey{Z: 1}) {
		t.Errorf("unexpected meta tile at level 1: %v %d", first, n)
	}
	u, err := url.Parse(l.MetaURL(geo.TileKey{Z: 1}, 2))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	world := geo.TileKey{}.Bounds()
	want := fmt.Sprintf("%f,%f,%f,%f", world.Min.X, world.Min.Y, world.Max.X, world.Max.Y)
	if q.Get("WIDTH") != "512" || q.Get("HEIGHT") != "512" || q.Get("BBOX") != want {
		t.Errorf("unexpected request %v", q)
	}
	// Single tiles for XYZ layers and without meta tiles.
	if l.MetaURL(geo.TileKey{Z: 3}, 1) != l.TileURL(geo.TileKey{Z: 3}) {
		t.Error("single tile should use the tile URL")
	}
	for _, other := range []*Layer{{Type: TypeXYZ, URL: "https://x/{z}/{x}/{y}.png", MetaTiles: 4}, {URL: "https://x"}} {
		if _, n := other.MetaTile(geo.TileKey{Z: 5, X: 13, Y: 6}); n != 1 {
			t.Errorf("expected single tiles for %+v", other)
		}
	}
}

func TestLayerEnabled(t *testing.T) {
	off, on := false, true
	if !(&Layer{}).IsEnabled() || !(&Layer{Enabled: &on}).IsEnabled() || (&Layer{Enabled: &off}).IsEnabled() {
		t.Error("unexpected enabled state")
	}
}

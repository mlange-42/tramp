package tiles

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/diskcache"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/wms"
)

func TestShade(t *testing.T) {
	src := image.NewRGBA(image.Rect(10, 10, 13, 11))
	src.Set(10, 10, color.RGBA{255, 255, 255, 255})
	src.Set(11, 10, color.RGBA{0, 0, 0, 255})
	src.Set(12, 10, color.RGBA{128, 128, 128, 255})

	dst := Shade(src)
	if dst.Bounds() != image.Rect(0, 0, 3, 1) {
		t.Fatalf("unexpected bounds %v", dst.Bounds())
	}
	expected := []uint8{0, 255, 127}
	for x, a := range expected {
		c := dst.RGBAAt(x, 0)
		if c.R != 0 || c.G != 0 || c.B != 0 || c.A != a {
			t.Errorf("pixel %d: expected black with alpha %d, got %v", x, a, c)
		}
	}
}

// pngServer serves a PNG image for all tiles, answers If-Modified-Since with 304,
// and fails while down is set. With noStore, it forbids storing the images.
type pngServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests int
	checks   int
	down     bool
	noStore  bool
}

func newPNGServer(t *testing.T) *pngServer {
	s := &pngServer{}
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.down {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		s.requests++
		if r.Header.Get("If-Modified-Since") != "" {
			s.checks++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if s.noStore {
			w.Header().Set("Cache-Control", "no-store")
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(buf.Bytes())
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *pngServer) counts() (requests, checks int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests, s.checks
}

func TestWMSFetcherCache(t *testing.T) {
	srv := newPNGServer(t)
	cache := diskcache.New(t.TempDir(), 1<<20)
	cache.Wait()
	f := &WMSFetcher{
		Client: &wms.Client{},
		Layer:  &wms.Layer{Name: "Test map", Type: wms.TypeXYZ, URL: srv.URL + "/{z}/{x}/{y}.png"},
		Cache:  cache,
		MaxAge: time.Hour,
	}
	key := geo.TileKey{Z: 3, X: 2, Y: 1}
	ctx := context.Background()

	// Downloaded and stored.
	if _, err := f.Fetch(ctx, key); err != nil {
		t.Fatal(err)
	}
	ck := f.dir + "/3/2/1.png"
	if _, _, ok := cache.Get(ck); !ok {
		t.Fatalf("tile not stored as %s", ck)
	}
	if _, _, ok := cache.Get(f.dir + "/" + LayerInfo); !ok {
		t.Error("layer description not stored")
	}

	// From the cache.
	if _, err := f.Fetch(ctx, key); err != nil {
		t.Fatal(err)
	}
	if r, _ := srv.counts(); r != 1 {
		t.Errorf("%d requests, want 1", r)
	}

	// Too old: checked with the server, and touched.
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(cache.Dir(), filepath.FromSlash(ck)), old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Fetch(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, c := srv.counts(); c != 1 {
		t.Errorf("%d checks, want 1", c)
	}
	if _, mod, _ := cache.Get(ck); time.Since(mod) > time.Minute {
		t.Error("tile not touched")
	}

	// Too old and the server is down: the old tile is used.
	if err := os.Chtimes(filepath.Join(cache.Dir(), filepath.FromSlash(ck)), old, old); err != nil {
		t.Fatal(err)
	}
	srv.mu.Lock()
	srv.down = true
	srv.mu.Unlock()
	if _, err := f.Fetch(ctx, key); err != nil {
		t.Errorf("old tile not used: %v", err)
	}
	// Not cached and the server is down: fails.
	if _, err := f.Fetch(ctx, geo.TileKey{Z: 3}); err == nil {
		t.Error("no error for a tile neither cached nor available")
	}
}

func TestImageExt(t *testing.T) {
	tests := []struct {
		layer wms.Layer
		want  string
	}{
		{wms.Layer{Type: wms.TypeXYZ, URL: "https://a/{z}/{x}/{y}.png"}, ".png"},
		{wms.Layer{Type: wms.TypeXYZ, URL: "https://a/{z}/{x}/{y}.JPG?key=1"}, ".jpg"},
		{wms.Layer{Type: wms.TypeXYZ, URL: "https://a/{z}/{x}/{y}"}, ""},
		{wms.Layer{Type: wms.TypeXYZ, URL: "https://a/tile.php?z={z}&x={x}&y={y}"}, ""},
		{wms.Layer{URL: "https://a/wms"}, ".png"},
		{wms.Layer{URL: "https://a/wms", Format: "image/jpeg"}, ".jpg"},
		{wms.Layer{URL: "https://a/wms", Format: "image/gif"}, ".gif"},
		{wms.Layer{URL: "https://a/wms", Format: "application/x-unknown"}, ""},
	}
	for _, tt := range tests {
		if got := imageExt(&tt.layer); got != tt.want {
			t.Errorf("imageExt(%q, %q) = %q, want %q", tt.layer.URL, tt.layer.Format, got, tt.want)
		}
	}
}

func TestWMSFetcherNoStore(t *testing.T) {
	srv := newPNGServer(t)
	srv.noStore = true
	cache := diskcache.New(t.TempDir(), 1<<20)
	cache.Wait()
	f := &WMSFetcher{
		Client: &wms.Client{},
		Layer:  &wms.Layer{Name: "Test", Type: wms.TypeXYZ, URL: srv.URL + "/{z}/{x}/{y}.png"},
		Shade:  true,
		Cache:  cache,
		MaxAge: time.Hour,
	}
	img, err := f.Fetch(context.Background(), geo.TileKey{Z: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := img.(*image.RGBA); !ok {
		t.Errorf("expected a shaded image, got %T", img)
	}
	if _, _, ok := cache.Get(f.dir + "/1/0/0.png"); ok {
		t.Error("tile stored although the server forbids it")
	}
}

func TestWMSFetcherWithoutCache(t *testing.T) {
	srv := newPNGServer(t)
	f := &WMSFetcher{Client: &wms.Client{}, Layer: &wms.Layer{Type: wms.TypeXYZ, URL: srv.URL + "/{z}/{x}/{y}.png"}}
	for range 2 {
		if _, err := f.Fetch(context.Background(), geo.TileKey{}); err != nil {
			t.Fatal(err)
		}
	}
	if r, _ := srv.counts(); r != 2 {
		t.Errorf("%d requests, want 2", r)
	}
}

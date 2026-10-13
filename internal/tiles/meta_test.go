package tiles

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/mlange-42/tramp/internal/diskcache"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/wms"
)

// quadrantColor is the color of a quadrant of the test meta tile.
func quadrantColor(col, row int) color.NRGBA {
	return color.NRGBA{R: uint8(100 * col), G: uint8(100 * row), B: 50, A: 255}
}

// newMetaServer serves WMS images of the requested size, with a different color in each quadrant.
func newMetaServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		size, _ := strconv.Atoi(r.URL.Query().Get("WIDTH"))
		img := image.NewNRGBA(image.Rect(0, 0, size, size))
		for y := range size {
			for x := range size {
				img.SetNRGBA(x, y, quadrantColor(2*x/size, 2*y/size))
			}
		}
		var buf bytes.Buffer
		_ = png.Encode(&buf, img)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func TestWMSFetcherMetaTiles(t *testing.T) {
	srv, requests := newMetaServer(t)
	f := &WMSFetcher{Client: &wms.Client{}, Layer: &wms.Layer{Name: "Meta", URL: srv.URL, Layers: "a", MetaTiles: 2}}

	// The four tiles of a meta tile share one download, also when requested concurrently.
	var wg sync.WaitGroup
	imgs := make([]image.Image, 4)
	for i := range 4 {
		wg.Go(func() {
			key := geo.TileKey{Z: 3, X: 4 + i%2, Y: 2 + i/2}
			img, err := f.Fetch(context.Background(), key)
			if err != nil {
				t.Error(err)
			}
			imgs[i] = img
		})
	}
	wg.Wait()
	if n := requests.Load(); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
	for i, img := range imgs {
		if img == nil {
			continue
		}
		if img.Bounds() != image.Rect(0, 0, geo.TileSize, geo.TileSize) {
			t.Errorf("tile %d has bounds %v", i, img.Bounds())
		}
		if got, want := color.NRGBAModel.Convert(img.At(10, 10)), quadrantColor(i%2, i/2); got != want {
			t.Errorf("tile %d has color %v, want %v", i, got, want)
		}
	}

	// Kept meta tiles are downloaded again after a refresh.
	if _, err := f.Fetch(context.Background(), geo.TileKey{Z: 3, X: 4, Y: 2}); err != nil || requests.Load() != 1 {
		t.Errorf("expected the kept meta tile, %d requests, %v", requests.Load(), err)
	}
	f.Refresh()
	if _, err := f.Fetch(context.Background(), geo.TileKey{Z: 3, X: 4, Y: 2}); err != nil || requests.Load() != 2 {
		t.Errorf("expected a new download, %d requests, %v", requests.Load(), err)
	}
}

func TestWMSFetcherMetaTilesCache(t *testing.T) {
	srv, requests := newMetaServer(t)
	cache := diskcache.New(t.TempDir(), 1<<24)
	cache.Wait()
	layer := &wms.Layer{Name: "Meta", URL: srv.URL, Layers: "a", MetaTiles: 4}
	f := &WMSFetcher{Client: &wms.Client{}, Layer: layer, Cache: cache, MaxAge: 1 << 62}
	if _, err := f.Fetch(context.Background(), geo.TileKey{Z: 4, X: 5, Y: 6}); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := cache.Get(f.dir + "/4/4/4-m4.png"); !ok {
		t.Error("meta tile not stored by its first tile")
	}
	// Another fetcher reads the meta tile from the disk cache.
	g := &WMSFetcher{Client: &wms.Client{}, Layer: layer, Cache: cache, MaxAge: 1 << 62}
	if _, err := g.Fetch(context.Background(), geo.TileKey{Z: 4, X: 7, Y: 7}); err != nil || requests.Load() != 1 {
		t.Errorf("expected the stored meta tile, %d requests, %v", requests.Load(), err)
	}
}

func TestMetaCacheKeepsRecent(t *testing.T) {
	var c metaCache
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	loads := 0
	load := func() (image.Image, error) { loads++; return img, nil }
	for i := range metaKeep + 1 {
		if _, err := c.get(context.Background(), strconv.Itoa(i), load); err != nil {
			t.Fatal(err)
		}
	}
	// The oldest was dropped, the newest is kept.
	_, _ = c.get(context.Background(), strconv.Itoa(metaKeep), load)
	_, _ = c.get(context.Background(), "0", load)
	if loads != metaKeep+2 {
		t.Errorf("%d loads, want %d", loads, metaKeep+2)
	}
	// Failures are not kept.
	fail := func() (image.Image, error) { loads++; return nil, errors.New("failed") }
	for range 2 {
		if _, err := c.get(context.Background(), "bad", fail); err == nil {
			t.Error("expected an error")
		}
	}
	if loads != metaKeep+4 {
		t.Errorf("failed download should be tried again, %d loads", loads)
	}
}

func TestManagerRefreshesFetcher(t *testing.T) {
	srv, _ := newMetaServer(t)
	f := &WMSFetcher{Client: &wms.Client{}, Layer: &wms.Layer{URL: srv.URL, Layers: "a", MetaTiles: 2}}
	m := NewManager(f, Options{Workers: 1})
	defer m.Close()
	if _, err := f.Fetch(context.Background(), geo.TileKey{Z: 2}); err != nil {
		t.Fatal(err)
	}
	m.Refresh()
	f.metas.mu.Lock()
	n := len(f.metas.recent)
	f.metas.mu.Unlock()
	if n != 0 {
		t.Error("manager refresh should drop the fetcher's meta tiles")
	}
}

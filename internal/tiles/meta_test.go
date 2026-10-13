package tiles

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/mlange-42/tramp/internal/diskcache"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/wms"
)

// quadrantColor is the color of a quadrant of the test meta tile.
func quadrantColor(col, row int) color.NRGBA {
	return color.NRGBA{R: uint8(100 * col), G: uint8(100 * row), B: 50, A: 255}
}

// metaServer serves WMS images of the requested size, or of size if not zero,
// with a different color in each quadrant, and counts the requests by extent.
type metaServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests map[string]int
}

func newMetaServer(t *testing.T, size int) *metaServer {
	s := &metaServer{requests: map[string]int{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests[r.URL.Query().Get("BBOX")]++
		s.mu.Unlock()
		size := size
		if size == 0 {
			size, _ = strconv.Atoi(r.URL.Query().Get("WIDTH"))
		}
		img := image.NewNRGBA(image.Rect(0, 0, size, size))
		for row := range 2 {
			for col := range 2 {
				r := image.Rect(col*size/2, row*size/2, (col+1)*size/2, (row+1)*size/2)
				draw.Draw(img, r, image.NewUniform(quadrantColor(col, row)), image.Point{}, draw.Src)
			}
		}
		var buf bytes.Buffer
		_ = png.Encode(&buf, img)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(buf.Bytes())
	}))
	t.Cleanup(s.Close)
	return s
}

// counts returns the number of requests, and of different extents requested.
func (s *metaServer) counts() (requests, extents int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range s.requests {
		requests += n
	}
	return requests, len(s.requests)
}

func TestWMSFetcherGroups(t *testing.T) {
	srv := newMetaServer(t, 0)
	f := &WMSFetcher{Client: &wms.Client{}, Layer: &wms.Layer{Name: "Meta", URL: srv.URL, Layers: "a", MetaTiles: 2}}
	first, n := f.Group(geo.TileKey{Z: 3, X: 5, Y: 3})
	if n != 2 || first != (geo.TileKey{Z: 3, X: 4, Y: 2}) {
		t.Fatalf("unexpected group %v %d", first, n)
	}
	imgs, err := f.FetchGroup(context.Background(), first, n)
	if err != nil || len(imgs) != 4 {
		t.Fatalf("got %d images, %v", len(imgs), err)
	}
	for i, img := range imgs {
		if img.Bounds() != image.Rect(0, 0, geo.TileSize, geo.TileSize) {
			t.Errorf("tile %d has bounds %v", i, img.Bounds())
		}
		if got, want := color.NRGBAModel.Convert(img.At(10, 10)), quadrantColor(i%2, i/2); got != want {
			t.Errorf("tile %d has color %v, want %v", i, got, want)
		}
	}
	// A single tile of a meta tile.
	img, err := f.Fetch(context.Background(), geo.TileKey{Z: 3, X: 5, Y: 2})
	if err != nil || color.NRGBAModel.Convert(img.At(10, 10)) != quadrantColor(1, 0) {
		t.Errorf("unexpected single tile, %v", err)
	}
	// Without meta tiles, single tiles.
	single := &WMSFetcher{Client: &wms.Client{}, Layer: &wms.Layer{URL: srv.URL, Layers: "a"}}
	if _, n := single.Group(geo.TileKey{Z: 3, X: 5, Y: 3}); n != 1 {
		t.Errorf("expected single tiles, got %d", n)
	}
}

func TestManagerLoadsGroupsOnce(t *testing.T) {
	// Small images, as the size doesn't matter here.
	srv := newMetaServer(t, 8)
	f := &WMSFetcher{Client: &wms.Client{}, Layer: &wms.Layer{URL: srv.URL, Layers: "a", MetaTiles: 4}}
	m := NewManager(f, Options{})
	defer m.Close()

	// A large view of 16×9 tiles, not aligned to the meta tiles, requested every frame like by the map.
	var keys []geo.TileKey
	for y := 41; y < 50; y++ {
		for x := 69; x < 85; x++ {
			keys = append(keys, geo.TileKey{Z: 8, X: x, Y: y})
		}
	}
	m.Request(keys)
	// The queue holds the meta tiles: 5×3 of them, starting at 68 and 40.
	if n := m.Loading(); n != 15 {
		t.Errorf("%d downloads, want 15", n)
	}
	waitFor(t, func() bool { m.Request(keys); return m.Loading() == 0 })
	if requests, extents := srv.counts(); requests != 15 || extents != 15 {
		t.Errorf("%d requests of %d meta tiles, want 15 each", requests, extents)
	}
	// All tiles of the meta tiles are added, also those outside the view.
	for _, k := range []geo.TileKey{{Z: 8, X: 69, Y: 41}, {Z: 8, X: 68, Y: 40}, {Z: 8, X: 87, Y: 51}} {
		if _, ok := m.Get(k); !ok {
			t.Errorf("tile %v not cached", k)
		}
	}
}

func TestWMSFetcherMetaTilesCache(t *testing.T) {
	srv := newMetaServer(t, 0)
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
	if _, err := g.Fetch(context.Background(), geo.TileKey{Z: 4, X: 7, Y: 7}); err != nil {
		t.Fatal(err)
	}
	if requests, _ := srv.counts(); requests != 1 {
		t.Errorf("expected the stored meta tile, %d requests", requests)
	}
}

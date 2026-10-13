package elevation

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/diskcache"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/httpx"
)

// terrarium returns the color of an elevation in the terrarium encoding.
func terrarium(e float64) color.NRGBA {
	v := e + 32768
	r := math.Floor(v / 256)
	g := math.Floor(v - r*256)
	b := math.Round((v - r*256 - g) * 256)
	return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
}

// tileServer serves terrarium tiles of 4x4 pixels up to level maxZoom, with the elevation 1000*z + 10*x + y
// of the tile in all pixels. It answers 404 above maxZoom, and fail for tiles in it.
type tileServer struct {
	*httptest.Server
	maxZoom int

	mu       sync.Mutex
	requests map[string]int
	fail     map[string]bool
}

func newTileServer(t *testing.T, maxZoom int) *tileServer {
	ts := &tileServer{maxZoom: maxZoom, requests: map[string]int{}, fail: map[string]bool{}}
	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var z, x, y int
		if _, err := fmt.Sscanf(r.URL.Path, "/%d/%d/%d.png", &z, &x, &y); err != nil {
			http.NotFound(w, r)
			return
		}
		ts.mu.Lock()
		ts.requests[r.URL.Path]++
		fail := ts.fail[r.URL.Path]
		ts.mu.Unlock()
		switch {
		case fail:
			http.Error(w, "broken", http.StatusInternalServerError)
			return
		case z > ts.maxZoom:
			http.NotFound(w, r)
			return
		}
		img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
		c := terrarium(float64(1000*z + 10*x + y))
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
		}
		_ = png.Encode(w, img)
	}))
	t.Cleanup(ts.Close)
	return ts
}

func (ts *tileServer) source() Source {
	return Source{Name: "test", URL: ts.URL + "/{z}/{x}/{y}.png", Encoding: EncodingTerrarium, TileSize: 4, MaxZoom: 10}
}

// count returns the number of requests of a tile.
func (ts *tileServer) count(z, x, y int) int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.requests[fmt.Sprintf("/%d/%d/%d.png", z, x, y)]
}

func TestElevations(t *testing.T) {
	ts := newTileServer(t, 3)
	s := NewService(ts.source(), httpx.Client{UserAgent: "test"}, 0)
	// Leipzig is in tile 3/4/2, and Sydney in tile 3/7/4. Positions beyond the Web Mercator extent have none.
	pts := []geo.LonLat{{Lon: 12.38, Lat: 51.34}, {Lon: 151.2, Lat: -33.9}, {Lon: 0, Lat: 89}}
	e, err := s.Elevations(context.Background(), pts, 5)
	if err != nil {
		t.Fatal(err)
	}
	if e[0] != 3042 || e[1] != 3074 || !math.IsNaN(e[2]) {
		t.Errorf("unexpected elevations %v", e)
	}
	// Levels without tiles were tried once, and are cached as missing like the tile.
	if ts.count(5, 17, 10) != 1 || ts.count(4, 8, 5) != 1 || ts.count(3, 4, 2) != 1 {
		t.Errorf("unexpected requests %v", ts.requests)
	}
	e2, err := s.Elevation(context.Background(), pts[0], 5)
	if err != nil || e2 != 3042 {
		t.Errorf("expected 3042, got %v, %v", e2, err)
	}
	if ts.count(5, 17, 10) != 1 || ts.count(3, 4, 2) != 1 {
		t.Errorf("expected cached tiles, got requests %v", ts.requests)
	}

	// The level is limited to the highest of the source.
	src := ts.source()
	src.MaxZoom = 2
	e3, err := NewService(src, httpx.Client{}, 0).Elevation(context.Background(), pts[0], 5)
	if err != nil || e3 != 2021 {
		t.Errorf("expected 2021, got %v, %v", e3, err)
	}
}

func TestCached(t *testing.T) {
	ts := newTileServer(t, 3)
	s := NewService(ts.source(), httpx.Client{}, 0)
	p := geo.LonLat{Lon: 12.38, Lat: 51.34}
	if _, ok := s.Cached(p, 5); ok {
		t.Error("expected nothing cached")
	}
	if _, err := s.Elevation(context.Background(), p, 5); err != nil {
		t.Fatal(err)
	}
	// Levels 5 and 4 are cached as missing, so level 3 is used.
	if e, ok := s.Cached(p, 5); !ok || e != 3042 {
		t.Errorf("expected 3042 from the cache, got %v, %v", e, ok)
	}
	// Level 6 was not tried yet.
	if _, ok := s.Cached(p, 6); ok {
		t.Error("expected level 6 not cached")
	}
	if e, ok := s.Cached(geo.LonLat{Lat: 89}, 5); !ok || !math.IsNaN(e) {
		t.Errorf("expected no elevation beyond the tiles, got %v, %v", e, ok)
	}
}

func TestElevationsError(t *testing.T) {
	ts := newTileServer(t, 3)
	ts.fail["/3/4/2.png"] = true
	s := NewService(ts.source(), httpx.Client{}, 0)
	p := geo.LonLat{Lon: 12.38, Lat: 51.34}
	if _, err := s.Elevation(context.Background(), p, 3); err == nil {
		t.Fatal("expected an error")
	}
	// Failed tiles are not cached, but requested again.
	ts.fail["/3/4/2.png"] = false
	e, err := s.Elevation(context.Background(), p, 3)
	if err != nil || e != 3042 || ts.count(3, 4, 2) != 2 {
		t.Errorf("expected 3042 after a second request, got %v, %v, %d requests", e, err, ts.count(3, 4, 2))
	}
}

func TestElevationsCanceled(t *testing.T) {
	ts := newTileServer(t, 3)
	s := NewService(ts.source(), httpx.Client{}, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Elevation(ctx, geo.LonLat{Lon: 12.38, Lat: 51.34}, 3); err == nil {
		t.Error("expected an error for a canceled request")
	}
}

func TestConcurrentDownloads(t *testing.T) {
	ts := newTileServer(t, 3)
	s := NewService(ts.source(), httpx.Client{}, 0)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if e, err := s.Elevation(context.Background(), geo.LonLat{Lon: 12.38, Lat: 51.34}, 3); err != nil || e != 3042 {
				t.Errorf("expected 3042, got %v, %v", e, err)
			}
		})
	}
	wg.Wait()
	if n := ts.count(3, 4, 2); n != 1 {
		t.Errorf("expected the tile downloaded once, got %d", n)
	}
}

func TestCacheEviction(t *testing.T) {
	ts := newTileServer(t, 3)
	// Room for about two tiles of 4x4 pixels.
	s := NewService(ts.source(), httpx.Client{}, 2*(4*16+64))
	a, b, c := geo.LonLat{Lon: 12.38, Lat: 51.34}, geo.LonLat{Lon: 151.2, Lat: -33.9}, geo.LonLat{Lon: -74, Lat: 40.7}
	for _, p := range []geo.LonLat{a, b, c, a} {
		if _, err := s.Elevation(context.Background(), p, 3); err != nil {
			t.Fatal(err)
		}
	}
	// a was evicted by c, and downloaded again.
	if n := ts.count(3, 4, 2); n != 2 {
		t.Errorf("expected the evicted tile downloaded again, got %d requests", n)
	}
	if s.cache.Len() != 2 || len(s.entries) != 2 || s.size > s.maxSize {
		t.Errorf("unexpected cache: %d tiles, %d bytes", s.cache.Len(), s.size)
	}
}

func TestDecode(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, terrarium(-12.5))
	img.SetNRGBA(1, 0, terrarium(8848.25))
	img.SetNRGBA(0, 1, terrarium(0))
	// (1, 1) stays transparent.
	g, err := decode(img, EncodingTerrarium)
	if err != nil {
		t.Fatal(err)
	}
	if g.size != 2 || g.data[0] != -12.5 || g.data[1] != 8848.25 || g.data[2] != 0 || !math.IsNaN(float64(g.data[3])) {
		t.Errorf("unexpected terrarium grid %v", g.data)
	}

	// Mapbox Terrain-RGB, through a PNG round trip, which decodes opaque images as RGBA.
	rgba := image.NewRGBA(image.Rect(0, 0, 1, 1))
	// (100000 + 1234.5 * 10) = 112345 = 0x01B6D9
	rgba.Set(0, 0, color.RGBA{R: 0x01, G: 0xB6, B: 0xD9, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, rgba); err != nil {
		t.Fatal(err)
	}
	dec, _, err := image.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	g, err = decode(dec, EncodingMapbox)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(g.data[0])-1234.5) > 0.01 {
		t.Errorf("unexpected mapbox elevation %v", g.data[0])
	}

	// Other image types are read pixel by pixel.
	gray := image.NewGray(image.Rect(0, 0, 1, 1))
	gray.Pix[0] = 128
	if g, err := decode(gray, EncodingTerrarium); err != nil || g.data[0] != 128*256+128+0.5-32768 {
		t.Errorf("unexpected gray grid %v, %v", g, err)
	}

	if _, err := decode(image.NewNRGBA(image.Rect(0, 0, 2, 1)), EncodingTerrarium); err == nil {
		t.Error("expected an error for a tile that is not square")
	}
	if _, err := decode(img, "lerc"); err == nil {
		t.Error("expected an error for an unknown encoding")
	}
}

func TestSample(t *testing.T) {
	// Pixels 0 10 / 20 30, in tile 1/0/0, which spans the north-west quarter of the Web Mercator plane.
	g := &grid{size: 2, data: []float32{0, 10, 20, 30}}
	k := geo.TileKey{Z: 1}
	b := k.Bounds()
	at := func(fx, fy float64) geo.Point {
		return geo.Point{X: b.Min.X + fx*(b.Max.X-b.Min.X), Y: b.Max.Y - fy*(b.Max.Y-b.Min.Y)}
	}
	for _, tc := range []struct {
		fx, fy, want float64
	}{
		{0.25, 0.25, 0},  // center of the first pixel
		{0.75, 0.75, 30}, // center of the last pixel
		{0.5, 0.5, 15},   // between all four
		{0.5, 0.25, 5},   // between the top two
		{0, 0, 0},        // edges use the edge pixels
		{1, 0.5, 20},
	} {
		if got := g.sample(k, at(tc.fx, tc.fy)); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("at %v, %v: expected %v, got %v", tc.fx, tc.fy, tc.want, got)
		}
	}
}

func TestZoomFor(t *testing.T) {
	for _, tc := range []struct {
		src             Source
		resolution, lat float64
		want            int
	}{
		// At 51°, pixels of 256-pixel tiles are 6.1 m at level 14, and 12.3 m at level 13.
		{AWS, 10, 51, 14},
		{AWS, 12.5, 51, 13},
		// Twice the pixels per tile need one level less.
		{Mapterhorn, 10, 51, 13},
		{AWS, 0.1, 51, 15},
		{AWS, 0, 51, 15},
		{AWS, 1e9, 51, 0},
	} {
		if got := tc.src.ZoomFor(tc.resolution, tc.lat); got != tc.want {
			t.Errorf("%s, %v m at %v°: expected %d, got %d", tc.src.Name, tc.resolution, tc.lat, tc.want, got)
		}
	}
}

func TestConfigure(t *testing.T) {
	s, err := Configure("aws", "", "", 0, 0)
	if err != nil || s != AWS {
		t.Errorf("expected AWS unchanged, got %+v, %v", s, err)
	}
	s, err = Configure("", " https://dem.example.com/{z}/{x}/{y}.png ", "Mapbox", 512, 14)
	want := Source{Name: "mapterhorn", URL: "https://dem.example.com/{z}/{x}/{y}.png", Encoding: EncodingMapbox, TileSize: 512, MaxZoom: 14}
	if err != nil || s != want {
		t.Errorf("expected %+v, got %+v, %v", want, s, err)
	}
	if _, err := Configure("", "", "lerc", 0, 0); err == nil {
		t.Error("expected an error for an unknown encoding")
	}
	if _, err := Configure("srtm", "", "", 0, 0); err == nil {
		t.Error("expected an error for an unknown source")
	}
}

func TestSourceByName(t *testing.T) {
	for name, want := range map[string]string{"": "mapterhorn", " AWS ": "aws", "mapterhorn": "mapterhorn"} {
		s, err := SourceByName(name)
		if err != nil || s.Name != want {
			t.Errorf("%q: expected %s, got %v, %v", name, want, s.Name, err)
		}
	}
	if _, err := SourceByName("srtm"); err == nil {
		t.Error("expected an error for an unknown source")
	}
}

func TestDiskCache(t *testing.T) {
	ts := newTileServer(t, 3)
	disk := diskcache.New(t.TempDir(), 1<<20)
	disk.Wait()
	newService := func() *Service {
		s := NewService(ts.source(), httpx.Client{}, 0)
		s.SetDiskCache(disk, time.Hour)
		return s
	}
	p := geo.LonLat{Lon: 12.38, Lat: 51.34}
	if e, err := newService().Elevation(context.Background(), p, 5); err != nil || e != 3042 {
		t.Fatalf("expected 3042, got %v, %v", e, err)
	}
	dir := diskcache.Name("test", ts.source().URL)
	if data, _, ok := disk.Get(dir + "/3/4/2.png"); !ok || len(data) == 0 {
		t.Error("tile not stored")
	}
	if data, _, ok := disk.Get(dir + "/5/17/10.png"); !ok || len(data) != 0 {
		t.Error("missing tile not stored as empty file")
	}

	// A new service uses the stored tiles, also for knowing which are missing.
	if e, err := newService().Elevation(context.Background(), p, 5); err != nil || e != 3042 {
		t.Fatalf("expected 3042, got %v, %v", e, err)
	}
	if ts.count(5, 17, 10) != 1 || ts.count(4, 8, 5) != 1 || ts.count(3, 4, 2) != 1 {
		t.Errorf("expected stored tiles, got requests %v", ts.requests)
	}

	// Too old, and the server fails: the old tiles are used.
	old := time.Now().Add(-2 * time.Hour)
	for _, f := range []string{"5/17/10", "4/8/5", "3/4/2"} {
		if err := os.Chtimes(filepath.Join(disk.Dir(), dir, filepath.FromSlash(f+".png")), old, old); err != nil {
			t.Fatal(err)
		}
		ts.mu.Lock()
		ts.fail["/"+f+".png"] = true
		ts.mu.Unlock()
	}
	if e, err := newService().Elevation(context.Background(), p, 5); err != nil || e != 3042 {
		t.Fatalf("expected 3042 from old tiles, got %v, %v", e, err)
	}
	if ts.count(5, 17, 10) != 2 || ts.count(3, 4, 2) != 2 {
		t.Errorf("expected old tiles checked once, got requests %v", ts.requests)
	}
}

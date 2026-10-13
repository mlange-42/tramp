// Package elevation looks up the terrain elevation at positions, from Terrain-RGB tiles:
// map tiles with the elevation encoded in the colors of their pixels.
package elevation

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/png" // Tiles of AWS Terrain Tiles.
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mlange-42/tramp/internal/diskcache"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/httpx"
	_ "golang.org/x/image/webp" // Tiles of Mapterhorn.
)

// Encodings of the elevation in the pixel colors.
const (
	// EncodingTerrarium is elevation = R*256 + G + B/256 - 32768, used by AWS Terrain Tiles and Mapterhorn.
	EncodingTerrarium = "terrarium"
	// EncodingMapbox is elevation = (R*65536 + G*256 + B) * 0.1 - 10000, used by Mapbox Terrain-RGB.
	EncodingMapbox = "mapbox"
)

// Source is a service of Terrain-RGB tiles.
type Source struct {
	// Name identifies the source in the settings.
	Name string
	// URL is a template like "https://tiles.example.com/{z}/{x}/{y}.png".
	URL string
	// Encoding is [EncodingTerrarium] or [EncodingMapbox].
	Encoding string
	// TileSize is the edge length of the tiles in pixels.
	TileSize int
	// MaxZoom is the highest tile level. Where the service has no tile at a level, the levels below are used.
	MaxZoom int
	// Attribution is the full credit of the source, AttributionShort the short one shown on the map,
	// and AttributionURL links to the credits of the data, as the data licenses require.
	Attribution, AttributionShort, AttributionURL string
}

// Built-in sources.
var (
	// AWS are the AWS Terrain Tiles from the Registry of Open Data on AWS, worldwide and mostly based on SRTM.
	AWS = Source{
		Name:             "aws",
		URL:              "https://s3.amazonaws.com/elevation-tiles-prod/terrarium/{z}/{x}/{y}.png",
		Encoding:         EncodingTerrarium,
		TileSize:         256,
		MaxZoom:          15,
		Attribution:      "AWS Terrain Tiles, from Mapzen, with data of SRTM, GMTED, ETOPO1 and others",
		AttributionShort: "AWS Terrain Tiles",
		AttributionURL:   "https://github.com/tilezen/joerd/blob/master/docs/attribution.md",
	}
	// Mapterhorn is worldwide, with high-resolution national elevation models in many countries.
	// Its data comes from many open-data sources, which are listed on its attribution page.
	// There is no usage policy for its public tiles; it logs IP addresses and user agents for up to 30 days,
	// see https://mapterhorn.com/privacy-policy/.
	Mapterhorn = Source{
		Name:             "mapterhorn",
		URL:              "https://tiles.mapterhorn.com/{z}/{x}/{y}.webp",
		Encoding:         EncodingTerrarium,
		TileSize:         512,
		MaxZoom:          16,
		Attribution:      "© Mapterhorn, with elevation data from many open-data sources",
		AttributionShort: "© Mapterhorn",
		AttributionURL:   "https://mapterhorn.com/attribution",
	}
)

// Sources are the built-in sources, the default first.
// Mapterhorn is the default, as it is much more accurate, especially in the mountains,
// and needs fewer downloads for the same resolution.
var Sources = []Source{Mapterhorn, AWS}

// SourceByName returns the built-in source with the given name, ignoring case, or the default for an empty name.
func SourceByName(name string) (Source, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Sources[0], nil
	}
	for _, s := range Sources {
		if strings.EqualFold(s.Name, name) {
			return s, nil
		}
	}
	names := make([]string, len(Sources))
	for i, s := range Sources {
		names[i] = s.Name
	}
	return Source{}, fmt.Errorf("unknown elevation source %q, expected one of %s", name, strings.Join(names, ", "))
}

// Configure returns the built-in source with the given name, the default if empty, with the URL, encoding,
// tile size and highest level replaced by those given, if not empty or zero, for using another service.
func Configure(name, url, encoding string, tileSize, maxZoom int) (Source, error) {
	s, err := SourceByName(name)
	if err != nil {
		return s, err
	}
	if url = strings.TrimSpace(url); url != "" {
		s.URL = url
		// The attribution of the built-in source doesn't apply to another service.
		s.Attribution, s.AttributionShort, s.AttributionURL = "", "", ""
	}
	if encoding = strings.ToLower(strings.TrimSpace(encoding)); encoding != "" {
		if encoding != EncodingTerrarium && encoding != EncodingMapbox {
			return s, fmt.Errorf("unknown elevation encoding %q, expected %s or %s", encoding, EncodingTerrarium, EncodingMapbox)
		}
		s.Encoding = encoding
	}
	if tileSize > 0 {
		s.TileSize = tileSize
	}
	if maxZoom > 0 {
		s.MaxZoom = maxZoom
	}
	return s, nil
}

// tileURL returns the address of a tile.
func (s *Source) tileURL(k geo.TileKey) string {
	return strings.NewReplacer("{z}", strconv.Itoa(k.Z), "{x}", strconv.Itoa(k.X), "{y}", strconv.Itoa(k.Y)).Replace(s.URL)
}

// ZoomFor returns the lowest tile level with pixels no larger than resolution meters at latitude lat,
// at most [Source.MaxZoom].
func (s Source) ZoomFor(resolution, lat float64) int {
	size := s.TileSize
	if size <= 0 {
		size = geo.TileSize
	}
	// The ground size of a pixel at level 0, at the latitude.
	pixel0 := 2 * geo.OriginShift * math.Cos(lat*math.Pi/180) / float64(size)
	if resolution <= 0 {
		return s.MaxZoom
	}
	z := int(math.Ceil(math.Log2(pixel0 / resolution)))
	return min(max(z, 0), s.MaxZoom)
}

// errMissing is the error for tiles the source doesn't have.
var errMissing = errors.New("tile not available")

// workers is the number of tiles downloaded at the same time.
const workers = 4

// DefaultCacheSize is the default memory for decoded tiles, in bytes.
const DefaultCacheSize = 128 << 20

// Service looks up elevations from a source, with an in-memory cache of decoded tiles.
// It is safe for concurrent use.
type Service struct {
	src    Source
	client httpx.Client

	mu sync.Mutex
	// cache holds the tiles, the most recently used in front, and entries finds them.
	cache   *list.List
	entries map[geo.TileKey]*list.Element
	// size is the memory of the cached tiles, and maxSize the limit.
	size, maxSize int
	// loading are the tiles being downloaded, so that each is downloaded once.
	loading map[geo.TileKey]*download

	// disk stores the downloaded tiles, nil for none, and maxAge is the age after which they are checked
	// with the server. diskDir is the directory of the source in it, and diskExt the file extension of the tiles.
	disk             *diskcache.Cache
	maxAge           time.Duration
	diskDir, diskExt string
}

// cacheEntry is a cached tile. grid is nil for a tile the source doesn't have.
type cacheEntry struct {
	key  geo.TileKey
	grid *grid
}

// download is a tile being downloaded. done is closed when it finished.
type download struct {
	done chan struct{}
	grid *grid
	err  error
}

// NewService returns a service for the source. cacheSize is the memory for decoded tiles in bytes,
// [DefaultCacheSize] if zero or less.
func NewService(src Source, client httpx.Client, cacheSize int) *Service {
	if cacheSize <= 0 {
		cacheSize = DefaultCacheSize
	}
	return &Service{
		src:     src,
		client:  client,
		cache:   list.New(),
		entries: map[geo.TileKey]*list.Element{},
		maxSize: cacheSize,
		loading: map[geo.TileKey]*download{},
	}
}

// SetDiskCache stores the downloaded tiles on disk, in a directory for the source.
// Tiles older than maxAge are checked with the server, and still used if it can't be reached.
// Tiles the source doesn't have are stored as empty files. It must be called before the service is used.
func (s *Service) SetDiskCache(c *diskcache.Cache, maxAge time.Duration) {
	s.disk, s.maxAge = c, maxAge
	s.diskDir = diskcache.Name(s.src.Name, s.src.URL)
	s.diskExt = diskcache.ImageExt(s.src.URL)
}

// Source returns the source of the service.
func (s *Service) Source() Source { return s.src }

// Elevation returns the elevation at a position in meters, from the tile at level zoom, or below
// where the source has none. It is NaN outside the tiles.
func (s *Service) Elevation(ctx context.Context, p geo.LonLat, zoom int) (float64, error) {
	e, err := s.Elevations(ctx, []geo.LonLat{p}, zoom)
	if err != nil {
		return math.NaN(), err
	}
	return e[0], nil
}

// Elevations returns the elevations at positions in meters, like [Service.Elevation].
// The tiles are downloaded concurrently.
func (s *Service) Elevations(ctx context.Context, pts []geo.LonLat, zoom int) ([]float64, error) {
	res := make([]float64, len(pts))
	merc := make([]geo.Point, len(pts))
	pending := make([]int, 0, len(pts))
	for i, p := range pts {
		res[i] = math.NaN()
		if math.Abs(p.Lat) <= geo.MaxLat {
			merc[i] = geo.ToMercator(p)
			pending = append(pending, i)
		}
	}
	for z := min(zoom, s.src.MaxZoom); z >= 0 && len(pending) > 0; z-- {
		byTile := map[geo.TileKey][]int{}
		var keys []geo.TileKey
		for _, i := range pending {
			k := geo.TileAt(z, merc[i])
			if !k.Valid() {
				continue
			}
			if _, ok := byTile[k]; !ok {
				keys = append(keys, k)
			}
			byTile[k] = append(byTile[k], i)
		}
		grids, err := s.tiles(ctx, keys)
		if err != nil {
			return nil, err
		}
		pending = pending[:0]
		for _, k := range keys {
			g := grids[k]
			for _, i := range byTile[k] {
				if g == nil {
					// Tried again on the level below.
					pending = append(pending, i)
					continue
				}
				res[i] = g.sample(k, merc[i])
			}
		}
	}
	return res, nil
}

// Cached returns the elevation at a position like [Service.Elevation], but only from cached tiles,
// without waiting for downloads. It returns false if a needed tile is not cached.
func (s *Service) Cached(p geo.LonLat, zoom int) (float64, bool) {
	if math.Abs(p.Lat) > geo.MaxLat {
		return math.NaN(), true
	}
	m := geo.ToMercator(p)
	s.mu.Lock()
	defer s.mu.Unlock()
	for z := min(zoom, s.src.MaxZoom); z >= 0; z-- {
		k := geo.TileAt(z, m)
		if !k.Valid() {
			break
		}
		el, ok := s.entries[k]
		if !ok {
			return math.NaN(), false
		}
		s.cache.MoveToFront(el)
		if g := el.Value.(*cacheEntry).grid; g != nil {
			return g.sample(k, m), true
		}
	}
	return math.NaN(), true
}

// tiles returns the tiles, downloading those not cached concurrently.
// Tiles the source doesn't have are nil.
func (s *Service) tiles(ctx context.Context, keys []geo.TileKey) (map[geo.TileKey]*grid, error) {
	grids := make(map[geo.TileKey]*grid, len(keys))
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, k := range keys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			g, err := s.tile(ctx, k)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				grids[k] = g
			case errors.Is(err, errMissing):
				grids[k] = nil
			case firstErr == nil:
				firstErr = err
			}
		}()
	}
	wg.Wait()
	return grids, firstErr
}

// tile returns a tile from the cache, or downloads it. It returns [errMissing] for tiles the source doesn't have.
func (s *Service) tile(ctx context.Context, k geo.TileKey) (*grid, error) {
	s.mu.Lock()
	if el, ok := s.entries[k]; ok {
		s.cache.MoveToFront(el)
		g := el.Value.(*cacheEntry).grid
		s.mu.Unlock()
		if g == nil {
			return nil, errMissing
		}
		return g, nil
	}
	d, ok := s.loading[k]
	if !ok {
		d = &download{done: make(chan struct{})}
		s.loading[k] = d
		go s.download(k, d)
	}
	s.mu.Unlock()

	select {
	case <-d.done:
		return d.grid, d.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// downloadTimeout limits a single download. Downloads are not canceled with the request that started them,
// as other requests may wait for the same tile, and the tile is cached for later ones.
var downloadTimeout = 30 * time.Second

// download downloads and decodes a tile, caches it, and finishes d.
func (s *Service) download(k geo.TileKey, d *download) {
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()
	g, err := s.fetch(ctx, k)
	s.mu.Lock()
	delete(s.loading, k)
	if err == nil || errors.Is(err, errMissing) {
		s.insert(k, g)
	}
	s.mu.Unlock()
	d.grid, d.err = g, err
	close(d.done)
}

// fetch loads and decodes a tile.
func (s *Service) fetch(ctx context.Context, k geo.TileKey) (*grid, error) {
	data, err := s.load(ctx, k)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("elevation tile %d/%d/%d: %w", k.Z, k.X, k.Y, err)
	}
	return decode(img, s.src.Encoding)
}

// load returns the image data of a tile from the disk cache, or downloads it if it is not stored or too old.
// It returns [errMissing] for tiles the source doesn't have.
func (s *Service) load(ctx context.Context, k geo.TileKey) ([]byte, error) {
	if s.disk == nil {
		data, _, err := s.get(ctx, k, time.Time{})
		return data, err
	}
	key := s.diskDir + "/" + strconv.Itoa(k.Z) + "/" + strconv.Itoa(k.X) + "/" + strconv.Itoa(k.Y) + s.diskExt
	cached, mod, ok := s.disk.Get(key)
	stored := func() ([]byte, error) {
		if len(cached) == 0 {
			return nil, errMissing
		}
		return cached, nil
	}
	if ok && time.Since(mod) < s.maxAge {
		return stored()
	}
	var since time.Time
	if ok && len(cached) > 0 {
		since = mod
	}
	data, noStore, err := s.get(ctx, k, since)
	switch {
	case errors.Is(err, errNotModified):
		if err := s.disk.Touch(key); err != nil {
			log.Printf("elevation cache: %v", err)
		}
		return stored()
	case errors.Is(err, errMissing):
		data, noStore = nil, false
	case err != nil:
		if ok {
			// Offline, or the server fails: the old tile is better than none.
			return stored()
		}
		return nil, err
	}
	if !noStore {
		if err := s.disk.Put(key, data); err != nil {
			log.Printf("elevation cache: %v", err)
		}
	}
	if data == nil {
		return nil, errMissing
	}
	return data, nil
}

// errNotModified is returned by [Service.get] if the tile has not changed since the given time.
var errNotModified = errors.New("not modified")

// get downloads a tile. If since is not zero, the server is asked whether the tile changed since then,
// and [errNotModified] is returned if not. noStore is whether the server forbids storing the tile.
func (s *Service) get(ctx context.Context, k geo.TileKey, since time.Time) (data []byte, noStore bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.src.tileURL(k), nil)
	if err != nil {
		return nil, false, err
	}
	if !since.IsZero() {
		req.Header.Set("If-Modified-Since", since.UTC().Format(http.TimeFormat))
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("elevation tile %d/%d/%d: %w", k.Z, k.X, k.Y, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotModified:
		if !since.IsZero() {
			return nil, false, errNotModified
		}
		return nil, false, fmt.Errorf("elevation tile %d/%d/%d: %s", k.Z, k.X, k.Y, resp.Status)
	case http.StatusNotFound, http.StatusForbidden, http.StatusNoContent:
		// AWS answers 403 for tiles not in its bucket.
		return nil, false, errMissing
	default:
		return nil, false, fmt.Errorf("elevation tile %d/%d/%d: %s", k.Z, k.X, k.Y, resp.Status)
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, false, fmt.Errorf("elevation tile %d/%d/%d: %w", k.Z, k.X, k.Y, err)
	}
	noStore = strings.Contains(strings.ToLower(resp.Header.Get("Cache-Control")), "no-store")
	return data, noStore, nil
}

// insert caches a tile, nil for a tile the source doesn't have, and evicts the least recently used ones
// over the memory limit. The caller must hold s.mu.
func (s *Service) insert(k geo.TileKey, g *grid) {
	s.entries[k] = s.cache.PushFront(&cacheEntry{key: k, grid: g})
	s.size += g.bytes()
	for s.size > s.maxSize && s.cache.Len() > 1 {
		el := s.cache.Back()
		e := el.Value.(*cacheEntry)
		s.cache.Remove(el)
		delete(s.entries, e.key)
		s.size -= e.grid.bytes()
	}
}

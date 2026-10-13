// Package tiles loads map tiles in the background and keeps them in an in-memory cache.
package tiles

import (
	"container/list"
	"context"
	"image"
	"log"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"gioui.org/op/paint"
	"github.com/mlange-42/tramp/internal/geo"
)

// TileBytes is the memory of a loaded tile, with 4 bytes per pixel.
const TileBytes = geo.TileSize * geo.TileSize * 4

// Fetcher retrieves the image for a single tile.
type Fetcher interface {
	Fetch(ctx context.Context, key geo.TileKey) (image.Image, error)
}

// GroupFetcher is a [Fetcher] that loads tiles in square groups, like the meta tiles of a WMS.
// The manager then requests each group once, and adds all its tiles.
type GroupFetcher interface {
	Fetcher
	// Group returns the first tile of the group containing key, and its number of tiles per side.
	// A size of 1 means single tiles, loaded with Fetch.
	Group(key geo.TileKey) (geo.TileKey, int)
	// FetchGroup retrieves the images of the n×n tiles starting at first, row by row.
	FetchGroup(ctx context.Context, first geo.TileKey, n int) ([]image.Image, error)
}

// Tile is a loaded map tile, ready for drawing.
type Tile struct {
	Key geo.TileKey
	Op  paint.ImageOp
	// gen is the generation of the manager when the tile was loaded, see [Manager.Refresh].
	gen int
}

// Options configure a [Manager].
type Options struct {
	// Workers is the number of concurrent downloads.
	Workers int
	// Capacity is the maximum number of tiles kept in memory.
	Capacity int
	// Timeout for a single download, of a tile or a group of tiles.
	Timeout time.Duration
	// RetryAfter is the delay before a failed tile is requested again.
	RetryAfter time.Duration
	// OnUpdate is called from a worker goroutine whenever a tile has been loaded or failed.
	OnUpdate func()
	// Limiter limits the simultaneous downloads from the server Host, shared with other managers,
	// to Connections, see [Limiter.Acquire]. Nil for no limit besides Workers.
	Limiter     *Limiter
	Host        string
	Connections int
	// Name identifies the tiles in log messages, like the name of the layer.
	Name string
}

// DefaultOptions returns sensible defaults for interactive use.
func DefaultOptions() Options {
	return Options{
		Workers:    6,
		Capacity:   512,
		Timeout:    20 * time.Second,
		RetryAfter: 30 * time.Second,
	}
}

// Manager loads tiles with a pool of workers and caches them with LRU eviction.
//
// The map view calls [Manager.Get] for tiles it wants to draw and
// [Manager.Request] once per frame with all tiles it wants.
// Requests that are no longer wanted are dropped from the queue.
//
// Downloads are done in units: single tiles, or groups for a [GroupFetcher].
// The queue, the downloads and the failures are kept by unit, identified by its first tile.
type Manager struct {
	fetcher Fetcher
	opts    Options
	ctx     context.Context
	cancel  context.CancelFunc

	mu    sync.Mutex
	wake  *sync.Cond
	cache map[geo.TileKey]*list.Element
	lru   *list.List
	// queue and inflight are units, see [Manager.unit].
	queue    []geo.TileKey
	inflight map[geo.TileKey]struct{}
	// wanted are the tiles of the last request, see [Manager.Wanted].
	wanted []geo.TileKey
	failed map[geo.TileKey]time.Time
	// gen is increased by [Manager.Refresh]. Cached tiles of older generations are loaded again.
	gen    int
	closed bool
	// background is set for managers that load tiles not shown yet, see [Manager.SetBackground].
	background atomic.Bool
	wg         sync.WaitGroup
}

// NewManager creates a manager and starts its workers.
func NewManager(fetcher Fetcher, opts Options) *Manager {
	def := DefaultOptions()
	if opts.Workers <= 0 {
		opts.Workers = def.Workers
	}
	if opts.Capacity <= 0 {
		opts.Capacity = def.Capacity
	}
	if opts.Timeout <= 0 {
		opts.Timeout = def.Timeout
	}
	if opts.RetryAfter <= 0 {
		opts.RetryAfter = def.RetryAfter
	}

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		fetcher:  fetcher,
		opts:     opts,
		ctx:      ctx,
		cancel:   cancel,
		cache:    map[geo.TileKey]*list.Element{},
		lru:      list.New(),
		inflight: map[geo.TileKey]struct{}{},
		failed:   map[geo.TileKey]time.Time{},
	}
	m.wake = sync.NewCond(&m.mu)

	for range opts.Workers {
		m.wg.Add(1)
		go m.work()
	}
	return m
}

// SetCapacity changes the maximum number of tiles kept in memory, evicting tiles if necessary.
func (m *Manager) SetCapacity(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.opts.Capacity = max(1, n)
	m.evict()
}

// Capacity returns the maximum number of tiles kept in memory.
func (m *Manager) Capacity() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.opts.Capacity
}

// Get returns the tile if it is in the cache, and marks it as recently used.
func (m *Manager) Get(key geo.TileKey) (*Tile, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	el, ok := m.cache[key]
	if !ok {
		return nil, false
	}
	m.lru.MoveToFront(el)
	return el.Value.(*Tile), true
}

// Request replaces the download queue with the given tiles, in order of priority.
// Tiles that are cached, already loading or failed recently are skipped.
// Cached tiles from before the last [Manager.Refresh] are loaded again, after the missing tiles.
func (m *Manager) Request(keys []geo.TileKey) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	m.queue = m.queue[:0]
	var stale []geo.TileKey
	queued := map[geo.TileKey]bool{}
	for _, k := range keys {
		el, cached := m.cache[k]
		if cached && el.Value.(*Tile).gen == m.gen {
			continue
		}
		u, _ := m.unit(k)
		if queued[u] {
			continue
		}
		if _, ok := m.inflight[u]; ok {
			continue
		}
		if t, ok := m.failed[u]; ok {
			if now.Sub(t) < m.opts.RetryAfter {
				continue
			}
			delete(m.failed, u)
		}
		queued[u] = true
		if cached {
			stale = append(stale, u)
		} else {
			m.queue = append(m.queue, u)
		}
	}
	m.queue = append(m.queue, stale...)
	m.wanted = append(m.wanted[:0], keys...)
	if len(m.queue) > 0 {
		m.wake.Broadcast()
	}
}

// SetBackground sets whether the manager loads tiles in the background, not shown yet.
// Its downloads then wait for those of other managers from the same server, see [Options.Limiter].
func (m *Manager) SetBackground(b bool) {
	m.background.Store(b)
}

// Wanted returns the tiles of the last [Manager.Request], like the visible tiles of the map.
func (m *Manager) Wanted() []geo.TileKey {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.wanted)
}

// Refresh marks all cached tiles as outdated, for layers that change over time.
// They are still returned by [Manager.Get] until they are loaded again on request.
func (m *Manager) Refresh() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gen++
	clear(m.failed)
}

// unit returns the first tile of the download unit containing key, and its number of tiles per side.
func (m *Manager) unit(key geo.TileKey) (geo.TileKey, int) {
	if g, ok := m.fetcher.(GroupFetcher); ok {
		return g.Group(key)
	}
	return key, 1
}

// Loading returns the number of downloads queued or running: of single tiles, or of groups of tiles.
func (m *Manager) Loading() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.queue) + len(m.inflight)
}

// Close stops all workers and cancels running downloads.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	m.queue = nil
	m.wake.Broadcast()
	m.mu.Unlock()

	m.cancel()
	m.wg.Wait()
}

func (m *Manager) work() {
	defer m.wg.Done()
	for {
		key, gen, ok := m.next()
		if !ok {
			return
		}
		tiles, err := m.load(key, gen)

		m.mu.Lock()
		delete(m.inflight, key)
		if err != nil {
			if m.ctx.Err() == nil {
				log.Printf("tile %v of %s: %v", key, m.opts.Name, err)
				m.failed[key] = time.Now()
			}
		} else {
			for _, t := range tiles {
				m.insert(t)
			}
		}
		m.mu.Unlock()

		if m.opts.OnUpdate != nil {
			m.opts.OnUpdate()
		}
	}
}

// next blocks until a unit is queued, and marks it as in flight. It returns the current generation.
func (m *Manager) next() (geo.TileKey, int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for len(m.queue) == 0 && !m.closed {
		m.wake.Wait()
	}
	if m.closed {
		return geo.TileKey{}, 0, false
	}
	key := m.queue[0]
	m.queue = m.queue[1:]
	m.inflight[key] = struct{}{}
	return key, m.gen, true
}

// load fetches the tiles of a unit, and stamps them with the generation they were requested in.
func (m *Manager) load(first geo.TileKey, gen int) ([]*Tile, error) {
	// Waiting for a free connection doesn't count for the timeout.
	if m.opts.Limiter != nil {
		release, err := m.opts.Limiter.Acquire(m.ctx, m.opts.Host, m.opts.Connections, m.background.Load())
		if err != nil {
			return nil, err
		}
		defer release()
	}
	ctx, cancel := context.WithTimeout(m.ctx, m.opts.Timeout)
	defer cancel()

	// Converts to RGBA off the UI goroutine.
	tile := func(key geo.TileKey, img image.Image) *Tile {
		return &Tile{Key: key, Op: paint.NewImageOp(img), gen: gen}
	}
	if _, n := m.unit(first); n > 1 {
		imgs, err := m.fetcher.(GroupFetcher).FetchGroup(ctx, first, n)
		if err != nil {
			return nil, err
		}
		tiles := make([]*Tile, 0, len(imgs))
		for i, img := range imgs {
			tiles = append(tiles, tile(geo.TileKey{Z: first.Z, X: first.X + i%n, Y: first.Y + i/n}, img))
		}
		return tiles, nil
	}
	img, err := m.fetcher.Fetch(ctx, first)
	if err != nil {
		return nil, err
	}
	return []*Tile{tile(first, img)}, nil
}

// insert adds a tile to the cache. The caller must hold the lock.
func (m *Manager) insert(t *Tile) {
	if el, ok := m.cache[t.Key]; ok {
		el.Value = t
		m.lru.MoveToFront(el)
		return
	}
	m.cache[t.Key] = m.lru.PushFront(t)
	m.evict()
}

// evict removes the least recently used tiles until the capacity is met. The caller must hold the lock.
func (m *Manager) evict() {
	for m.lru.Len() > m.opts.Capacity {
		el := m.lru.Back()
		m.lru.Remove(el)
		delete(m.cache, el.Value.(*Tile).Key)
	}
}

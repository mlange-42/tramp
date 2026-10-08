// Package tiles loads map tiles in the background and keeps them in an in-memory cache.
package tiles

import (
	"container/list"
	"context"
	"image"
	"log"
	"sync"
	"time"

	"gioui.org/op/paint"
	"github.com/mlange-42/tramp/internal/geo"
)

// Fetcher retrieves the image for a single tile.
type Fetcher interface {
	Fetch(ctx context.Context, key geo.TileKey) (image.Image, error)
}

// Tile is a loaded map tile, ready for drawing.
type Tile struct {
	Key geo.TileKey
	Op  paint.ImageOp
}

// Options configure a [Manager].
type Options struct {
	// Workers is the number of concurrent downloads.
	Workers int
	// Capacity is the maximum number of tiles kept in memory.
	Capacity int
	// Timeout for a single tile request.
	Timeout time.Duration
	// RetryAfter is the delay before a failed tile is requested again.
	RetryAfter time.Duration
	// OnUpdate is called from a worker goroutine whenever a tile has been loaded or failed.
	OnUpdate func()
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
// [Manager.Request] once per frame with the tiles still missing.
// Requests that are no longer wanted are dropped from the queue.
type Manager struct {
	fetcher Fetcher
	opts    Options
	ctx     context.Context
	cancel  context.CancelFunc

	mu       sync.Mutex
	wake     *sync.Cond
	cache    map[geo.TileKey]*list.Element
	lru      *list.List
	queue    []geo.TileKey
	inflight map[geo.TileKey]struct{}
	failed   map[geo.TileKey]time.Time
	closed   bool
	wg       sync.WaitGroup
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
func (m *Manager) Request(keys []geo.TileKey) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	m.queue = m.queue[:0]
	for _, k := range keys {
		if _, ok := m.cache[k]; ok {
			continue
		}
		if _, ok := m.inflight[k]; ok {
			continue
		}
		if t, ok := m.failed[k]; ok {
			if now.Sub(t) < m.opts.RetryAfter {
				continue
			}
			delete(m.failed, k)
		}
		m.queue = append(m.queue, k)
	}
	if len(m.queue) > 0 {
		m.wake.Broadcast()
	}
}

// Loading returns the number of tiles currently queued or downloading.
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
		key, ok := m.next()
		if !ok {
			return
		}
		tile, err := m.load(key)

		m.mu.Lock()
		delete(m.inflight, key)
		if err != nil {
			if m.ctx.Err() == nil {
				log.Printf("tile %v: %v", key, err)
				m.failed[key] = time.Now()
			}
		} else {
			m.insert(tile)
		}
		m.mu.Unlock()

		if m.opts.OnUpdate != nil {
			m.opts.OnUpdate()
		}
	}
}

// next blocks until a tile is queued, and marks it as in flight.
func (m *Manager) next() (geo.TileKey, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for len(m.queue) == 0 && !m.closed {
		m.wake.Wait()
	}
	if m.closed {
		return geo.TileKey{}, false
	}
	key := m.queue[0]
	m.queue = m.queue[1:]
	m.inflight[key] = struct{}{}
	return key, true
}

func (m *Manager) load(key geo.TileKey) (*Tile, error) {
	ctx, cancel := context.WithTimeout(m.ctx, m.opts.Timeout)
	defer cancel()

	img, err := m.fetcher.Fetch(ctx, key)
	if err != nil {
		return nil, err
	}
	// Converts to RGBA off the UI goroutine.
	return &Tile{Key: key, Op: paint.NewImageOp(img)}, nil
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

package tiles

import (
	"context"
	"errors"
	"image"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
)

type fakeFetcher struct {
	calls atomic.Int32
	fail  atomic.Bool
}

func (f *fakeFetcher) Fetch(ctx context.Context, key geo.TileKey) (image.Image, error) {
	f.calls.Add(1)
	if f.fail.Load() {
		return nil, errors.New("failed")
	}
	return image.NewRGBA(image.Rect(0, 0, 2, 2)), nil
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timeout")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestManagerLoads(t *testing.T) {
	var loaded sync.WaitGroup
	loaded.Add(3)
	f := &fakeFetcher{}
	m := NewManager(f, Options{Workers: 2, OnUpdate: loaded.Done})
	defer m.Close()

	keys := []geo.TileKey{{Z: 1, X: 0, Y: 0}, {Z: 1, X: 1, Y: 0}, {Z: 1, X: 0, Y: 1}}
	m.Request(keys)
	loaded.Wait()

	for _, k := range keys {
		tile, ok := m.Get(k)
		if !ok || tile.Key != k {
			t.Errorf("tile %v not cached", k)
		}
	}
	// Cached tiles are not requested again.
	m.Request(keys)
	if m.Loading() != 0 || f.calls.Load() != 3 {
		t.Errorf("expected no new requests, got %d calls", f.calls.Load())
	}
}

func TestManagerEvicts(t *testing.T) {
	f := &fakeFetcher{}
	m := NewManager(f, Options{Workers: 1, Capacity: 2})
	defer m.Close()

	for x := range 3 {
		k := geo.TileKey{Z: 2, X: x}
		m.Request([]geo.TileKey{k})
		waitFor(t, func() bool { _, ok := m.Get(k); return ok })
	}
	if _, ok := m.Get(geo.TileKey{Z: 2, X: 0}); ok {
		t.Error("oldest tile should have been evicted")
	}
}

func TestManagerSetCapacity(t *testing.T) {
	f := &fakeFetcher{}
	m := NewManager(f, Options{Workers: 1, Capacity: 3})
	defer m.Close()

	for x := range 3 {
		k := geo.TileKey{Z: 2, X: x}
		m.Request([]geo.TileKey{k})
		waitFor(t, func() bool { _, ok := m.Get(k); return ok })
	}
	m.SetCapacity(1)
	for x, want := range []bool{false, false, true} {
		if _, ok := m.Get(geo.TileKey{Z: 2, X: x}); ok != want {
			t.Errorf("tile %d cached: %v, want %v", x, ok, want)
		}
	}
}

func TestManagerFailedNotRetried(t *testing.T) {
	f := &fakeFetcher{}
	f.fail.Store(true)
	m := NewManager(f, Options{Workers: 1, RetryAfter: time.Hour})
	defer m.Close()

	k := geo.TileKey{Z: 3}
	m.Request([]geo.TileKey{k})
	waitFor(t, func() bool { return m.Loading() == 0 && f.calls.Load() == 1 })
	m.Request([]geo.TileKey{k})
	if m.Loading() != 0 {
		t.Error("failed tile should not be queued again before RetryAfter")
	}
}

func TestManagerRefresh(t *testing.T) {
	f := &fakeFetcher{}
	m := NewManager(f, Options{Workers: 1, RetryAfter: time.Hour})
	defer m.Close()

	a, b := geo.TileKey{Z: 4, X: 1}, geo.TileKey{Z: 4, X: 2}
	m.Request([]geo.TileKey{a})
	waitFor(t, func() bool { _, ok := m.Get(a); return ok })
	old, _ := m.Get(a)

	// The outdated tile is still drawn, and loaded again after the missing one.
	m.Refresh()
	if tile, ok := m.Get(a); !ok || tile != old {
		t.Fatal("outdated tile should stay cached until it is replaced")
	}
	m.Request([]geo.TileKey{a, b})
	waitFor(t, func() bool { return m.Loading() == 0 && f.calls.Load() == 3 })
	if tile, _ := m.Get(a); tile == old {
		t.Error("outdated tile was not replaced")
	}
	// Fresh tiles are not requested again.
	m.Request([]geo.TileKey{a, b})
	if m.Loading() != 0 {
		t.Error("fresh tiles should not be queued")
	}

	// If loading fails, the outdated tile is kept and not retried before RetryAfter.
	f.fail.Store(true)
	m.Refresh()
	m.Request([]geo.TileKey{a})
	waitFor(t, func() bool { return m.Loading() == 0 && f.calls.Load() == 4 })
	if _, ok := m.Get(a); !ok {
		t.Error("outdated tile should be kept if loading fails")
	}
	m.Request([]geo.TileKey{a})
	if m.Loading() != 0 {
		t.Error("failed refresh should not be retried before RetryAfter")
	}
}

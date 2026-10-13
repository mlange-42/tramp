package tiles

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
)

func TestLimiterLimit(t *testing.T) {
	l := NewLimiter()
	ctx := context.Background()
	var releases []func()
	for range 2 {
		r, err := l.Acquire(ctx, "a", 2, false)
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, r)
	}
	// Other servers are not affected.
	if r, err := l.Acquire(ctx, "b", 2, false); err != nil {
		t.Fatal(err)
	} else {
		r()
	}
	// A third download waits for a free slot.
	got := make(chan struct{})
	go func() {
		r, err := l.Acquire(ctx, "a", 2, false)
		if err == nil {
			close(got)
			r()
		}
	}()
	select {
	case <-got:
		t.Fatal("third download should wait")
	case <-time.After(20 * time.Millisecond):
	}
	releases[0]()
	releases[0]() // releasing twice has no effect
	<-got
	releases[1]()
}

func TestLimiterPriority(t *testing.T) {
	l := NewLimiter()
	ctx := context.Background()
	r, _ := l.Acquire(ctx, "a", 1, false)

	var mu sync.Mutex
	var order []string
	var wg sync.WaitGroup
	start := func(name string, background bool) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := l.Acquire(ctx, "a", 1, background)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			release()
		}()
		time.Sleep(10 * time.Millisecond)
	}
	// The background download waits first, but the foreground one goes before it.
	start("background", true)
	start("foreground", false)
	r()
	wg.Wait()
	if len(order) != 2 || order[0] != "foreground" {
		t.Errorf("unexpected order %v", order)
	}
}

func TestLimiterCancel(t *testing.T) {
	l := NewLimiter()
	r, _ := l.Acquire(context.Background(), "a", 1, false)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		_, err := l.Acquire(ctx, "a", 1, true)
		done <- err
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	if err := <-done; err == nil {
		t.Error("expected an error after canceling")
	}
	r()
	// The slot is free again.
	r2, err := l.Acquire(context.Background(), "a", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	r2()
	// The lowest limit of a server applies, and the default for zero.
	r1, _ := l.Acquire(context.Background(), "c", 0, false)
	if h := l.hosts["c"]; h.limit != DefaultConnections {
		t.Errorf("limit %d, want %d", h.limit, DefaultConnections)
	}
	r1()
	r1, _ = l.Acquire(context.Background(), "c", 1, false)
	if h := l.hosts["c"]; h.limit != 1 {
		t.Errorf("limit %d, want 1", h.limit)
	}
	r1()
}

func TestManagerLimited(t *testing.T) {
	f := &fakeFetcher{}
	l := NewLimiter()
	m := NewManager(f, Options{Workers: 4, Limiter: l, Host: "a", Connections: 1, Name: "Test"})
	defer m.Close()
	// While the only connection is taken, nothing loads.
	r, _ := l.Acquire(context.Background(), "a", 1, false)
	m.Request([]geo.TileKey{{Z: 1}, {Z: 1, X: 1}})
	time.Sleep(20 * time.Millisecond)
	if f.calls.Load() != 0 {
		t.Error("expected no downloads while the connection is taken")
	}
	r()
	waitFor(t, func() bool { return m.Loading() == 0 && f.calls.Load() == 2 })
}

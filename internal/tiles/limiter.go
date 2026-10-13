package tiles

import (
	"context"
	"sync"
)

// DefaultConnections is the number of simultaneous downloads from a server if not configured otherwise.
const DefaultConnections = 4

// Limiter limits the simultaneous downloads per server, shared by all tile managers.
// Downloads in the background wait until no others are waiting for the server.
type Limiter struct {
	mu    sync.Mutex
	hosts map[string]*hostSlots
}

// hostSlots are the downloads from a server: running ones, and waiting ones by priority.
type hostSlots struct {
	limit   int
	running int
	// waiting are the waiting downloads, in the foreground (0) and the background (1).
	waiting [2][]chan struct{}
}

// NewLimiter creates a limiter without any downloads.
func NewLimiter() *Limiter {
	return &Limiter{hosts: map[string]*hostSlots{}}
}

// Acquire waits until a download from host may start, and returns the function to call when it is done.
// limit is the number of simultaneous downloads from the host, [DefaultConnections] if zero or less.
// If the layers of a host give different limits, the lowest applies.
// Background downloads wait until no foreground downloads are waiting.
func (l *Limiter) Acquire(ctx context.Context, host string, limit int, background bool) (func(), error) {
	if limit <= 0 {
		limit = DefaultConnections
	}
	l.mu.Lock()
	h, ok := l.hosts[host]
	if !ok {
		h = &hostSlots{limit: limit}
		l.hosts[host] = h
	}
	h.limit = min(h.limit, limit)
	prio := 0
	if background {
		prio = 1
	}
	if h.running < h.limit && len(h.waiting[0]) == 0 && (prio == 0 || len(h.waiting[1]) == 0) {
		h.running++
		l.mu.Unlock()
		return l.releaser(h), nil
	}
	ch := make(chan struct{})
	h.waiting[prio] = append(h.waiting[prio], ch)
	l.mu.Unlock()

	select {
	case <-ch:
		return l.releaser(h), nil
	case <-ctx.Done():
		l.mu.Lock()
		defer l.mu.Unlock()
		for i, c := range h.waiting[prio] {
			if c == ch {
				h.waiting[prio] = append(h.waiting[prio][:i], h.waiting[prio][i+1:]...)
				return nil, ctx.Err()
			}
		}
		// The slot was handed over meanwhile: pass it on.
		h.running--
		l.next(h)
		return nil, ctx.Err()
	}
}

// releaser returns the function that ends a download from a server.
func (l *Limiter) releaser(h *hostSlots) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			h.running--
			l.next(h)
		})
	}
}

// next starts waiting downloads while there are free slots, foreground ones first. The caller must hold the lock.
func (l *Limiter) next(h *hostSlots) {
	for h.running < h.limit {
		prio := 0
		if len(h.waiting[0]) == 0 {
			prio = 1
		}
		if len(h.waiting[prio]) == 0 {
			return
		}
		ch := h.waiting[prio][0]
		h.waiting[prio] = h.waiting[prio][1:]
		h.running++
		close(ch)
	}
}

package tiles

import (
	"context"
	"image"
	"image/draw"
	"sync"

	"github.com/mlange-42/tramp/internal/geo"
)

// metaKeep is the number of decoded meta tiles kept per layer, for the other tiles they contain.
const metaKeep = 6

// metaCache shares the download of a meta tile between the requests for the tiles it contains,
// and keeps the last ones, see [wms.Layer.MetaTiles].
type metaCache struct {
	mu     sync.Mutex
	calls  map[string]*metaCall
	recent []*metaCall
}

// metaCall is a download of a meta tile, by its request URL.
type metaCall struct {
	url  string
	done chan struct{}
	img  image.Image
	err  error
}

// get returns the meta tile with the given URL, from a running or a recent download, or by calling load.
func (c *metaCache) get(ctx context.Context, url string, load func() (image.Image, error)) (image.Image, error) {
	c.mu.Lock()
	if call, ok := c.calls[url]; ok {
		c.mu.Unlock()
		select {
		case <-call.done:
			return call.img, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if c.calls == nil {
		c.calls = map[string]*metaCall{}
	}
	call := &metaCall{url: url, done: make(chan struct{})}
	c.calls[url] = call
	c.mu.Unlock()

	call.img, call.err = load()
	close(call.done)

	c.mu.Lock()
	defer c.mu.Unlock()
	if call.err != nil {
		// Failed downloads are not kept, so that the next request tries again.
		if c.calls[url] == call {
			delete(c.calls, url)
		}
		return nil, call.err
	}
	c.recent = append(c.recent, call)
	if len(c.recent) > metaKeep {
		if old := c.recent[0]; c.calls[old.url] == old {
			delete(c.calls, old.url)
		}
		c.recent = c.recent[1:]
	}
	return call.img, nil
}

// clear drops the kept meta tiles. Running downloads are still shared.
func (c *metaCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, call := range c.recent {
		if c.calls[call.url] == call {
			delete(c.calls, call.url)
		}
	}
	c.recent = nil
}

// cropTile returns the part of a meta tile of n×n tiles starting at first that shows key.
// It works for meta tile images of any size, which servers may deliver.
func cropTile(meta image.Image, first, key geo.TileKey, n int) image.Image {
	b := meta.Bounds()
	col, row := key.X-first.X, key.Y-first.Y
	r := image.Rect(b.Min.X+col*b.Dx()/n, b.Min.Y+row*b.Dy()/n, b.Min.X+(col+1)*b.Dx()/n, b.Min.Y+(row+1)*b.Dy()/n)
	dst := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(dst, dst.Bounds(), meta, r.Min, draw.Src)
	return dst
}

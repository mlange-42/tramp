package tiles

import (
	"context"
	"errors"
	"image"
	"log"
	"mime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mlange-42/tramp/internal/diskcache"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/wms"
	"go.yaml.in/yaml/v3"
)

// WMSFetcher fetches tiles from a WMS or XYZ layer.
type WMSFetcher struct {
	Client *wms.Client
	Layer  *wms.Layer
	// Shade converts the images with [Shade], for hillshade overlays.
	Shade bool
	// Cache stores the downloaded images on disk. Nil for none.
	Cache *diskcache.Cache
	// MaxAge is the age after which a stored image is checked with the server.
	// Older images are still used if the server can't be reached.
	MaxAge time.Duration

	once sync.Once
	// dir is the directory of the layer in the cache, and ext the file extension of the images.
	dir, ext string
}

// LayerInfo is the name of the file in the directory of a layer in the cache that describes the layer.
const LayerInfo = "layer.yaml"

// Fetch implements [Fetcher].
func (f *WMSFetcher) Fetch(ctx context.Context, key geo.TileKey) (image.Image, error) {
	data, err := f.fetch(ctx, key)
	if err != nil {
		return nil, err
	}
	img, err := wms.Decode(data)
	if err != nil {
		return nil, err
	}
	if f.Shade {
		return Shade(img), nil
	}
	return img, nil
}

// fetch returns the image data from the cache, or from the server if it is not stored or too old.
func (f *WMSFetcher) fetch(ctx context.Context, key geo.TileKey) ([]byte, error) {
	if f.Cache == nil {
		d, err := f.Client.GetTileData(ctx, f.Layer, key, time.Time{})
		return d.Body, err
	}
	f.once.Do(f.initCache)

	ck := f.cacheKey(key)
	cached, mod, ok := f.Cache.Get(ck)
	ok = ok && len(cached) > 0
	if ok && time.Since(mod) < f.MaxAge {
		return cached, nil
	}
	var since time.Time
	if ok {
		since = mod
	}
	d, err := f.Client.GetTileData(ctx, f.Layer, key, since)
	if errors.Is(err, wms.ErrNotModified) {
		if err := f.Cache.Touch(ck); err != nil {
			log.Printf("tile cache: %v", err)
		}
		return cached, nil
	}
	if err != nil {
		if ok && ctx.Err() == nil {
			// Offline, or the server fails: the old image is better than none.
			return cached, nil
		}
		return nil, err
	}
	if !d.NoStore {
		if err := f.Cache.Put(ck, d.Body); err != nil {
			log.Printf("tile cache: %v", err)
		}
	}
	return d.Body, nil
}

// initCache determines the directory of the layer in the cache, and writes the description of the layer there.
func (f *WMSFetcher) initCache() {
	// The URL of a tile contains everything that determines the images.
	f.dir = diskcache.Name(f.Layer.Name, f.Layer.TileURL(geo.TileKey{}))
	f.ext = imageExt(f.Layer)
	data, err := yaml.Marshal(f.Layer)
	if err == nil {
		err = f.Cache.Put(f.dir+"/"+LayerInfo, data)
	}
	if err != nil {
		log.Printf("tile cache: %v", err)
	}
}

func (f *WMSFetcher) cacheKey(key geo.TileKey) string {
	return f.dir + "/" + strconv.Itoa(key.Z) + "/" + strconv.Itoa(key.X) + "/" + strconv.Itoa(key.Y) + f.ext
}

// imageExt returns the file extension of the images of a layer, like ".png", or "" if not known.
func imageExt(l *wms.Layer) string {
	if l.IsXYZ() {
		return diskcache.ImageExt(l.URL)
	}
	format := l.Format
	if format == "" {
		format = "image/png"
	}
	switch strings.ToLower(format) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	}
	if exts, err := mime.ExtensionsByType(format); err == nil && len(exts) > 0 {
		return exts[0]
	}
	return ""
}

// Shade converts a grayscale relief image into black with transparency.
// White becomes fully transparent and black fully opaque,
// so that drawing the result darkens the map below like a multiply blend.
func Shade(src image.Image) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := dst.Pix[(y-b.Min.Y)*dst.Stride:]
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := src.At(x, y).RGBA()
			// Luminance of the premultiplied color, relative to its alpha.
			lum := (299*r + 587*g + 114*bl) / 1000
			// Pixels are premultiplied, so black only needs alpha.
			row[(x-b.Min.X)*4+3] = uint8((a - min(lum, a)) >> 8)
		}
	}
	return dst
}

package tiles

import (
	"context"
	"image"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/wms"
)

// WMSFetcher fetches tiles from a WMS layer.
type WMSFetcher struct {
	Client *wms.Client
	Layer  *wms.Layer
	// Shade converts the images with [Shade], for hillshade overlays.
	Shade bool
}

// Fetch implements [Fetcher].
func (f *WMSFetcher) Fetch(ctx context.Context, key geo.TileKey) (image.Image, error) {
	img, err := f.Client.GetMap(ctx, f.Layer, key.Bounds(), geo.TileSize, geo.TileSize)
	if err != nil {
		return nil, err
	}
	if f.Shade {
		return Shade(img), nil
	}
	return img, nil
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

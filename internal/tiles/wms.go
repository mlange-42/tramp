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
}

// Fetch implements [Fetcher].
func (f *WMSFetcher) Fetch(ctx context.Context, key geo.TileKey) (image.Image, error) {
	return f.Client.GetMap(ctx, f.Layer, key.Bounds(), geo.TileSize, geo.TileSize)
}

package tiles

import (
	"image"
	"image/draw"
)

// cropTile returns the tile in column col and row row of a meta tile of n×n tiles.
// It works for meta tile images of any size, which servers may deliver.
func cropTile(meta image.Image, col, row, n int) image.Image {
	b := meta.Bounds()
	r := image.Rect(b.Min.X+col*b.Dx()/n, b.Min.Y+row*b.Dy()/n, b.Min.X+(col+1)*b.Dx()/n, b.Min.Y+(row+1)*b.Dy()/n)
	dst := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(dst, dst.Bounds(), meta, r.Min, draw.Src)
	return dst
}

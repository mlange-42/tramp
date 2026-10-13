package elevation

import (
	"fmt"
	"image"
	"math"

	"github.com/mlange-42/tramp/internal/geo"
)

// grid is a decoded tile: the elevations of its pixels in meters, row by row from the north,
// NaN for transparent pixels without data.
type grid struct {
	size int
	data []float32
}

// bytes returns the memory of the grid, with a small amount for a missing tile.
func (g *grid) bytes() int {
	if g == nil {
		return 64
	}
	return 4*len(g.data) + 64
}

// decode converts a square tile image with the given encoding.
func decode(img image.Image, encoding string) (*grid, error) {
	var conv func(r, g, b uint8) float32
	switch encoding {
	case "", EncodingTerrarium:
		conv = func(r, g, b uint8) float32 {
			return float32(r)*256 + float32(g) + float32(b)/256 - 32768
		}
	case EncodingMapbox:
		conv = func(r, g, b uint8) float32 {
			return float32(int(r)<<16|int(g)<<8|int(b))*0.1 - 10000
		}
	default:
		return nil, fmt.Errorf("unknown elevation encoding %q, expected %s or %s", encoding, EncodingTerrarium, EncodingMapbox)
	}
	bounds := img.Bounds()
	size := bounds.Dx()
	if size == 0 || bounds.Dy() != size {
		return nil, fmt.Errorf("elevation tile of %dx%d pixels, expected a square", bounds.Dx(), bounds.Dy())
	}
	g := &grid{size: size, data: make([]float32, size*size)}
	nan := float32(math.NaN())
	// The common image types are read directly, as image.At is slow.
	switch im := img.(type) {
	case *image.NRGBA:
		for y := range size {
			row := im.Pix[y*im.Stride:]
			for x := range size {
				p := row[4*x : 4*x+4]
				if p[3] == 0 {
					g.data[y*size+x] = nan
					continue
				}
				g.data[y*size+x] = conv(p[0], p[1], p[2])
			}
		}
	case *image.RGBA:
		// Opaque pixels are the same premultiplied, which is all that tiles have.
		for y := range size {
			row := im.Pix[y*im.Stride:]
			for x := range size {
				p := row[4*x : 4*x+4]
				if p[3] == 0 {
					g.data[y*size+x] = nan
					continue
				}
				g.data[y*size+x] = conv(p[0], p[1], p[2])
			}
		}
	default:
		for y := range size {
			for x := range size {
				r, gr, b, a := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
				if a == 0 {
					g.data[y*size+x] = nan
					continue
				}
				g.data[y*size+x] = conv(uint8(r>>8), uint8(gr>>8), uint8(b>>8))
			}
		}
	}
	return g, nil
}

// sample returns the elevation at a position in the tile with key k, interpolated bilinearly
// between the centers of the four nearest pixels. Near the edges of the tile, the edge pixels are used.
func (g *grid) sample(k geo.TileKey, p geo.Point) float64 {
	b := k.Bounds()
	n := float64(g.size)
	// Pixel coordinates, with pixel centers at whole numbers.
	fx := (p.X-b.Min.X)/(b.Max.X-b.Min.X)*n - 0.5
	fy := (b.Max.Y-p.Y)/(b.Max.Y-b.Min.Y)*n - 0.5
	fx = min(max(fx, 0), n-1)
	fy = min(max(fy, 0), n-1)
	x0, y0 := int(fx), int(fy)
	x1, y1 := min(x0+1, g.size-1), min(y0+1, g.size-1)
	tx, ty := fx-float64(x0), fy-float64(y0)
	at := func(x, y int) float64 { return float64(g.data[y*g.size+x]) }
	top := at(x0, y0)*(1-tx) + at(x1, y0)*tx
	bottom := at(x0, y1)*(1-tx) + at(x1, y1)*tx
	return top*(1-ty) + bottom*ty
}

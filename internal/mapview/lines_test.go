package mapview

import (
	"image"
	"testing"

	"gioui.org/op"
	"gioui.org/op/clip"
	"github.com/mlange-42/tramp/internal/geo"
)

func TestAppendLine(t *testing.T) {
	v := View{Zoom: 10, Size: image.Pt(100, 100)}
	res := v.Resolution()
	area := screenRect{minX: 0, minY: 0, maxX: 100, maxY: 100}
	// Points in pixels from the view center.
	px := func(pts ...[2]float64) []geo.Point {
		out := make([]geo.Point, len(pts))
		for i, p := range pts {
			out[i] = geo.Point{X: p[0] * res, Y: -p[1] * res}
		}
		return out
	}

	for _, c := range []struct {
		name string
		pts  []geo.Point
		want int
	}{
		{"single point", px([2]float64{0, 0}), 0},
		{"two points", px([2]float64{0, 0}, [2]float64{10, 0}), 1},
		// Steps below minStep are merged, the last point is always drawn.
		{"dense", px([2]float64{0, 0}, [2]float64{0.5, 0}, [2]float64{1, 0}, [2]float64{1.5, 0}, [2]float64{2, 0}, [2]float64{2.2, 0}), 2},
		// Segments beyond the same edge of the area are skipped.
		{"outside", px([2]float64{0, 0}, [2]float64{100, 0}, [2]float64{100, 10}, [2]float64{0, 10}), 2},
		// A segment crossing the area is kept even if both ends are outside.
		{"crossing", px([2]float64{-100, 0}, [2]float64{100, 0}), 1},
	} {
		var ops op.Ops
		var path clip.Path
		path.Begin(&ops)
		if n := appendLine(&path, &v, area, c.pts); n != c.want {
			t.Errorf("%s: expected %d segments, got %d", c.name, c.want, n)
		}
		path.End()
	}
}

func TestNewPolyline(t *testing.T) {
	l := NewPolyline([]geo.Point{{X: 1, Y: 2}, {X: -1, Y: 5}})
	if l.Bounds != (geo.Rect{Min: geo.Point{X: -1, Y: 2}, Max: geo.Point{X: 1, Y: 5}}) {
		t.Errorf("unexpected bounds %v", l.Bounds)
	}
}

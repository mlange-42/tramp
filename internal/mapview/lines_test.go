package mapview

import (
	"image"
	"image/color"
	"math"
	"slices"
	"testing"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/mlange-42/tramp/internal/geo"
)

func TestWalkLine(t *testing.T) {
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
		n := 0
		walkLine(&v, area, c.pts, func(_, _ f32.Point, _ int) { n++ })
		if n != c.want {
			t.Errorf("%s: expected %d segments, got %d", c.name, c.want, n)
		}
	}

	// Merged segments report the last original segment they cover.
	var segs []int
	dense := px([2]float64{0, 0}, [2]float64{0.5, 0}, [2]float64{1, 0}, [2]float64{1.5, 0}, [2]float64{2, 0}, [2]float64{2.2, 0})
	walkLine(&v, area, dense, func(_, _ f32.Point, seg int) { segs = append(segs, seg) })
	if !slices.Equal(segs, []int{2, 4}) {
		t.Errorf("unexpected segments %v", segs)
	}
}

func TestStrokes(t *testing.T) {
	var s strokes
	s.add(f32.Pt(0, 0), f32.Pt(1, 0))
	s.add(f32.Pt(1, 0), f32.Pt(2, 0))
	s.add(f32.Pt(5, 0), f32.Pt(6, 0))
	if !slices.Equal(s.starts, []int{0, 3}) || len(s.pts) != 5 {
		t.Errorf("unexpected runs %v %v", s.starts, s.pts)
	}
	s.reset()
	if len(s.pts) != 0 || len(s.starts) != 0 {
		t.Errorf("expected empty strokes after reset")
	}
}

func TestColoringIndex(t *testing.T) {
	c := Coloring{Min: 10, Max: 20, Colors: make([]color.NRGBA, 4)}
	for _, tc := range []struct {
		v    float64
		want int
	}{{5, 0}, {10, 0}, {12.4, 0}, {12.6, 1}, {17.6, 3}, {20, 3}, {30, 3}, {math.NaN(), 0}} {
		if got := c.index(tc.v); got != tc.want {
			t.Errorf("index(%v) = %d, want %d", tc.v, got, tc.want)
		}
	}
	same := Coloring{Min: 1, Max: 1, Colors: make([]color.NRGBA, 4)}
	if got := same.index(1); got != 0 {
		t.Errorf("expected 0 for empty range, got %d", got)
	}
}

func TestNewPolyline(t *testing.T) {
	l := NewPolyline([]geo.Point{{X: 1, Y: 2}, {X: -1, Y: 5}})
	if l.Bounds != (geo.Rect{Min: geo.Point{X: -1, Y: 2}, Max: geo.Point{X: 1, Y: 5}}) {
		t.Errorf("unexpected bounds %v", l.Bounds)
	}
}

func TestLinesRebuild(t *testing.T) {
	v := View{Zoom: 10, Size: image.Pt(200, 100)}
	res := v.Resolution()
	small := NewPolyline([]geo.Point{{X: 0, Y: 0}, {X: 50 * res, Y: 20 * res}})
	large := NewPolyline([]geo.Point{{X: 0, Y: 0}, {X: 5000 * res, Y: 0}})

	layoutAt := func(l *Lines, x float64) {
		var ops op.Ops
		v.Center = geo.Point{X: x * res}
		l.Layout(layout.Context{Ops: &ops, Constraints: layout.Exact(v.Size)}, &v)
	}
	for _, c := range []struct {
		name    string
		line    Polyline
		rebuild bool
	}{
		// Everything is in the drawing, so panning never rebuilds it.
		{"small", small, false},
		// Panning by more than half the view rebuilds a drawing that was cut.
		{"large", large, true},
	} {
		var l Lines
		l.Width = 3
		l.Set([]LineGroup{{Lines: []Polyline{c.line}}}, nil)
		layoutAt(&l, 0)
		if l.complete == c.rebuild {
			t.Errorf("%s: unexpected complete %v", c.name, l.complete)
		}
		layoutAt(&l, 150)
		if rebuilt := l.origin.X != 0; rebuilt != c.rebuild {
			t.Errorf("%s: expected rebuild %v, got %v", c.name, c.rebuild, rebuilt)
		}
	}
}

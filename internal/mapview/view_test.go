package mapview

import (
	"image"
	"math"
	"testing"

	"github.com/mlange-42/tramp/internal/geo"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestViewRoundTrip(t *testing.T) {
	v := View{Center: geo.Point{X: 1000, Y: 2000}, Zoom: 10.5, Size: image.Pt(800, 600)}
	x, y := v.ToScreen(v.Center)
	if !near(x, 400) || !near(y, 300) {
		t.Errorf("center should map to view center, got %f, %f", x, y)
	}
	p := v.ToMap(123, 456)
	x, y = v.ToScreen(p)
	if !near(x, 123) || !near(y, 456) {
		t.Errorf("round trip gave %f, %f", x, y)
	}
	b := v.Bounds()
	if b.Min.X >= b.Max.X || b.Min.Y >= b.Max.Y {
		t.Errorf("invalid bounds %v", b)
	}
}

func TestViewZoomAtKeepsAnchor(t *testing.T) {
	v := View{Center: geo.Point{X: 1000, Y: 2000}, Zoom: 10, Size: image.Pt(800, 600)}
	before := v.ToMap(100, 50)
	v.ZoomAt(100, 50, 1.4, 0, 20)
	after := v.ToMap(100, 50)
	if !near(before.X, after.X) || !near(before.Y, after.Y) {
		t.Errorf("anchor moved from %v to %v", before, after)
	}
	if v.Zoom != 11.5 {
		t.Errorf("unexpected zoom %f", v.Zoom)
	}
	v.ZoomAt(0, 0, 100, 0, 18)
	if v.Zoom != 18 {
		t.Errorf("zoom not clamped: %f", v.Zoom)
	}
}

func TestViewPan(t *testing.T) {
	v := View{Zoom: 5, Size: image.Pt(100, 100)}
	p := v.ToMap(60, 70)
	v.Pan(-10, -20)
	x, y := v.ToScreen(p)
	if !near(x, 50) || !near(y, 50) {
		t.Errorf("point should be in center after pan, got %f, %f", x, y)
	}
}

func TestViewFit(t *testing.T) {
	v := View{Size: image.Pt(800, 400)}
	r := geo.Rect{Min: geo.Point{X: -2000, Y: 1000}, Max: geo.Point{X: 2000, Y: 2500}}
	v.Fit(r, 0, 30)
	if !near(v.Center.X, 0) || !near(v.Center.Y, 1750) {
		t.Errorf("unexpected center %v", v.Center)
	}
	b := v.Bounds()
	if b.Min.X > r.Min.X || b.Max.X < r.Max.X || b.Min.Y > r.Min.Y || b.Max.Y < r.Max.Y {
		t.Errorf("bounds %v don't fit %v", b, r)
	}
	if v.Zoom != SnapZoom(v.Zoom) {
		t.Errorf("zoom %f is not a multiple of %f", v.Zoom, ZoomStep)
	}
	v.Zoom += ZoomStep
	b = v.Bounds()
	if b.Min.X <= r.Min.X && b.Max.X >= r.Max.X {
		t.Errorf("zoom %f is not the largest that fits", v.Zoom-ZoomStep)
	}
}

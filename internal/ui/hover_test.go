package ui

import (
	"math"
	"testing"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/mapview"
)

func TestLocate(t *testing.T) {
	dist := [][]float64{{0, 10, 20}, {20, 40}}
	for _, tc := range []struct {
		d    float64
		i, j int
		t    float64
		ok   bool
	}{
		{0, 0, 0, 0, true},
		{5, 0, 0, 0.5, true},
		{15, 0, 1, 0.5, true},
		{20, 0, 1, 1, true},
		{30, 1, 0, 0.5, true},
		{40, 1, 0, 1, true},
		{41, 0, 0, 0, false},
		{-1, 0, 0, 0, false},
	} {
		i, j, tt, ok := locate(dist, tc.d)
		if i != tc.i || j != tc.j || tt != tc.t || ok != tc.ok {
			t.Errorf("locate(%v) = %d, %d, %v, %v, want %d, %d, %v, %v", tc.d, i, j, tt, ok, tc.i, tc.j, tc.t, tc.ok)
		}
	}
}

func TestNearestSegment(t *testing.T) {
	lines := []mapview.Polyline{
		mapview.NewPolyline([]geo.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}}),
		mapview.NewPolyline([]geo.Point{{X: 20, Y: 0}, {X: 30, Y: 0}}),
	}
	i, j, tt, ok := nearestSegment(lines, geo.Point{X: 9, Y: 4}, 2)
	if !ok || i != 0 || j != 1 || tt != 0.4 {
		t.Errorf("got %d, %d, %v, %v, want 0, 1, 0.4, true", i, j, tt, ok)
	}
	i, j, tt, ok = nearestSegment(lines, geo.Point{X: 25, Y: -1}, 2)
	if !ok || i != 1 || j != 0 || tt != 0.5 {
		t.Errorf("got %d, %d, %v, %v, want 1, 0, 0.5, true", i, j, tt, ok)
	}
	if _, _, _, ok := nearestSegment(lines, geo.Point{X: 15, Y: 5}, 2); ok {
		t.Error("found a segment out of the radius")
	}
}

func TestHoverAt(t *testing.T) {
	lines := []mapview.Polyline{mapview.NewPolyline([]geo.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}})}
	c := chart{chartData: chartData{xs: [][]float64{{0, 10, 20}}, vals: [][]float64{{1, 3}}}}
	// Point values are 1, 2, 3.
	h := c.hoverAt(lines, 0, 1, 0.5)
	if !h.valid || h.x != 15 || h.pos != (geo.Point{X: 10, Y: 5}) || h.value != 2.5 {
		t.Errorf("got %+v", h)
	}
	c.vals = [][]float64{nil}
	if h := c.hoverAt(lines, 0, 0, 0.5); !math.IsNaN(h.value) {
		t.Errorf("got value %v without values, want NaN", h.value)
	}
}

func TestFormatValue(t *testing.T) {
	info := &metricInfo{unit: "%", scale: 1, format: "%+.0f"}
	for _, tc := range []struct {
		v    float64
		want string
	}{{-0.2, "0 %"}, {0.2, "0 %"}, {3, "+3 %"}, {-3, "-3 %"}} {
		if got := info.formatValue(tc.v); got != tc.want {
			t.Errorf("formatValue(%v) = %q, want %q", tc.v, got, tc.want)
		}
	}
}

func TestPreviewHover(t *testing.T) {
	a, f, gtx := editorApp(t)
	a.setTool(routeTool)
	route := f.data.Routes[0]
	end := route.Points[1].Pos
	a.mapView.Hover, a.mapView.HoverValid = geo.ToMercator(end), true
	if _, ok := a.previewHover(gtx); ok {
		t.Fatalf("expected no preview before drawing")
	}

	// Clicking the end continues the route. At the end, the preview is the length of the route.
	click(a, gtx, end)
	a.updateHover(gtx)
	if h := a.hover; !h.valid || !h.preview || math.Abs(h.x-route.Length()) > 1e-6 {
		t.Fatalf("expected the route length at the end, got %+v", h)
	}

	// Further on, the preview adds the distance to the pointer.
	to := geo.LonLat{Lon: 12.1, Lat: 51.1}
	a.mapView.Hover = geo.ToMercator(to)
	a.updateHover(gtx)
	want := route.Length() + geo.Distance(end, to)
	if h := a.hover; !h.valid || math.Abs(h.x-want) > 1e-6 || math.Abs(h.pos.X-a.mapView.Hover.X) > 1e-9 {
		t.Errorf("expected length %v at the pointer, got %+v", want, h)
	}
	if p := a.hoverParts(); len(p) != 1 || p[0].icon != iconDistance || p[0].text != formatDistance(want) {
		t.Errorf("unexpected hover parts %v", p)
	}
}

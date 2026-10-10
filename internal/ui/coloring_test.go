package ui

import (
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/track"
)

func TestGradient(t *testing.T) {
	g := Gradient{Stops: []color.NRGBA{{R: 0, A: 255}, {R: 100, A: 255}, {R: 200, A: 255}}}
	for _, c := range []struct {
		t    float64
		want uint8
	}{{-1, 0}, {0, 0}, {0.25, 50}, {0.5, 100}, {0.75, 150}, {1, 200}, {2, 200}} {
		if got := g.At(c.t); got.R != c.want || got.A != 255 {
			t.Errorf("At(%v) = %v, want R=%d", c.t, got, c.want)
		}
	}
	if b := g.Bins(2); b[0].R != 50 || b[1].R != 150 {
		t.Errorf("unexpected bins %v", b)
	}
	a := &App{}
	a.initColoring(nil, "", nil)
	for _, m := range metrics {
		if m.metric != track.NoMetric && a.gradientIndex(m.gradient) < 0 {
			t.Errorf("unknown default gradient %q for %s", m.gradient, m.name)
		}
	}
}

func TestValueRange(t *testing.T) {
	vals := make([]float64, 101)
	for i := range vals {
		vals[i] = float64(i)
	}
	// An outlier doesn't stretch the range.
	vals[100] = 1e6
	if lo, hi, ok := valueRange(vals, false); !ok || lo != 2 || hi != 98 {
		t.Errorf("unexpected range %v %v %v", lo, hi, ok)
	}
	if lo, hi, _ := valueRange([]float64{-3, -1, 0, 1, 5}, true); lo != -5 || hi != 5 {
		t.Errorf("unexpected symmetric range %v %v", lo, hi)
	}
	if lo, hi, _ := valueRange([]float64{7, 7}, false); lo != 6 || hi != 8 {
		t.Errorf("unexpected range for equal values %v %v", lo, hi)
	}
	if _, _, ok := valueRange(nil, false); ok {
		t.Error("expected no range without values")
	}
}

func TestColoringState(t *testing.T) {
	a := &App{}
	a.initColoring(nil, "slope", map[string]string{"speed": "Plasma", "elevation": "unknown"})
	if a.colorMetric() != track.SlopeMetric || a.gradients[a.gradientSel.Selected()].Name != "Blue–Red" {
		t.Errorf("unexpected coloring %v %d", a.colorMetric(), a.gradientSel.Selected())
	}
	colorBy, grads := a.coloringState()
	want := map[string]string{"speed": "Plasma", "elevation": "Viridis", "slope": "Blue–Red"}
	if colorBy != "slope" || len(grads) != len(want) {
		t.Errorf("unexpected state %q %v", colorBy, grads)
	}
	for k, v := range want {
		if grads[k] != v {
			t.Errorf("%s: got %q, want %q", k, grads[k], v)
		}
	}

	a.initColoring(nil, "", nil)
	if a.colorMetric() != track.NoMetric {
		t.Errorf("expected no coloring by default")
	}
	if l := a.newLegend([]mapview.LineGroup{{Values: [][]float64{{1, 2}}}}); l != nil {
		t.Errorf("expected no legend without coloring")
	}
}

func TestLegend(t *testing.T) {
	a := &App{style: DefaultStyle()}
	a.initColoring(nil, "speed", nil)
	if l := a.newLegend([]mapview.LineGroup{{}}); l != nil {
		t.Errorf("expected no legend without values")
	}
	l := a.newLegend([]mapview.LineGroup{{Values: [][]float64{{1, 2, math.NaN()}, nil}}, {Values: [][]float64{{3, 10}}}})
	if l == nil || l.min != 1 || l.max != 10 {
		t.Fatalf("unexpected legend %+v", l)
	}
	if s := l.label(10); s != "36" {
		t.Errorf("expected 36 km/h, got %q", s)
	}
	if c := l.coloring(); c.Min != 1 || c.Max != 10 || len(c.Colors) != colorBins {
		t.Errorf("unexpected coloring %+v", c)
	}
}

func TestLineValues(t *testing.T) {
	data, err := track.ReadFile(writeTemp(t, "mixed.gpx", testGPX))
	if err != nil {
		t.Fatal(err)
	}
	f := newOpenFile("mixed.gpx", data)
	trk, rte, wpts := f.items[0], f.items[1], f.items[2]

	vals := trk.lineValues(track.SpeedMetric)
	if len(vals) != 1 || len(vals[0]) != 3 {
		t.Fatalf("unexpected track speeds %v", vals)
	}
	if again := trk.lineValues(track.SpeedMetric); &again[0] != &vals[0] {
		t.Errorf("expected cached values")
	}
	// The route has no times, the waypoints no lines.
	if v := rte.lineValues(track.SpeedMetric); v != nil {
		t.Errorf("expected no route speeds, got %v", v)
	}
	if v := wpts.lineValues(track.ElevationMetric); v != nil {
		t.Errorf("expected no waypoint values, got %v", v)
	}
	if v := trk.lineValues(track.NoMetric); v != nil {
		t.Errorf("expected nil for no metric")
	}
}

func TestGradientConfig(t *testing.T) {
	def := DefaultGradients()
	grads, err := def.Parse()
	if err != nil || len(grads) != len(def.Gradients) {
		t.Fatalf("built-in gradients: %d, %v", len(grads), err)
	}

	cfg := GradientConfig{Gradients: []GradientDef{
		{Name: "Mine", Colors: []string{"#000000", "#ff0000"}},
		{Name: "", Colors: []string{"#000000", "#ff0000"}},
		{Name: "Short", Colors: []string{"#000000"}},
		{Name: "Bad", Colors: []string{"#000000", "red"}},
		{Name: "Mine", Colors: []string{"#000000", "#00ff00"}},
	}}
	grads, err = cfg.Parse()
	if len(grads) != 1 || grads[0].Name != "Mine" || grads[0].Stops[1] != (color.NRGBA{R: 255, A: 255}) {
		t.Errorf("unexpected gradients %+v", grads)
	}
	if err == nil || strings.Count(err.Error(), "gradient ") != 4 {
		t.Errorf("expected 4 errors, got %v", err)
	}
}

func TestCustomGradients(t *testing.T) {
	a := &App{}
	own := []Gradient{{Name: "A", Stops: []color.NRGBA{{A: 255}, {R: 255, A: 255}}}, {Name: "B", Stops: []color.NRGBA{{A: 255}, {B: 255, A: 255}}}}
	// Missing default and saved gradients fall back to the first one.
	a.initColoring(own, "speed", map[string]string{"slope": "B", "elevation": "Viridis"})
	if a.gradientSel.Selected() != 0 || len(a.gradientNames) != 2 {
		t.Errorf("unexpected selection %d %v", a.gradientSel.Selected(), a.gradientNames)
	}
	_, grads := a.coloringState()
	if grads["slope"] != "B" || grads["elevation"] != "A" || grads["speed"] != "A" {
		t.Errorf("unexpected gradients %v", grads)
	}
}

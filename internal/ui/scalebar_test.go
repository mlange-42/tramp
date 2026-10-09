package ui

import "testing"

func TestScaleLength(t *testing.T) {
	for _, c := range []struct {
		metersPerPx float64
		maxPx       int
		meters      float64
		segments    int
	}{
		{10, 180, 1000, 5},
		{10, 250, 2000, 4},
		{10, 600, 5000, 5},
		{0.5, 180, 50, 5},
		{1000, 180, 100000, 5},
		{0.001, 180, 0, 0},
	} {
		m, n := scaleLength(c.metersPerPx, c.maxPx)
		if m != c.meters || n != c.segments {
			t.Errorf("scaleLength(%v, %d) = %v, %d; want %v, %d", c.metersPerPx, c.maxPx, m, n, c.meters, c.segments)
		}
	}
	for m, want := range map[float64]string{50: "50 m", 500: "500 m", 1000: "1 km", 20000: "20 km"} {
		if got := formatScale(m); got != want {
			t.Errorf("formatScale(%v) = %q, want %q", m, got, want)
		}
	}
}

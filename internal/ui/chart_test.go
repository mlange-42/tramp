package ui

import (
	"math"
	"slices"
	"testing"
)

func TestColumnMeans(t *testing.T) {
	// Two lines over 0-40 m: values 1 and 3 on the first, a gap, and 5 on the second.
	dist := [][]float64{{0, 10, 20}, {20, 30, 40}}
	vals := [][]float64{{1, 3}, {math.NaN(), 5}}
	got := columnMeans(dist, vals, 40, 4)
	want := []float64{1, 3, math.NaN(), 5}
	for i := range want {
		if got[i] != want[i] && !(math.IsNaN(got[i]) && math.IsNaN(want[i])) {
			t.Errorf("column %d: got %v, want %v", i, got[i], want[i])
		}
	}

	// Segments within a column are weighted by length.
	got = columnMeans([][]float64{{0, 1, 4}}, [][]float64{{2, 6}}, 4, 1)
	if got[0] != 5 {
		t.Errorf("weighted mean: got %v, want 5", got[0])
	}
}

func TestNiceStep(t *testing.T) {
	for _, tc := range []struct{ raw, want float64 }{
		{0.7, 1}, {1, 1}, {1.2, 2}, {3, 5}, {7, 10}, {120, 200}, {2000, 2000}, {0, 1},
	} {
		if got := niceStep(tc.raw); got != tc.want {
			t.Errorf("niceStep(%v) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestTicks(t *testing.T) {
	if got := ticks(-15, 25, 10); !slices.Equal(got, []float64{-10, 0, 10, 20}) {
		t.Errorf("got %v", got)
	}
	if got := ticks(0, 20, 10); !slices.Equal(got, []float64{0, 10, 20}) {
		t.Errorf("got %v", got)
	}
}

func TestFormatTick(t *testing.T) {
	for _, tc := range []struct {
		m, step float64
		want    string
	}{
		{0, 1000, "0"}, {500, 500, "500 m"}, {5000, 5000, "5 km"}, {12000, 2000, "12 km"},
	} {
		if got := formatTick(tc.m, tc.step); got != tc.want {
			t.Errorf("formatTick(%v, %v) = %q, want %q", tc.m, tc.step, got, tc.want)
		}
	}
}

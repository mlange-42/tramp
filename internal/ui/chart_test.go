package ui

import (
	"math"
	"slices"
	"testing"
)

func TestColumnMeans(t *testing.T) {
	// Two lines over 0-40 m: values 1 and 3 on the first, a gap, and 5 on the second.
	// Point values are 1, 2, 3 on the first line, and 5, 5 at the ends of the second segment.
	dist := [][]float64{{0, 10, 20}, {20, 30, 40}}
	vals := [][]float64{{1, 3}, {math.NaN(), 5}}
	got := columnMeans(dist, vals, 0, 40, 4)
	want := []float64{1.5, 2.5, math.NaN(), 5}
	for i := range want {
		if got[i] != want[i] && !(math.IsNaN(got[i]) && math.IsNaN(want[i])) {
			t.Errorf("column %d: got %v, want %v", i, got[i], want[i])
		}
	}

	// Segments within a column are weighted by length: point values 2, 4, 6,
	// so segment means 3 over 1 m and 5 over 3 m.
	got = columnMeans([][]float64{{0, 1, 4}}, [][]float64{{2, 6}}, 0, 4, 1)
	if got[0] != 4.5 {
		t.Errorf("weighted mean: got %v, want 4.5", got[0])
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
		{1500, 500, "1.5 km"}, {12050, 50, "12.05 km"}, {12100, 100, "12.1 km"},
	} {
		if got := formatTick(tc.m, tc.step); got != tc.want {
			t.Errorf("formatTick(%v, %v) = %q, want %q", tc.m, tc.step, got, tc.want)
		}
	}
}

func TestColumnMeansRange(t *testing.T) {
	// Values 1, 2, 3, 4 over 0-40 m, shown from 15 to 35 m in two columns.
	// Point values are 1, 1.5, 2.5, 3.5, 4, so the line is 0.5 + x/10 from 10 to 30 m,
	// and rises from 3.5 to 4 between 30 and 40 m.
	dist := [][]float64{{0, 10, 20, 30, 40}}
	vals := [][]float64{{1, 2, 3, 4}}
	got := columnMeans(dist, vals, 15, 35, 2)
	want := []float64{2.5, (3.25 + 3.625) / 2}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Errorf("column %d: got %v, want %v", i, got[i], want[i])
		}
	}

	// Zoomed into a single segment, the value rises linearly instead of showing a step.
	got = columnMeans(dist, vals, 10, 20, 2)
	if want := []float64{1.75, 2.25}; math.Abs(got[0]-want[0]) > 1e-9 || math.Abs(got[1]-want[1]) > 1e-9 {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestChartRange(t *testing.T) {
	c := chart{chartData: chartData{total: 1000}}
	c.setRange(-100, 200)
	if c.from != 0 || c.to != 200 {
		t.Errorf("range not shifted into the item: %v-%v", c.from, c.to)
	}
	c.setRange(900, 200)
	if c.from != 800 || c.to != 1000 {
		t.Errorf("range not shifted into the item: %v-%v", c.from, c.to)
	}
	c.setRange(100, 5000)
	if c.from != 0 || c.to != 1000 {
		t.Errorf("zoomed out beyond the item: %v-%v", c.from, c.to)
	}
	c.setRange(500, 1)
	if c.to-c.from != chartMinSpan {
		t.Errorf("zoomed in beyond the minimum span: %v-%v", c.from, c.to)
	}

	// Short items can't be zoomed in.
	c = chart{chartData: chartData{total: 10}}
	c.setRange(2, 1)
	if c.from != 0 || c.to != 10 {
		t.Errorf("short item: %v-%v", c.from, c.to)
	}
}

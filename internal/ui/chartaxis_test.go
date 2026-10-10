package ui

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/track"
)

func TestChartAxisByKey(t *testing.T) {
	for i, ax := range chartAxes {
		if got := chartAxisByKey(ax.key); got != chartAxis(i) {
			t.Errorf("chartAxisByKey(%q) = %v, want %v", ax.key, got, i)
		}
	}
	if got := chartAxisByKey("foo"); got != distanceAxis {
		t.Errorf("unknown key: got %v, want distance", got)
	}
}

func TestNiceTimeStep(t *testing.T) {
	for _, tc := range []struct{ raw, want float64 }{
		{0.5, 1}, {3, 5}, {11, 15}, {40, 60}, {200, 300}, {1000, 1800}, {4000, 7200}, {30000, 43200},
		{90000, 2 * 86400},
	} {
		if got := niceTimeStep(tc.raw); got != tc.want {
			t.Errorf("niceTimeStep(%v) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestFormatElapsed(t *testing.T) {
	if got := formatElapsed(3725, true); got != "1:02:05" {
		t.Errorf("got %q, want 1:02:05", got)
	}
	if got := formatElapsed(3725, false); got != "1:02" {
		t.Errorf("got %q, want 1:02", got)
	}
	if got := formatElapsed(26*3600, false); got != "26:00" {
		t.Errorf("got %q, want 26:00", got)
	}
}

func TestClockTicks(t *testing.T) {
	// Ticks are at round times of day, not at round times since the start.
	start := time.Date(2026, 10, 7, 9, 7, 30, 0, time.Local)
	c := chart{chartData: chartData{xAxis: clockTimeAxis, start: start}, from: 0, to: 3600}
	xs, labels := c.xTicks(1000)
	if want := []float64{1350, 3150}; !slices.Equal(xs, want) {
		t.Errorf("got ticks %v, want %v", xs, want)
	}
	if want := []string{"09:30", "10:00"}; !slices.Equal(labels, want) {
		t.Errorf("got labels %v, want %v", labels, want)
	}
}

func TestItemTimes(t *testing.T) {
	t0 := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	it := &fileItem{points: [][]track.Point{
		{{Time: t0}, {Time: t0.Add(10 * time.Second)}},
		// Times going back are kept at the latest time so far.
		{{Time: t0.Add(5 * time.Second)}, {Time: t0.Add(20 * time.Second)}},
	}}
	times, start := it.times()
	if !start.Equal(t0) || !slices.Equal(times[0], []float64{0, 10}) || !slices.Equal(times[1], []float64{10, 20}) {
		t.Errorf("got %v, %v", times, start)
	}

	it = &fileItem{points: [][]track.Point{{{Time: t0}, {}}}}
	if times, _ := it.times(); times != nil {
		t.Errorf("got times %v for a point without time, want nil", times)
	}
}

func TestLineValuesGaps(t *testing.T) {
	t0 := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	var pts []track.Point
	for _, s := range []int{0, 1, 2, 300, 301} {
		pts = append(pts, track.Point{Ele: float64(s), Time: t0.Add(time.Duration(s) * time.Second)})
	}
	it := &fileItem{points: [][]track.Point{pts}}
	vals := it.lineValues(track.ElevationMetric)
	for j, v := range vals[0] {
		if gap := j == 2; gap != math.IsNaN(v) {
			t.Errorf("segment %d: got %v, gap %v", j, v, gap)
		}
	}
}

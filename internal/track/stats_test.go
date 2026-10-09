package track

import (
	"math"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
)

func TestTrackStats(t *testing.T) {
	t0 := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	pt := func(lat float64, sec int) Point {
		return Point{Pos: geo.LonLat{Lon: 12, Lat: lat}, Ele: math.NaN(), Time: t0.Add(time.Duration(sec) * time.Second)}
	}
	tr := Track{Segments: []Segment{
		{Points: []Point{pt(51, 0), pt(51.001, 1), pt(51.002, 2), pt(51.001, -1)}},
		{Points: []Point{pt(51.1, 3600), pt(51.1, 3602), {Pos: geo.LonLat{Lon: 12, Lat: 51.1}, Ele: math.NaN()}}},
	}}
	if n := tr.NumPoints(); n != 7 {
		t.Errorf("expected 7 points, got %d", n)
	}
	// 3 steps of 0.001° latitude, the gap between segments is not counted.
	if l := tr.Length(); math.Abs(l-3*111.195) > 0.1 {
		t.Errorf("unexpected length %f", l)
	}
	start, end := tr.TimeSpan()
	if !start.Equal(t0.Add(-time.Second)) || !end.Equal(t0.Add(3602*time.Second)) {
		t.Errorf("unexpected time span %v - %v", start, end)
	}
	// Positive steps are 1, 1, 2 s; the backwards step and the point without time are skipped.
	if dt := tr.Interval(); dt != time.Second {
		t.Errorf("expected interval 1s, got %v", dt)
	}

	var empty Track
	if start, end := empty.TimeSpan(); !start.IsZero() || !end.IsZero() || empty.Interval() != 0 || empty.Length() != 0 {
		t.Error("expected zero stats for empty track")
	}

	r := Route{Points: []Waypoint{{Pos: geo.LonLat{Lon: 12, Lat: 51}}, {Pos: geo.LonLat{Lon: 12, Lat: 52}}}}
	if l := r.Length(); math.Abs(l-111_195) > 10 {
		t.Errorf("unexpected route length %f", l)
	}
}

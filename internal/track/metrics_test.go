package track

import (
	"math"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
)

// line returns points going north, n m apart with the given elevation step, every dt.
func line(n int, step, eleStep float64, dt time.Duration) []Point {
	t0 := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	degPerMeter := 1 / 111_195.0
	pts := make([]Point, n)
	for i := range pts {
		pts[i] = Point{
			Pos:  geo.LonLat{Lon: 12, Lat: 51 + float64(i)*step*degPerMeter},
			Ele:  100 + float64(i)*eleStep,
			Time: t0.Add(time.Duration(i) * dt),
		}
	}
	return pts
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestSpeedValues(t *testing.T) {
	// 5 m per second.
	pts := line(30, 5, 0, time.Second)
	vals := SegmentValues(pts, SpeedMetric)
	if len(vals) != 29 {
		t.Fatalf("expected 29 values, got %d", len(vals))
	}
	for i, v := range vals {
		if !near(v, 5, 0.01) {
			t.Errorf("segment %d: expected 5 m/s, got %f", i, v)
		}
	}

	// A step back in time gives no value for that segment, and limits the window of its neighbors.
	pts[10].Time = pts[9].Time.Add(-time.Second)
	vals = SegmentValues(pts, SpeedMetric)
	if !math.IsNaN(vals[9]) || math.IsNaN(vals[10]) || math.IsNaN(vals[8]) {
		t.Errorf("unexpected values around backwards time: %v", vals[7:12])
	}

	// No times, no speed.
	for i := range pts {
		pts[i].Time = time.Time{}
	}
	if vals := SegmentValues(pts, SpeedMetric); vals != nil {
		t.Errorf("expected nil without times, got %v", vals)
	}
}

func TestElevationValues(t *testing.T) {
	pts := line(3, 10, 2, time.Second)
	pts[2].Ele = math.NaN()
	vals := SegmentValues(pts, ElevationMetric)
	if vals[0] != 101 || vals[1] != 102 {
		t.Errorf("unexpected elevations %v", vals)
	}
	for i := range pts {
		pts[i].Ele = math.NaN()
	}
	if vals := SegmentValues(pts, ElevationMetric); vals != nil {
		t.Errorf("expected nil without elevation, got %v", vals)
	}
}

func TestSlopeValues(t *testing.T) {
	// 10 m apart, 0.5 m up: 5%.
	pts := line(50, 10, 0.5, time.Second)
	for i, v := range SegmentValues(pts, SlopeMetric) {
		if !near(v, 5, 0.01) {
			t.Errorf("segment %d: expected 5%%, got %f", i, v)
		}
	}
	// Standing still gives no slope.
	still := line(5, 0, 1, time.Second)
	if vals := SegmentValues(still, SlopeMetric); vals != nil {
		t.Errorf("expected nil when standing still, got %v", vals)
	}
	if vals := SegmentValues(pts[:1], SlopeMetric); vals != nil {
		t.Errorf("expected nil for a single point")
	}
	if vals := SegmentValues(pts, NoMetric); vals != nil {
		t.Errorf("expected nil for no metric")
	}
}

package track

import (
	"math"
	"testing"

	"github.com/mlange-42/tramp/internal/geo"
)

func TestDensify(t *testing.T) {
	// 0.01° of latitude is about 1112 m.
	pts := []geo.LonLat{{Lon: 12, Lat: 51}, {Lon: 12, Lat: 51.01}, {Lon: 12, Lat: 51.01}}
	res := Densify(pts, 100)
	// 12 steps for the first line, and one for the second of zero length.
	if len(res) != 1+12+1 {
		t.Fatalf("expected 14 positions, got %d", len(res))
	}
	if res[0] != pts[0] || res[12] != pts[1] || res[13] != pts[2] {
		t.Errorf("expected the points kept, got %v", res)
	}
	for i := 1; i < len(res); i++ {
		if d := geo.Distance(res[i-1], res[i]); d > 100 {
			t.Errorf("positions %d and %d are %.0f m apart", i-1, i, d)
		}
	}
	if got := Densify(pts, 0); len(got) != 3 {
		t.Errorf("expected the points without spacing, got %v", got)
	}
	if got := Densify(nil, 100); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestClimb(t *testing.T) {
	nan := math.NaN()
	for _, tc := range []struct {
		eles     []float64
		up, down float64
	}{
		{nil, 0, 0},
		{[]float64{100, 110, 105, 120}, 25, 5},
		// Noise below the threshold is ignored.
		{[]float64{100, 101, 100, 101, 100, 101}, 0, 0},
		// Steady small steps add up once they pass the threshold.
		{[]float64{100, 101, 102, 103, 104, 105, 106}, 6, 0},
		{[]float64{nan, 100, nan, 90, 95}, 5, 10},
	} {
		up, down := Climb(tc.eles, 3)
		if up != tc.up || down != tc.down {
			t.Errorf("%v: expected %v up and %v down, got %v and %v", tc.eles, tc.up, tc.down, up, down)
		}
	}
}

package routing

import (
	"math"
	"testing"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/track"
)

// testTolerance is the tolerance of route points, in meters.
const testTolerance = 100

// lRoute returns a route with points every 10 m, 1 km to the east and then 1 km to the north,
// with the corner at index 100, and the given turns.
func lRoute(turns ...Turn) *Route {
	const lat = 51.0
	step := 10 / (6371008.8 * math.Pi / 180)
	r := &Route{}
	for i := 0; i <= 100; i++ {
		r.Points = append(r.Points, geo.LonLat{Lon: 12 + float64(i)*step/math.Cos(lat*math.Pi/180), Lat: lat})
	}
	corner := r.Points[100]
	for i := 1; i <= 100; i++ {
		r.Points = append(r.Points, geo.LonLat{Lon: corner.Lon, Lat: lat + float64(i)*step})
	}
	r.Turns = turns
	r.fixTurns()
	return r
}

// names returns the names of waypoints.
func names(wps []track.Waypoint) []string {
	n := make([]string, len(wps))
	for i, w := range wps {
		n[i] = w.Name
	}
	return n
}

// checkPoints reports differences between the waypoints and the expected names and route indices.
func checkPoints(t *testing.T, r *Route, wps []track.Waypoint, wantNames []string, wantIndex []int) {
	t.Helper()
	if len(wps) != len(wantNames) {
		t.Fatalf("expected %q, got %q", wantNames, names(wps))
	}
	for i, w := range wps {
		if w.Name != wantNames[i] || w.Pos != r.Points[wantIndex[i]] {
			t.Errorf("point %d: expected %q at %d, got %q at %v", i, wantNames[i], wantIndex[i], w.Name, w.Pos)
		}
		if !math.IsNaN(w.Ele) {
			t.Errorf("point %d: expected no elevation, got %v", i, w.Ele)
		}
	}
}

func TestRoutePointsTurns(t *testing.T) {
	r := lRoute(
		Turn{Index: 0, Kind: TurnStart, Street: "A"},
		Turn{Index: 20, Kind: TurnContinue, Street: "B"},
		Turn{Index: 40, Kind: TurnSlightRight, Street: "C"},
		Turn{Index: 100, Kind: TurnLeft, Street: "Waldweg"},
	)
	wps := RoutePoints(r, []string{"Home", "Lake"}, testTolerance)
	checkPoints(t, r, wps, []string{"Home", "02 L Waldweg", "Lake"}, []int{0, 100, 200})
	if wps[1].Desc != "Turn left onto Waldweg" || wps[0].Desc != "" {
		t.Errorf("unexpected descriptions %q, %q", wps[0].Desc, wps[1].Desc)
	}
}

func TestRoutePointsMerge(t *testing.T) {
	r := lRoute(
		// Right after the start.
		Turn{Index: 2, Kind: TurnRight, Street: "A"},
		// A road crossing with an offset of 20 m.
		Turn{Index: 50, Kind: TurnLeft, Street: "X"},
		Turn{Index: 52, Kind: TurnKeepRight},
		Turn{Index: 100, Kind: TurnRoundabout, Street: "Hauptstraße", Exit: 2},
		// Right before the end.
		Turn{Index: 198, Kind: TurnLeft, Street: "B"},
	)
	wps := RoutePoints(r, nil, testTolerance)
	checkPoints(t, r, wps, []string{"Start", "02 L-KR", "03 RB2 Hauptstraße", "End"}, []int{0, 50, 100, 200})
	if wps[1].Desc != "Turn left, then keep right" || wps[2].Desc != "Take exit 2 at the roundabout onto Hauptstraße" {
		t.Errorf("unexpected descriptions %q, %q", wps[1].Desc, wps[2].Desc)
	}
}

func TestRoutePointsTolerance(t *testing.T) {
	// Without turns, the corner is added as it is 707 m from the straight line.
	r := lRoute(Turn{Index: 0, Kind: TurnStart, Street: "A"})
	wps := RoutePoints(r, []string{"Home", "Lake"}, testTolerance)
	checkPoints(t, r, wps, []string{"Home", "02 A", "Lake"}, []int{0, 100, 200})
	if wps[1].Desc != "Follow A" {
		t.Errorf("unexpected description %q", wps[1].Desc)
	}

	// The check can be turned off.
	wps = RoutePoints(r, nil, 0)
	checkPoints(t, r, wps, []string{"Start", "End"}, []int{0, 200})

	// On a half circle with a radius of 1 km, points are added until the lines are within the tolerance.
	c := &Route{}
	for i := 0; i <= 180; i++ {
		a := float64(i) * math.Pi / 180
		c.Points = append(c.Points, geo.LonLat{
			Lon: 12 + 1000*math.Cos(a)/(6371008.8*math.Pi/180*math.Cos(51*math.Pi/180)),
			Lat: 51 + 1000*math.Sin(a)/(6371008.8*math.Pi/180),
		})
	}
	c.fixTurns()
	wps = RoutePoints(c, nil, 50)
	// A chord of a circle with radius R strays R(1-cos(θ/2)) from it, so 50 m allow chords of about 36°.
	if len(wps) < 6 || len(wps) > 9 {
		t.Errorf("expected 6 to 9 points, got %d", len(wps))
	}
	for i, p := range c.Points {
		d := math.Inf(1)
		for j := 1; j < len(wps); j++ {
			d = min(d, segmentDistance(p, wps[j-1].Pos, wps[j].Pos))
		}
		if d > 50 {
			t.Errorf("point %d is %.0f m from the route points", i, d)
		}
	}
}

func TestRoutePointsNames(t *testing.T) {
	// The corner between the via points is added, without a street to name it by.
	r := lRoute(Turn{Index: 50, Kind: TurnVia}, Turn{Index: 150, Kind: TurnVia})
	index := []int{0, 50, 100, 150, 200}
	checkPoints(t, r, RoutePoints(r, nil, testTolerance), []string{"Start", "Via 1", "03", "Via 2", "End"}, index)
	checkPoints(t, r, RoutePoints(r, []string{"Home", " ", "Home"}, testTolerance),
		[]string{"Home", "Via 1", "03", "Home (2)", "End"}, index)
}

func TestRoutePointsShort(t *testing.T) {
	if wps := RoutePoints(&Route{Points: make([]geo.LonLat, 1)}, nil, testTolerance); wps != nil {
		t.Errorf("expected no points, got %v", wps)
	}
}

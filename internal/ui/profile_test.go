package ui

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/settings"
	"github.com/mlange-42/tramp/internal/track"
)

// waitProfile waits for the elevation profile of a route item, and applies it.
func waitProfile(t *testing.T, a *App, it *fileItem) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for it.profile == profileLoading {
		if time.Now().After(deadline) {
			t.Fatal("timeout")
		}
		a.runUITasks()
		time.Sleep(time.Millisecond)
	}
}

func TestRouteProfile(t *testing.T) {
	srv, failing, _ := elevationServer(t)
	a, f := editApp(t, testPlanGPX)
	cfg := settings.Elevation{URL: srv.URL + "/{z}/{x}/{y}.png", TileSize: 4}
	a.elev = elevationView{cfg: cfg, service: newElevationService(cfg)}
	it := f.routeItem(0)
	a.selected = it

	a.updateProfiles()
	if it.profile != profileLoading {
		t.Fatalf("expected the profile requested, got %v", it.profile)
	}
	// The route points have no elevation, so the chart waits for the profile.
	a.updateChart()
	if a.chart.hint != "Loading elevations…" {
		t.Errorf("expected a loading hint, got %q", a.chart.hint)
	}
	waitProfile(t, a, it)

	// The route from 51.0° to 51.1° is about 11 km long, sampled every 20 m along the same line.
	if it.profile != profileDone || len(it.points) != 1 || len(it.lines) != 1 {
		t.Fatalf("expected a profile, got %v", it.profile)
	}
	pts := it.points[0]
	if n := len(pts); n < 550 || n > 600 || len(it.lines[0].Points) != n {
		t.Errorf("expected about 560 points, got %d", n)
	}
	if pts[0].Pos != (geo.LonLat{Lon: 12, Lat: 51}) || pts[len(pts)-1].Pos != (geo.LonLat{Lon: 12, Lat: 51.1}) {
		t.Errorf("expected the route ends kept, got %v and %v", pts[0].Pos, pts[len(pts)-1].Pos)
	}
	for _, p := range pts {
		if p.Ele != 123 {
			t.Fatalf("expected 123 m, got %v", p.Ele)
		}
	}
	if !strings.HasSuffix(it.summary, " · ↑0 m ↓0 m") {
		t.Errorf("expected the climb in the summary, got %q", it.summary)
	}
	if !a.tracksChanged {
		t.Error("expected the map and chart updated")
	}
	a.updateChart()
	if a.chart.info == nil || math.Abs(a.chart.total-it.distances()[0][len(pts)-1]) > 1e-9 {
		t.Errorf("expected the profile in the chart, hint %q", a.chart.hint)
	}

	// A changed route gets a new profile, and the result for the old one is dropped.
	a.toggleEdit(f)
	f.change(func(d *track.File) { d.Routes[0].Points[1].Pos.Lat = 51.05 })
	if it.profile != profileNone || len(it.points[0]) != 2 {
		t.Fatalf("expected the route points back, got %v with %d points", it.profile, len(it.points[0]))
	}
	// The result for the first change is dropped, as the route changed again meanwhile.
	a.updateProfiles()
	f.change(func(d *track.File) { d.Routes[0].Points[1].Pos.Lat = 51.02 })
	a.updateProfiles()
	waitProfile(t, a, it)
	a.runUITasks()
	if pts := it.points[0]; it.profile != profileDone || pts[len(pts)-1].Pos.Lat != 51.02 {
		t.Errorf("expected the profile of the latest route, got %v ending at %v", it.profile, pts[len(pts)-1].Pos)
	}

	// Where tiles fail to load, the route keeps its points.
	failing.Store(true)
	f.change(func(d *track.File) { d.Routes[0].Points[1].Pos.Lon = 13 })
	a.updateProfiles()
	waitProfile(t, a, it)
	if it.profile != profileFailed || len(it.points[0]) != 2 {
		t.Errorf("expected a failed profile, got %v with %d points", it.profile, len(it.points[0]))
	}
}

func TestRouteProfileOff(t *testing.T) {
	a, f := editApp(t, testPlanGPX)
	a.updateProfiles()
	if it := f.routeItem(0); it.profile != profileNone || len(it.points[0]) != 2 {
		t.Errorf("expected no profile without elevation service")
	}
}

func TestFormatClimb(t *testing.T) {
	if got := formatClimb(420.4, 379.6); got != "↑420 m ↓380 m" {
		t.Errorf("unexpected %q", got)
	}
}

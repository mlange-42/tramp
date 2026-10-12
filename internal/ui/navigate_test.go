package ui

import (
	"context"
	"errors"
	"image"
	"strings"
	"sync"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/unit"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/routing"
	"github.com/mlange-42/tramp/internal/settings"
)

// fakeRouter returns a straight route through the points, or an error, and records the requests.
type fakeRouter struct {
	mu       sync.Mutex
	err      error
	requests []routing.Request
}

func (f *fakeRouter) Attribution() string { return "test" }

func (f *fakeRouter) Route(_ context.Context, r routing.Request) (*routing.Route, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r)
	if f.err != nil {
		return nil, f.err
	}
	return &routing.Route{Points: r.Points, Distance: 1234, Duration: 20 * time.Minute}, nil
}

// navApp returns an app with a map of a fixed size and a fake router, with the route panel open.
func navApp() (*App, *fakeRouter, layout.Context) {
	a := &App{style: DefaultStyle(), mapView: mapview.New(geo.LonLat{Lon: 12.3, Lat: 51.3}, 12), invalidate: func() {}}
	a.mapView.View.Size = image.Pt(800, 600)
	f := &fakeRouter{}
	a.router = f
	a.toggleNav()
	return a, f, layout.Context{Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
}

// waitRoute waits for the background routing to finish, and applies its result.
func waitRoute(a *App) {
	for a.nav.busy {
		a.bgMu.Lock()
		n := len(a.uiTasks)
		a.bgMu.Unlock()
		if n > 0 {
			a.runUITasks()
		}
	}
}

// navClick clicks the map at a position, moving the pointer by drag pixels between press and release.
func navClick(a *App, ll geo.LonLat, drag float32) {
	ev := at(a, pointer.Press, ll)
	a.navPointer(ev)
	ev.Kind, ev.Screen = pointer.Drag, ev.Screen.Add(f32.Pt(drag, 0))
	a.navPointer(ev)
	ev.Kind = pointer.Release
	a.navPointer(ev)
}

func TestNavPicking(t *testing.T) {
	a, f, _ := navApp()
	start, target := geo.LonLat{Lon: 12.2, Lat: 51.2}, geo.LonLat{Lon: 12.4, Lat: 51.4}
	if !a.nav.picking() || a.navStatus() != "Click the map to set the start" {
		t.Fatalf("expected picking the start, got %q", a.navStatus())
	}
	// Dragging doesn't set a point.
	navClick(a, start, 10)
	if a.nav.has[0] {
		t.Fatal("a drag set the start")
	}
	navClick(a, start, 1)
	if !a.nav.has[0] || !near(a.nav.ends[0], start) || a.nav.busy || a.navStatus() != "Click the map to set the target" {
		t.Fatalf("expected the start set, got %+v", a.nav)
	}
	navClick(a, target, 0)
	if !a.nav.has[1] || !near(a.nav.ends[1], target) || !a.nav.busy || a.navStatus() != "Routing…" {
		t.Fatalf("expected routing, got %+v", a.nav)
	}
	waitRoute(a)
	if a.nav.picking() || a.nav.route == nil || a.nav.err != nil {
		t.Fatalf("expected a route, got %+v", a.nav)
	}
	if s := a.navStatus(); s != "1.2 km · 20 min" {
		t.Errorf("unexpected status %q", s)
	}
	if len(f.requests) != 1 || f.requests[0].Profile != routing.ProfileFoot || len(f.requests[0].Points) != 2 {
		t.Errorf("unexpected requests %+v", f.requests)
	}

	// Further clicks are not taken.
	navClick(a, geo.LonLat{Lon: 12.3, Lat: 51.3}, 0)
	if !near(a.nav.ends[0], start) || !near(a.nav.ends[1], target) {
		t.Errorf("ends changed: %v", a.nav.ends)
	}

	a.closeNav()
	if a.nav.open || a.nav.has[0] || a.nav.has[1] || a.nav.route != nil {
		t.Errorf("expected everything cleared, got %+v", a.nav)
	}
}

func TestNavProfileAndSwap(t *testing.T) {
	a, f, gtx := navApp()
	start, target := geo.LonLat{Lon: 12.2, Lat: 51.2}, geo.LonLat{Lon: 12.4, Lat: 51.4}
	navClick(a, start, 0)
	navClick(a, target, 0)
	waitRoute(a)

	// Clicking a profile button routes again with that profile, and keeps it for the settings.
	a.nav.profileBtns[1].btn.Click()
	a.updateNav(gtx)
	waitRoute(a)
	if len(f.requests) != 2 || f.requests[1].Profile != routing.ProfileBike || a.routingCfg.Profile != routing.ProfileBike {
		t.Errorf("expected routing by bike, got %+v, settings %+v", f.requests, a.routingCfg)
	}

	a.nav.swapBtn.btn.Click()
	a.updateNav(gtx)
	waitRoute(a)
	if !near(a.nav.ends[0], target) || !near(a.nav.ends[1], start) || len(f.requests) != 3 ||
		!near(f.requests[2].Points[0], target) {
		t.Errorf("expected swapped ends, got %v, requests %+v", a.nav.ends, f.requests)
	}
}

func TestNavError(t *testing.T) {
	a, f, _ := navApp()
	f.err = errors.New("routing failed: Path distance exceeds the max distance limit: 100000 meters")
	navClick(a, geo.LonLat{Lon: 12.2, Lat: 51.2}, 0)
	navClick(a, geo.LonLat{Lon: 12.4, Lat: 51.4}, 0)
	waitRoute(a)
	s := a.navStatus()
	if a.nav.route != nil || !strings.HasPrefix(s, "Routing failed: Path distance") || !strings.Contains(s, "osrm") {
		t.Errorf("expected the error with a hint, got %q", s)
	}
}

func TestNavOutdated(t *testing.T) {
	a, f, _ := navApp()
	navClick(a, geo.LonLat{Lon: 12.2, Lat: 51.2}, 0)
	navClick(a, geo.LonLat{Lon: 12.4, Lat: 51.4}, 0)
	// Closing while routing drops the result.
	a.closeNav()
	for {
		f.mu.Lock()
		n := len(f.requests)
		f.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(10 * time.Millisecond)
	a.runUITasks()
	if a.nav.route != nil || a.nav.busy {
		t.Errorf("expected the result dropped, got %+v", a.nav)
	}
}

func TestProfileIndex(t *testing.T) {
	for p, want := range map[string]int{"": 0, "foot": 0, "bike": 1, "car": 2, "horse": 0} {
		if got := profileIndex(p); got != want {
			t.Errorf("profile %q: expected %d, got %d", p, want, got)
		}
	}
	a := newApp(func() {}, Options{Maps: DefaultMaps(), State: settings.Settings{Routing: settings.Routing{Profile: "car"}}})
	if a.nav.profile != 2 || a.State().Routing.Profile != "car" {
		t.Errorf("expected the car profile from the settings, got %d, %+v", a.nav.profile, a.State().Routing)
	}
}

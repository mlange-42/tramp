package ui

import (
	"context"
	"errors"
	"image"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/unit"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/geocode"
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

// navPts returns the positions of the stops of the route that are set.
func navPts(a *App) []geo.LonLat {
	pts, _ := a.nav.points()
	return pts
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
// It returns the events the route didn't use.
func navClick(a *App, ll geo.LonLat, drag float32) []mapview.PointerEvent {
	gtx := layout.Context{Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	ev := at(a, pointer.Press, ll)
	evs := []mapview.PointerEvent{ev}
	ev.Kind, ev.Screen = pointer.Drag, ev.Screen.Add(f32.Pt(drag, 0))
	ev.Pos = a.mapView.View.ToMap(float64(ev.Screen.X), float64(ev.Screen.Y))
	evs = append(evs, ev)
	ev.Kind = pointer.Release
	evs = append(evs, ev)
	return a.updateNav(gtx, evs)
}

func TestNavPicking(t *testing.T) {
	a, f, _ := navApp()
	start, target := geo.LonLat{Lon: 12.2, Lat: 51.2}, geo.LonLat{Lon: 12.4, Lat: 51.4}
	if !a.nav.picking() || a.navStatus() != "Click the map to set the start, or search it" {
		t.Fatalf("expected picking the start, got %q", a.navStatus())
	}
	// Dragging doesn't set a point.
	navClick(a, start, 10)
	if len(navPts(a)) != 0 {
		t.Fatal("a drag set the start")
	}
	navClick(a, start, 0)
	if len(navPts(a)) != 1 || !near(navPts(a)[0], start) || a.nav.busy || a.navStatus() != "Click the map to set the target, or search it" {
		t.Fatalf("expected the start set, got %+v", a.nav)
	}
	navClick(a, target, 0)
	if len(navPts(a)) != 2 || !near(navPts(a)[1], target) || !a.nav.busy || a.navStatus() != "Routing…" {
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

	// Further clicks away from the route are left to the editor.
	if rest := navClick(a, geo.LonLat{Lon: 12.4, Lat: 51.2}, 0); len(rest) != 3 {
		t.Errorf("expected the click passed on, got %v", rest)
	}
	if len(navPts(a)) != 2 || !near(navPts(a)[0], start) || !near(navPts(a)[1], target) {
		t.Errorf("points changed: %v", navPts(a))
	}

	// Closing keeps the route, and opening shows it again without routing.
	a.closeNav()
	if a.nav.open || len(navPts(a)) != 2 || a.nav.route == nil {
		t.Errorf("expected the route kept, got %+v", a.nav)
	}
	if rest := navClick(a, start, 40); len(rest) != 3 {
		t.Errorf("expected clicks passed on while closed, got %v", rest)
	}
	a.toggleNav()
	if !a.nav.open || a.nav.route == nil || a.nav.busy || len(f.requests) != 1 {
		t.Errorf("expected the route shown again, got %+v", a.nav)
	}

	a.clearNav()
	if !a.nav.open || len(navPts(a)) != 0 || a.nav.route != nil || !a.nav.picking() {
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
	a.updateNav(gtx, nil)
	waitRoute(a)
	if len(f.requests) != 2 || f.requests[1].Profile != routing.ProfileBike || a.routingCfg.Profile != routing.ProfileBike {
		t.Errorf("expected routing by bike, got %+v, settings %+v", f.requests, a.routingCfg)
	}

	a.nav.swapBtn.btn.Click()
	a.updateNav(gtx, nil)
	waitRoute(a)
	if !near(navPts(a)[0], target) || !near(navPts(a)[1], start) || len(f.requests) != 3 ||
		!near(f.requests[2].Points[0], target) {
		t.Errorf("expected swapped ends, got %v, requests %+v", navPts(a), f.requests)
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
	// Clearing while routing drops the result.
	a.clearNav()
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

func TestNavDragEnds(t *testing.T) {
	a, f, gtx := navApp()
	start, target := geo.LonLat{Lon: 12.2, Lat: 51.2}, geo.LonLat{Lon: 12.4, Lat: 51.4}
	navClick(a, start, 0)
	navClick(a, target, 0)
	waitRoute(a)

	// Dragging the target moves it, and routes again on release.
	if rest := navClick(a, target, 40); len(rest) != 0 {
		t.Errorf("expected the drag taken, got %v", rest)
	}
	moved := geo.ToLonLat(a.mapView.View.ToMap(float64(screenPos(&a.mapView.View, target).X+40), float64(screenPos(&a.mapView.View, target).Y)))
	if !near(navPts(a)[1], moved) || !near(navPts(a)[0], start) || !a.nav.busy {
		t.Fatalf("expected the target moved and routing, got %+v", a.nav)
	}
	waitRoute(a)
	if len(f.requests) != 2 || !near(f.requests[1].Points[1], moved) {
		t.Errorf("unexpected requests %+v", f.requests)
	}

	// Clicking an end without moving it doesn't route again.
	navClick(a, start, 1)
	if a.nav.busy || len(f.requests) != 2 || !near(navPts(a)[0], start) {
		t.Errorf("expected nothing changed, got %+v", a.nav)
	}

	// A canceled drag puts the end back.
	p := at(a, pointer.Press, start)
	d := p
	d.Kind, d.Screen = pointer.Drag, p.Screen.Add(f32.Pt(30, 30))
	d.Pos = a.mapView.View.ToMap(float64(d.Screen.X), float64(d.Screen.Y))
	c := d
	c.Kind = pointer.Cancel
	a.updateNav(gtx, []mapview.PointerEvent{p, d})
	if near(navPts(a)[0], start) || !a.nav.dragging {
		t.Fatalf("expected the start dragged, got %+v", a.nav)
	}
	if cur, ok := a.navCursor(gtx); !ok || cur != pointer.CursorGrabbing {
		t.Errorf("expected the grabbing cursor, got %v", cur)
	}
	a.updateNav(gtx, []mapview.PointerEvent{c})
	if !near(navPts(a)[0], start) || a.nav.dragging || a.nav.busy {
		t.Errorf("expected the start back, got %+v", a.nav)
	}
}

func TestNavDragWhilePicking(t *testing.T) {
	a, f, _ := navApp()
	start := geo.LonLat{Lon: 12.2, Lat: 51.2}
	navClick(a, start, 0)
	// Dragging the start while the target is missing moves the start, and doesn't route.
	navClick(a, start, 40)
	if len(navPts(a)) != 1 || near(navPts(a)[0], start) || a.nav.busy || len(f.requests) != 0 {
		t.Errorf("expected only the start moved, got %+v", a.nav)
	}
}

func TestNavVia(t *testing.T) {
	a, f, _ := navApp()
	start, target := geo.LonLat{Lon: 12.2, Lat: 51.3}, geo.LonLat{Lon: 12.4, Lat: 51.3}
	navClick(a, start, 0)
	navClick(a, target, 0)
	waitRoute(a)

	// A click on the route adds nothing.
	mid := geo.LonLat{Lon: 12.3, Lat: 51.3}
	if rest := navClick(a, mid, 0); len(rest) != 0 || len(navPts(a)) != 2 || a.nav.busy {
		t.Fatalf("expected the click taken without change, got %v, %+v", rest, navPts(a))
	}
	// Dragging the route adds a via point where it is released, and routes through it.
	navClick(a, mid, 0)
	navClick(a, geo.LonLat{Lon: 12.3, Lat: 51.3}, -40)
	if len(navPts(a)) != 3 || !a.nav.busy || a.nav.isVia(0) || !a.nav.isVia(1) {
		t.Fatalf("expected a via point, got %v", navPts(a))
	}
	via := navPts(a)[1]
	waitRoute(a)
	if len(f.requests) != 2 || len(f.requests[1].Points) != 3 || !near(f.requests[1].Points[1], via) {
		t.Errorf("expected routing through the via point, got %+v", f.requests)
	}

	// Dragging the second leg adds a via point after the first.
	leg2 := geo.ToLonLat(geo.Point{
		X: (geo.ToMercator(via).X + geo.ToMercator(target).X) / 2,
		Y: (geo.ToMercator(via).Y + geo.ToMercator(target).Y) / 2,
	})
	navClick(a, leg2, 40)
	waitRoute(a)
	if len(navPts(a)) != 4 || !near(navPts(a)[1], via) || a.navStatus() == "" {
		t.Fatalf("expected a second via point after the first, got %v", navPts(a))
	}

	// Clicking a via point removes it.
	navClick(a, via, 0)
	if len(navPts(a)) != 3 || near(navPts(a)[1], via) || !a.nav.busy {
		t.Errorf("expected the first via point removed, got %v", navPts(a))
	}
	waitRoute(a)

	// Reversing keeps the via points in route order.
	second := navPts(a)[1]
	slices.Reverse(a.nav.stops)
	if !near(navPts(a)[1], second) || !near(navPts(a)[0], target) {
		t.Errorf("unexpected reversed points %v", navPts(a))
	}
}

func TestViaInsertIndex(t *testing.T) {
	// A line along the x axis, with points at x = 0, 1, ..., 10.
	line := make([]geo.Point, 11)
	for i := range line {
		line[i] = geo.Point{X: float64(i)}
	}
	ll := func(x float64) geo.LonLat { return geo.ToLonLat(geo.Point{X: x}) }
	points := []geo.LonLat{ll(0), ll(3), ll(7), ll(10)}
	for seg, want := range map[int]int{0: 1, 2: 1, 3: 2, 5: 2, 7: 3, 9: 3} {
		if got := viaInsertIndex(line, points, seg); got != want {
			t.Errorf("segment %d: expected index %d, got %d", seg, want, got)
		}
	}
	if got := viaInsertIndex(line, points[:2:2], 4); got != 1 {
		t.Errorf("without via points: expected 1, got %d", got)
	}
}

func TestNavSettings(t *testing.T) {
	a, f, _ := navApp()
	start, via, target := geo.LonLat{Lon: 12.2, Lat: 51.3}, geo.LonLat{Lon: 12.3, Lat: 51.2}, geo.LonLat{Lon: 12.4, Lat: 51.3}
	a.nav.setPoints([]geo.LonLat{start, via, target})
	a.nav.stops[2].name = "Leipzig"
	cfg := a.navState()
	if len(cfg.Points) != 3 || cfg.Points[1] != (settings.RoutePoint{Lon: 12.3, Lat: 51.2}) ||
		cfg.Points[2] != (settings.RoutePoint{Lon: 12.4, Lat: 51.3, Name: "Leipzig"}) {
		t.Fatalf("unexpected saved points %+v", cfg.Points)
	}
	// Saved points are rounded to about a meter.
	a.nav.stops[0].pos = geo.LonLat{Lon: 12.373114999, Lat: -51.339741234}
	if p := a.navState().Points[0]; p != (settings.RoutePoint{Lon: 12.37311, Lat: -51.33974}) {
		t.Errorf("expected rounded coordinates, got %+v", p)
	}
	a.nav.stops[0].pos = start

	// Loaded points are routed when the panel is opened.
	a.closeNav()
	a.loadNav(cfg)
	if len(navPts(a)) != 3 || !a.nav.stale || len(f.requests) != 0 || a.nav.stops[2].label() != "Leipzig" {
		t.Fatalf("expected the points loaded without routing, got %+v", a.nav)
	}
	a.toggleNav()
	waitRoute(a)
	if a.nav.stale || a.nav.route == nil || len(f.requests) != 1 || len(f.requests[0].Points) != 3 {
		t.Errorf("expected the route found on opening, got %+v", f.requests)
	}

	// A single point is kept, but not routed.
	a.closeNav()
	a.loadNav(settings.Routing{Points: cfg.Points[:1]})
	a.toggleNav()
	if len(navPts(a)) != 1 || a.nav.busy || !a.nav.picking() {
		t.Errorf("expected picking the target, got %+v", a.nav)
	}
}

func TestNavSearch(t *testing.T) {
	a, f, gtx := navApp()
	a.search = searchBox{}
	leipzig := geocode.Place{Name: "Leipzig", Pos: geo.LonLat{Lon: 12.37, Lat: 51.34}}
	a.geocoder = &fakeSearcher{places: []geocode.Place{leipzig}}

	// Searching the target first leaves the start to be clicked.
	a.setNavStop(1, leipzig)
	if !a.nav.stops[1].set || a.nav.stops[0].set || !a.nav.picking() || a.nav.busy {
		t.Fatalf("expected only the target set, got %+v", a.nav.stops)
	}
	a.updateNavFields(gtx)
	if txt := a.nav.stops[1].field.ed.Text(); txt != "Leipzig" {
		t.Errorf("expected the name in the field, got %q", txt)
	}
	start := geo.LonLat{Lon: 12.2, Lat: 51.3}
	navClick(a, start, 0)
	waitRoute(a)
	if !near(a.nav.stops[0].pos, start) || len(f.requests) != 1 || !near(f.requests[0].Points[1], leipzig.Pos) {
		t.Fatalf("expected a route from the click to the place, got %+v", f.requests)
	}
	a.updateNavFields(gtx)
	if txt := a.nav.stops[0].field.ed.Text(); txt != "51.30000, 12.20000" {
		t.Errorf("expected the coordinates in the field, got %q", txt)
	}

	// Typed coordinates set a stop.
	a.nav.stops[0].field.ed.SetText("51.25, 12.25")
	p, ok := a.nav.stops[0].field.submit(gtx, a)
	if !ok {
		t.Fatal("expected coordinates")
	}
	a.setNavStop(0, p)
	waitRoute(a)
	if !near(a.nav.stops[0].pos, geo.LonLat{Lon: 12.25, Lat: 51.25}) || len(f.requests) != 2 {
		t.Errorf("expected the typed start, got %+v", a.nav.stops[0])
	}

	// A stop outside the view shows all stops.
	far := geocode.Place{Name: "Dresden", Pos: geo.LonLat{Lon: 13.74, Lat: 51.05}}
	a.setNavStop(1, far)
	waitRoute(a)
	b := a.mapView.View.Bounds()
	for _, p := range navPts(a) {
		if !b.Contains(geo.EmptyRect().Extend(geo.ToMercator(p))) {
			t.Errorf("expected %v in the view %v", p, b)
		}
	}
	a.setNavStop(1, leipzig)
	waitRoute(a)

	// Searching a place by name lists it for picking.
	fld := &a.nav.stops[1].field
	fld.ed.SetText("Leipzig")
	if _, ok := fld.submit(gtx, a); ok {
		t.Fatal("a name is no coordinates")
	}
	for fld.busy {
		a.bgMu.Lock()
		n := len(a.uiTasks)
		a.bgMu.Unlock()
		if n > 0 {
			a.runUITasks()
		}
	}
	if len(fld.places) != 1 || !fld.open {
		t.Errorf("expected the place listed, got %+v", fld.places)
	}
	fld.clear()

	// A dragged stop loses its name.
	a.mapView.View.Center, a.mapView.View.Zoom = geo.ToMercator(leipzig.Pos), 12
	navClick(a, leipzig.Pos, 40)
	waitRoute(a)
	if a.nav.stops[1].name != "" || near(a.nav.stops[1].pos, leipzig.Pos) {
		t.Errorf("expected the target moved without name, got %+v", a.nav.stops[1])
	}

	// Clearing a field unsets its stop, and removes the route.
	a.unsetNavStop(0)
	if a.nav.stops[0].set || a.nav.route != nil || !a.nav.picking() {
		t.Errorf("expected the start unset, got %+v", a.nav.stops[0])
	}
}

func TestNavAddVia(t *testing.T) {
	a, f, gtx := navApp()
	navClick(a, geo.LonLat{Lon: 12.2, Lat: 51.3}, 0)
	navClick(a, geo.LonLat{Lon: 12.4, Lat: 51.3}, 0)
	waitRoute(a)

	// An added via point goes before the target, and is set by the next click.
	a.nav.viaBtn.btn.Click()
	a.updateNav(gtx, nil)
	if len(a.nav.stops) != 3 || a.nav.stops[1].set || !a.nav.picking() || !a.nav.routable() {
		t.Fatalf("expected an empty via point, got %+v", a.nav.stops)
	}
	via := geo.LonLat{Lon: 12.3, Lat: 51.2}
	navClick(a, via, 0)
	waitRoute(a)
	if !near(a.nav.stops[1].pos, via) || len(f.requests) != 2 || len(f.requests[1].Points) != 3 {
		t.Fatalf("expected routing through the via point, got %+v", f.requests)
	}

	// Its button removes it.
	a.nav.stops[1].removeBtn.btn.Click()
	a.updateNav(gtx, nil)
	waitRoute(a)
	if len(a.nav.stops) != 2 || len(f.requests) != 3 || len(f.requests[2].Points) != 2 {
		t.Errorf("expected the via point removed, got %+v", a.nav.stops)
	}

	// An empty via point is left out of the route.
	a.nav.viaBtn.btn.Click()
	a.updateNav(gtx, nil)
	a.startRoute()
	waitRoute(a)
	if n := len(f.requests); n != 4 || len(f.requests[3].Points) != 2 {
		t.Errorf("expected routing without the empty via point, got %+v", f.requests)
	}
}

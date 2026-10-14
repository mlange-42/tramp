package ui

import (
	"image"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/settings"
	"github.com/mlange-42/tramp/internal/track"
)

func TestNavRouteName(t *testing.T) {
	for _, tc := range []struct {
		stops []string
		want  string
	}{
		{nil, "Route"},
		{[]string{"", "", ""}, "Route"},
		{[]string{"Leipzig", "", "Halle"}, "Leipzig → Halle"},
		{[]string{"Leipzig", ""}, "Leipzig → Target"},
		{[]string{" ", "Halle"}, "Start → Halle"},
	} {
		if got := navRouteName(tc.stops); got != tc.want {
			t.Errorf("stops %q: expected %q, got %q", tc.stops, tc.want, got)
		}
	}
}

func TestFileName(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"Leipzig → Halle", "Leipzig - Halle"},
		{`A/B: "C"?`, "AB C"},
		{" ..", "Route"},
	} {
		if got := fileName(tc.name); got != tc.want {
			t.Errorf("%q: expected %q, got %q", tc.name, tc.want, got)
		}
	}
}

func TestSaveNavRoute(t *testing.T) {
	a, f := editApp(t, testPlanGPX)
	a.style = DefaultStyle()
	a.mapView.View.Size = image.Pt(800, 600)
	a.router = &fakeRouter{}
	a.toggleNav()

	// Nothing to save without a route.
	if a.nav.navSavable() || !strings.Contains(a.navSaveHelp(), "once the route is found") {
		t.Errorf("expected no route to save, help %q", a.navSaveHelp())
	}

	a.nav.setPoints([]geo.LonLat{{Lon: 12.5, Lat: 51.5}, {Lon: 12.7, Lat: 51.5}})
	a.nav.stops[0].name = "Hut"
	a.startRoute()
	waitRoute(a)
	if !a.nav.navSavable() || a.nav.saveCount != 2 || !strings.HasSuffix(a.navSaveHelp(), "a new file, with 2 points") {
		t.Fatalf("expected a route to save, count %d, help %q", a.nav.saveCount, a.navSaveHelp())
	}

	// The route is added to the edited file, where it can be undone.
	a.toggleEdit(f)
	if !strings.Contains(a.navSaveHelp(), "in test.gpx") {
		t.Errorf("expected the edited file in the help, got %q", a.navSaveHelp())
	}
	a.saveNavRoute()
	if len(f.data.Routes) != 2 || !f.dirty() {
		t.Fatalf("expected the route added, got %+v", f.data.Routes)
	}
	r := f.data.Routes[1]
	// The waypoint Hut is in the file already, so the start gets another name.
	if r.Name != "Hut → Target" || len(r.Points) != 2 || r.Points[0].Name != "Hut (2)" || r.Points[1].Name != "End" {
		t.Errorf("unexpected route %+v", r)
	}
	if a.selected != f.routeItem(1) {
		t.Errorf("expected the new route selected")
	}

	// Saving again gives the route and its points other names.
	a.saveNavRoute()
	if r := f.data.Routes[2]; r.Name != "Hut → Target (2)" || r.Points[0].Name != "Hut (3)" || r.Points[1].Name != "End (2)" {
		t.Errorf("unexpected second route %+v", r)
	}

	a.undo()
	a.undo()
	if len(f.data.Routes) != 1 || f.dirty() {
		t.Errorf("expected the routes undone, got %d", len(f.data.Routes))
	}
}

func TestNavSaveWarning(t *testing.T) {
	a, _, _ := navApp()
	navClick(a, geo.LonLat{Lon: 12.2, Lat: 51.2}, 0)
	navClick(a, geo.LonLat{Lon: 12.4, Lat: 51.4}, 0)
	waitRoute(a)
	if w := a.navSaveWarning(); w != "" {
		t.Errorf("expected no warning, got %q", w)
	}
	a.nav.saveCount = maxDevicePoints + 1
	if w := a.navSaveWarning(); !strings.Contains(w, "251 points") {
		t.Errorf("expected a warning, got %q", w)
	}
}

func TestNavTrackFile(t *testing.T) {
	a, _, _ := navApp()
	if !strings.Contains(a.navSaveTrackHelp(), "once the route is found") {
		t.Errorf("unexpected help without route %q", a.navSaveTrackHelp())
	}
	navClick(a, geo.LonLat{Lon: 12.2, Lat: 51.2}, 0)
	navClick(a, geo.LonLat{Lon: 12.4, Lat: 51.4}, 0)
	waitRoute(a)
	a.nav.stops[1].name = "Halle"
	if h := a.navSaveTrackHelp(); h != "Save as GPX track in a new file, with all 2 points" {
		t.Errorf("unexpected help %q", h)
	}

	// The track has all points of the route, without elevation and time, and reads back as such.
	path := filepath.Join(t.TempDir(), "track.gpx")
	if err := track.WriteFile(path, a.navTrackFile()); err != nil {
		t.Fatal(err)
	}
	d, err := track.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Tracks) != 1 || d.Tracks[0].Name != "Start → Halle" || len(d.Tracks[0].Segments) != 1 || len(d.Routes) != 0 {
		t.Fatalf("unexpected file %+v", d)
	}
	pts := d.Tracks[0].Segments[0].Points
	if len(pts) != 2 || !near(pts[0].Pos, a.nav.route.Points[0]) || !near(pts[1].Pos, a.nav.route.Points[1]) ||
		pts[0].HasEle() || pts[0].HasTime() {
		t.Errorf("unexpected points %+v", pts)
	}
	if f := newOpenFile(path, d); f.editable() {
		t.Errorf("expected the track file not to be editable")
	}
}

func TestAddTrackElevations(t *testing.T) {
	srv, _, _ := elevationServer(t)
	svc := newElevationService(settings.Elevation{URL: srv.URL + "/{z}/{x}/{y}.png", TileSize: 4}, 0)
	tr := track.Track{Segments: []track.Segment{
		{Points: []track.Point{{Pos: geo.LonLat{Lon: 12.5, Lat: 51.5}, Ele: math.NaN()}}},
		{Points: []track.Point{{Pos: geo.LonLat{Lon: 12.6, Lat: 51.5}, Ele: math.NaN()}, {Pos: geo.LonLat{Lon: 12.7, Lat: 51.5}, Ele: math.NaN()}}},
	}}
	addTrackElevations(svc, &tr)
	if tr.Segments[0].Points[0].Ele != 123 || tr.Segments[1].Points[1].Ele != 123 {
		t.Errorf("expected elevations for all points, got %+v", tr.Segments)
	}
	tr.Segments[0].Points[0].Ele = math.NaN()
	addTrackElevations(nil, &tr)
	if !math.IsNaN(tr.Segments[0].Points[0].Ele) {
		t.Error("expected no elevation without service")
	}
}

func TestAddRouteElevations(t *testing.T) {
	srv, failing, _ := elevationServer(t)
	svc := newElevationService(settings.Elevation{URL: srv.URL + "/{z}/{x}/{y}.png", TileSize: 4}, 0)
	newRoute := func() track.Route {
		return track.Route{Points: []track.Waypoint{
			{Pos: geo.LonLat{Lon: 12.5, Lat: 51.5}, Ele: math.NaN()},
			{Pos: geo.LonLat{Lon: 12.6, Lat: 51.5}, Ele: math.NaN()},
		}}
	}
	r := newRoute()
	addRouteElevations(svc, &r)
	if r.Points[0].Ele != 123 || r.Points[1].Ele != 123 {
		t.Errorf("expected elevations, got %+v", r.Points)
	}
	// Without service, or when the download fails, the route keeps its points without elevation.
	r = newRoute()
	addRouteElevations(nil, &r)
	if !math.IsNaN(r.Points[0].Ele) {
		t.Error("expected no elevation without service")
	}
	failing.Store(true)
	r = newRoute()
	r.Points[0].Pos.Lon = 20
	addRouteElevations(svc, &r)
	if !math.IsNaN(r.Points[0].Ele) {
		t.Error("expected no elevation after a failed download")
	}
}

func TestRouteTrackFile(t *testing.T) {
	r := track.Route{Name: "Loop", Desc: "Nice", Points: []track.Waypoint{
		{Pos: geo.LonLat{Lon: 12, Lat: 51}, Ele: 100},
		{Pos: geo.LonLat{Lon: 12, Lat: 51.001}},
		{Pos: geo.LonLat{Lon: 12.001, Lat: 51.001}},
	}}
	f := routeTrackFile(&r)
	if len(f.Tracks) != 1 || f.Tracks[0].Name != "Loop" || f.Tracks[0].Desc != "Nice" || len(f.Tracks[0].Segments) != 1 {
		t.Fatalf("expected one named track with one segment, got %+v", f.Tracks)
	}
	pts := f.Tracks[0].Segments[0].Points
	if pts[0].Pos != r.Points[0].Pos || pts[len(pts)-1].Pos != r.Points[2].Pos {
		t.Errorf("expected the track from the start to the end of the route")
	}
	for i := 1; i < len(pts); i++ {
		if d := geo.Distance(pts[i-1].Pos, pts[i].Pos); d > profileSpacing+1e-6 {
			t.Fatalf("points %d and %d are %v m apart", i-1, i, d)
		}
	}
	// The track's length is the route's length.
	l := 0.0
	for i := 1; i < len(pts); i++ {
		l += geo.Distance(pts[i-1].Pos, pts[i].Pos)
	}
	if math.Abs(l-r.Length()) > 0.01 {
		t.Errorf("expected length %v, got %v", r.Length(), l)
	}
	if f := routeTrackFile(&track.Route{Points: r.Points}); f.Tracks[0].Name != "Route" {
		t.Errorf("expected a default name, got %q", f.Tracks[0].Name)
	}
}

func TestSelectedFileRoute(t *testing.T) {
	a, f := editApp(t, testPlanGPX)
	a.selected = nil
	if a.selectedFileRoute() != nil {
		t.Error("expected no route without selection")
	}
	a.selected = f.routeItem(0)
	if r := a.selectedFileRoute(); r != &f.data.Routes[0] {
		t.Errorf("expected the selected route, got %v", r)
	}
	// The route found by routing is not a route of a file.
	a.selected = &fileItem{kind: routeItem}
	if a.selectedFileRoute() != nil {
		t.Error("expected no route for an item outside the files")
	}
}

package ui

import (
	"image"
	"strings"
	"testing"

	"github.com/mlange-42/tramp/internal/geo"
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

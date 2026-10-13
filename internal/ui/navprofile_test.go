package ui

import (
	"image"
	"strings"
	"testing"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/settings"
	"github.com/mlange-42/tramp/internal/track"
)

// navFileApp returns an app with an opened file whose route is selected, a fake router,
// and an elevation service with a constant elevation of 123 m.
func navFileApp(t *testing.T) (*App, *fileItem) {
	t.Helper()
	srv, _, _ := elevationServer(t)
	a, f := editApp(t, testPlanGPX)
	a.style = DefaultStyle()
	// The map shows the found route, see [findNavRoute].
	a.mapView = mapview.New(geo.LonLat{Lon: 12.55, Lat: 51.5}, 12)
	a.mapView.View.Size = image.Pt(800, 600)
	a.router = &fakeRouter{}
	cfg := settings.Elevation{URL: srv.URL + "/{z}/{x}/{y}.png", TileSize: 4}
	a.elev = elevationView{cfg: cfg, service: newElevationService(cfg)}
	fileIt := f.routeItem(0)
	a.selected = fileIt
	return a, fileIt
}

// findNavRoute sets start and target, and waits for the route.
func findNavRoute(a *App) {
	a.nav.setPoints([]geo.LonLat{{Lon: 12.5, Lat: 51.5}, {Lon: 12.6, Lat: 51.5}})
	a.startRoute()
	waitRoute(a)
}

func TestNavProfile(t *testing.T) {
	a, fileIt := navFileApp(t)
	a.toggleNav()
	if a.selected != fileIt {
		t.Fatal("expected the file's route still selected without a found route")
	}

	// A found route shows its profile, with the ascent and descent in the panel once known.
	findNavRoute(a)
	it := a.nav.item
	if it == nil || a.selected != it || a.nav.prevSelected != fileIt || !a.shown(it) {
		t.Fatalf("expected the found route selected")
	}
	a.updateProfiles()
	waitProfile(t, a, it)
	a.updateChart()
	if a.chart.info == nil || a.chart.item != it {
		t.Errorf("expected the profile in the chart, hint %q", a.chart.hint)
	}
	if s := a.navStatus(); !strings.HasSuffix(s, " · ↑0 m ↓0 m") {
		t.Errorf("expected the climb in the status, got %q", s)
	}

	// Changes of the files keep the found route selected.
	a.changed()
	if a.selected != it {
		t.Error("expected the found route kept selected")
	}

	// A new route replaces the profile, and keeps the item to go back to.
	a.startRoute()
	waitRoute(a)
	if a.nav.item == it || a.selected != a.nav.item || a.nav.prevSelected != fileIt {
		t.Errorf("expected the new route selected")
	}
	if a.navStatus() == "" || strings.Contains(a.navStatus(), "↑") {
		t.Errorf("expected no climb before the new profile, got %q", a.navStatus())
	}

	// Closing the panel goes back to the file's route, and opening it shows the profile again.
	a.closeNav()
	if a.selected != fileIt || a.shown(a.nav.item) {
		t.Errorf("expected the file's route selected again")
	}
	a.toggleNav()
	if a.selected != a.nav.item {
		t.Errorf("expected the found route selected when opening the panel")
	}

	// A selected file item replaces the profile, and stays selected when the panel is closed.
	a.selected = fileIt
	a.closeNav()
	if a.selected != fileIt {
		t.Errorf("expected the file's route kept")
	}

	// Clearing the route goes back as well.
	a.toggleNav()
	a.clearNav()
	if a.nav.item != nil || a.selected != fileIt {
		t.Errorf("expected the profile removed with the route")
	}
}

func TestNavProfileClick(t *testing.T) {
	a, fileIt := navFileApp(t)
	a.toggleNav()
	findNavRoute(a)
	a.selected = fileIt

	// A click on the route shows its profile, without adding a via point.
	navClick(a, geo.LonLat{Lon: 12.55, Lat: 51.5}, 0)
	if a.selected != a.nav.item || len(a.nav.stops) != 2 {
		t.Errorf("expected the profile shown, got %d stops", len(a.nav.stops))
	}
}

func TestNavProfileClosedFile(t *testing.T) {
	a, fileIt := navFileApp(t)
	a.toggleNav()
	findNavRoute(a)
	// The item shown before is gone when the panel is closed.
	a.closeFile(a.files[0])
	if a.selected != a.nav.item {
		t.Fatal("expected the found route kept selected")
	}
	a.closeNav()
	if a.selected != nil || a.selected == fileIt {
		t.Errorf("expected nothing selected")
	}
}

func TestChartElevationFallback(t *testing.T) {
	a, fileIt := navFileApp(t)
	a.colorBy.SetSelected(metricIndex(track.SpeedMetric))
	a.updateProfiles()
	waitProfile(t, a, fileIt)
	// Routes have no speed, so the chart shows their elevation.
	a.updateChart()
	if a.chart.info == nil || a.chart.info.metric != track.ElevationMetric || a.chart.coloring != nil {
		t.Errorf("expected the elevation in the item's color, hint %q", a.chart.hint)
	}
}

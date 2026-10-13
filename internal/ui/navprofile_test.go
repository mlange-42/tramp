package ui

import (
	"image"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"

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

// navRouteGroup returns the group of the found route on the map, above its casing.
func navRouteGroup(a *App) mapview.LineGroup {
	g := a.nav.lines.Groups()
	return g[len(g)-1]
}

func TestNavRouteColoring(t *testing.T) {
	a, fileIt := navFileApp(t)
	// The file is hidden, so the color range comes from the found route alone.
	a.files[0].visible.Value = false
	a.initColoring(nil, "elevation", nil)
	a.toggleNav()
	findNavRoute(a)
	it := a.nav.item
	a.updateProfiles()
	waitProfile(t, a, it)
	waitProfile(t, a, fileIt)
	a.updateTracks()

	if a.legend == nil {
		t.Fatal("expected a legend from the found route")
	}
	g := navRouteGroup(a)
	if g.Values == nil || len(g.Lines) != 1 || len(g.Lines[0].Points) != len(it.points[0]) || a.nav.lines.Coloring() == nil {
		t.Errorf("expected the route colored along its profile")
	}
	if a.chart.item != it || a.chart.coloring == nil {
		t.Errorf("expected the profile colored in the chart")
	}

	// Zoomed in on the route, it is muted, with the shown part highlighted.
	a.chart.setRange(0, a.chart.total/2)
	a.updateTracks()
	a.updateHighlight()
	if g := navRouteGroup(a); g.Values != nil || g.Color != a.style.MutedTrack {
		t.Errorf("expected the route muted while zoomed in")
	}
	if a.highlightKey.item != it || len(a.highlight.Groups()) != 1 {
		t.Errorf("expected the shown part highlighted")
	}

	// Without coloring, the route has its own color.
	a.chart.setRange(0, a.chart.total)
	a.colorBy.SetSelected(metricIndex(track.NoMetric))
	a.updateTracks()
	if g := navRouteGroup(a); g.Values != nil || g.Color != a.style.NavLine || a.legend != nil {
		t.Errorf("expected the route in its own color")
	}
}

func TestNavPanelClick(t *testing.T) {
	a, fileIt := navFileApp(t)
	a.toggleNav()
	findNavRoute(a)
	a.selected = fileIt

	var r input.Router
	var ops op.Ops
	size := image.Pt(800, 600)
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: r.Source(), Now: time.Now(), Constraints: layout.Exact(size)}
		gtx.Metric.PxPerDp, gtx.Metric.PxPerSp = 1, 1
		a.updateNav(gtx, nil)
		a.layoutNavPanel(gtx)
		r.Frame(&ops)
	}
	frame()
	// A press on the panel shows the profile, here on the field of the start, which still gets the press.
	at := f32.Pt(float32(size.X)-float32(a.style.NavWidth)/2, float32(a.style.Spacing)+50)
	r.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: at},
	)
	frame()
	if a.selected != a.nav.item {
		t.Errorf("expected the profile shown")
	}
	if !r.Source().Focused(&a.nav.stops[0].field.ed) {
		t.Errorf("expected the press passed on to the field")
	}
}

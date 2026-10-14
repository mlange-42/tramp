package ui

import (
	"testing"

	"github.com/mlange-42/tramp/internal/settings"
)

func TestViewOptionsState(t *testing.T) {
	a := &App{}
	for i, o := range viewOptions {
		if a.showing(viewOption(i)) != o.def {
			t.Errorf("%s: expected the default %v", o.key, o.def)
		}
	}
	// Missing options get their default, others are taken.
	a.initViewOptions(map[string]bool{"track_colors": false, "waypoint_names": true})
	if a.showing(showTrackColors) || !a.showing(showWaypointNames) || !a.showing(showRouteColors) {
		t.Errorf("unexpected options %v", a.viewOptionsState())
	}
	b := &App{}
	b.initViewOptions(a.viewOptionsState())
	if b.flipped != a.flipped {
		t.Errorf("expected %v after a round trip, got %v", a.flipped, b.flipped)
	}
}

func TestViewOptionMenu(t *testing.T) {
	u := newUITester(t, settings.DiskCache{})
	u.click(u.header(1))
	u.clickItem(1, int(showScaleBar))
	if !u.a.menu.menus[1].open {
		t.Error("expected the menu open after switching an option")
	}
	if u.a.showing(showScaleBar) || u.a.menuItems()[1][showScaleBar].checked {
		t.Error("expected the scale bar off")
	}
	if u.a.State().Show["scale_bar"] {
		t.Error("expected the scale bar off in the settings")
	}
	// The map is drawn without legend box.
	u.frame()
}

func TestTrackColorsOff(t *testing.T) {
	a, f := terrainApp(t)
	rec := trackItemNamed(t, f, "Recorded")
	a.initColoring(nil, "elevation", nil)
	a.selected = rec
	a.updateTracks()
	if trackGroup(t, a, rec).Values == nil || a.legend == nil || a.chart.coloring == nil {
		t.Fatal("expected the track colored")
	}
	a.toggleViewOption(showTrackColors)
	a.updateTracks()
	if trackGroup(t, a, rec).Values != nil {
		t.Error("expected the track not colored on the map")
	}
	// The chart is still colored.
	if a.legend == nil || a.chart.coloring == nil {
		t.Error("expected the chart colored")
	}
}

func TestRoutePointDots(t *testing.T) {
	a, f := editApp(t, testPlanGPX)
	r := f.routeItem(0)
	a.updateTracks()
	if len(r.plainDots) != 2 || len(trackGroup(t, a, r).Dots) != 0 {
		t.Fatalf("expected 2 plain route points not drawn, got %d", len(r.plainDots))
	}
	a.toggleViewOption(showRoutePoints)
	a.updateTracks()
	if len(trackGroup(t, a, r).Dots) != 2 {
		t.Error("expected the route points drawn")
	}
	if a.namesShown(r) {
		t.Error("expected route point names off")
	}
	a.toggleViewOption(showRoutePointNames)
	if !a.namesShown(r) || a.namesShown(f.items[0]) {
		t.Error("expected only route point names on")
	}
}

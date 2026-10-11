package ui

import (
	"context"
	"errors"
	"image"
	"math"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/geocode"
	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/settings"
)

// fakeSearcher returns fixed places, and records the queries.
type fakeSearcher struct {
	places  []geocode.Place
	err     error
	queries []string
}

func (f *fakeSearcher) Search(_ context.Context, query string, _ int) ([]geocode.Place, error) {
	f.queries = append(f.queries, query)
	return f.places, f.err
}

// searchApp returns an app with a map of a fixed size and a fake geocoder.
func searchApp(places []geocode.Place, err error) (*App, *fakeSearcher, layout.Context) {
	a := &App{style: DefaultStyle(), mapView: mapview.New(geo.LonLat{}, 3), invalidate: func() {}}
	a.mapView.View.Size = image.Pt(800, 600)
	f := &fakeSearcher{places: places, err: err}
	a.geocoder = f
	return a, f, layout.Context{Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
}

// waitSearch waits for the background search to finish, and applies its results.
func waitSearch(a *App) {
	for a.search.busy {
		a.bgMu.Lock()
		n := len(a.uiTasks)
		a.bgMu.Unlock()
		if n > 0 {
			a.runUITasks()
		}
	}
}

func near(a, b geo.LonLat) bool {
	return math.Abs(a.Lon-b.Lon) < 1e-6 && math.Abs(a.Lat-b.Lat) < 1e-6
}

func TestSearchCoordinates(t *testing.T) {
	a, f, gtx := searchApp(nil, nil)
	a.search.ed.SetText("N 51° 20.4' E 12° 22.2'")
	a.submitSearch(gtx)
	want := geo.LonLat{Lon: 12.37, Lat: 51.34}
	if !near(geo.ToLonLat(a.mapView.View.Center), want) || a.mapView.View.Zoom != placeZoom {
		t.Errorf("map at %v zoom %v, want %v zoom %v", geo.ToLonLat(a.mapView.View.Center), a.mapView.View.Zoom, want, placeZoom)
	}
	if !a.search.hasMarker || !near(a.search.marker, want) || a.search.open {
		t.Errorf("expected a marker and no result list, got %+v", a.search)
	}
	if len(f.queries) != 0 {
		t.Errorf("coordinates should not be searched, got %v", f.queries)
	}

	// A closer view is kept.
	a.mapView.SetZoom(18)
	a.submitSearch(gtx)
	if a.mapView.View.Zoom != 18 {
		t.Errorf("expected zoom 18 kept, got %v", a.mapView.View.Zoom)
	}

	a.clearSearch()
	if a.search.hasMarker {
		t.Error("marker not cleared")
	}
}

func TestSearchPlaces(t *testing.T) {
	leipzig := geocode.Place{
		Name: "Leipzig", Kind: "city", Pos: geo.LonLat{Lon: 12.37, Lat: 51.34},
		Min: geo.LonLat{Lon: 12.24, Lat: 51.24}, Max: geo.LonLat{Lon: 12.54, Lat: 51.45}, HasBounds: true,
	}
	hut := geocode.Place{Name: "Hut", Kind: "alpine_hut", Pos: geo.LonLat{Lon: 11, Lat: 47}}
	a, f, gtx := searchApp([]geocode.Place{leipzig, hut}, nil)
	a.search.ed.SetText(" Leipzig ")
	a.submitSearch(gtx)
	if !a.search.open || !a.search.busy {
		t.Fatalf("expected a running search with open panel")
	}
	waitSearch(a)
	if len(f.queries) != 1 || f.queries[0] != "Leipzig" {
		t.Errorf("unexpected queries %v", f.queries)
	}
	if len(a.search.places) != 2 || len(a.search.rows) < 2 || a.search.err != nil {
		t.Fatalf("expected 2 places, got %+v", a.search)
	}

	// A place with an extent is shown fully.
	a.gotoPlace(leipzig)
	v := &a.mapView.View
	r := geo.EmptyRect().Extend(geo.ToMercator(leipzig.Min)).Extend(geo.ToMercator(leipzig.Max))
	if !v.Bounds().Contains(r) || a.search.open || !near(a.search.marker, leipzig.Pos) {
		t.Errorf("extent not shown: view %v, place %v", v.Bounds(), r)
	}
	// A place without extent is centered.
	a.gotoPlace(hut)
	if !near(geo.ToLonLat(v.Center), hut.Pos) {
		t.Errorf("expected map centered on %v, got %v", hut.Pos, geo.ToLonLat(v.Center))
	}
}

func TestSearchError(t *testing.T) {
	a, _, gtx := searchApp(nil, errors.New("offline"))
	a.search.ed.SetText("Leipzig")
	a.submitSearch(gtx)
	waitSearch(a)
	if a.search.err == nil || !a.search.open {
		t.Errorf("expected the error shown, got %+v", a.search)
	}
}

func TestSearchOutdated(t *testing.T) {
	a, _, gtx := searchApp([]geocode.Place{{Name: "Old"}}, nil)
	a.search.ed.SetText("Leipzig")
	a.submitSearch(gtx)
	// Clearing before the results arrive drops them.
	a.clearSearch()
	for {
		a.bgMu.Lock()
		n := len(a.uiTasks)
		a.bgMu.Unlock()
		if n > 0 {
			break
		}
	}
	a.runUITasks()
	if len(a.search.places) != 0 || a.search.open {
		t.Errorf("outdated results shown: %+v", a.search)
	}
}

func TestSearchLayout(t *testing.T) {
	a := newApp(func() {}, Options{
		Maps:  DefaultMaps(),
		State: settings.Settings{View: &settings.View{Map: "None", Zoom: 5}},
	})
	defer a.closeTiles()
	a.geocoder = nil

	var r input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: r.Source(), Now: time.Now(), Constraints: layout.Exact(image.Pt(1000, 700))}
		gtx.Metric.PxPerDp, gtx.Metric.PxPerSp = 1, 1
		a.layout(gtx)
		r.Frame(&ops)
	}
	frame()
	a.search.ed.SetText("Leipzig")
	a.search.open, a.search.busy = true, true
	frame()
	a.search.busy = false
	a.search.places = []geocode.Place{{Name: "Leipzig, Sachsen, Deutschland", Kind: "city"}}
	a.search.rows = make([]widget.Clickable, 1)
	a.search.hasMarker = true
	frame()
	frame()
}

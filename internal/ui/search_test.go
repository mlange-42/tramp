package ui

import (
	"context"
	"errors"
	"image"
	"math"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
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
	places      []geocode.Place
	err         error
	interactive bool
	queries     []geocode.Query
}

func (f *fakeSearcher) Attribution() string { return "test" }

func (f *fakeSearcher) Interactive() bool { return f.interactive }

func (f *fakeSearcher) Search(_ context.Context, q geocode.Query) ([]geocode.Place, error) {
	f.queries = append(f.queries, q)
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
	if len(f.queries) != 1 || f.queries[0].Text != "Leipzig" || !f.queries[0].HasNear {
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

func TestSearchService(t *testing.T) {
	cfg := settings.Search{Service: "nominatim", URL: "https://nominatim.example.org/search"}
	a := newApp(func() {}, Options{Maps: DefaultMaps(), State: settings.Settings{View: &settings.View{Map: "None"}, Search: cfg}})
	defer a.closeTiles()
	if a.geocoder.Attribution() != geocode.NominatimAttribution {
		t.Errorf("expected Nominatim, got %q", a.geocoder.Attribution())
	}
	if a.State().Search != cfg {
		t.Errorf("search settings not kept: %+v", a.State().Search)
	}
	// An unknown service falls back to the default, Photon.
	if g := newGeocoder(settings.Search{Service: "unknown"}); g.Attribution() != geocode.PhotonAttribution {
		t.Errorf("expected Photon, got %q", g.Attribution())
	}
}

func TestSearchWhileTyping(t *testing.T) {
	places := []geocode.Place{{Name: "Leipzig"}, {Name: "Leipzig, Ohio"}, {Name: "Leipe"}}
	a, f, gtx := searchApp(places, nil)
	f.interactive = true
	t0 := time.Now()
	// Setting the text reports a change on the next update, like typing.
	typeText := func(text string) {
		a.search.ed.SetText(text)
		gtx.Now = t0
		a.updateSearch(gtx)
	}
	at := func(d time.Duration) {
		gtx.Now = t0.Add(d)
		a.updateSearch(gtx)
	}

	// Short text and coordinates are not searched while typing.
	for _, text := range []string{"Le", "51.3", "N 51 20.4 E", "51°20'"} {
		typeText(text)
		if !a.search.due.IsZero() {
			t.Errorf("%q: expected no search scheduled", text)
		}
	}

	typeText("Leip")
	at(typeDelay - time.Millisecond)
	if a.search.busy || len(f.queries) != 0 {
		t.Fatal("searched before the typing delay")
	}
	at(typeDelay)
	if !a.search.busy || !a.search.open {
		t.Fatal("expected a search after the typing delay")
	}
	waitSearch(a)
	if len(f.queries) != 1 || f.queries[0].Text != "Leip" || len(a.search.places) != 3 || a.search.sel != 0 {
		t.Fatalf("unexpected search %v, places %v", f.queries, a.search.places)
	}

	// The arrow keys select a place.
	a.moveSearchSel(1)
	a.moveSearchSel(1)
	a.moveSearchSel(1)
	if a.search.sel != 2 {
		t.Errorf("expected the last place selected, got %d", a.search.sel)
	}
	a.moveSearchSel(-5)
	if a.search.sel != 0 {
		t.Errorf("expected the first place selected, got %d", a.search.sel)
	}

	// Typing on keeps the results until the new ones arrive.
	typeText("Leipz")
	at(typeDelay)
	if !a.search.busy || len(a.search.places) != 3 {
		t.Errorf("expected the old results kept while searching")
	}
	waitSearch(a)

	// Text that can't be searched while typing drops the results.
	typeText("51")
	if a.search.open || len(a.search.places) != 0 {
		t.Errorf("expected results dropped")
	}
}

func TestSearchOnEnterOnly(t *testing.T) {
	a, f, gtx := searchApp(nil, nil)
	a.search.ed.SetText("Leipzig")
	gtx.Now = time.Now()
	a.updateSearch(gtx)
	if !a.search.due.IsZero() {
		t.Error("a service that is not interactive should not be searched while typing")
	}
	gtx.Now = gtx.Now.Add(time.Second)
	a.updateSearch(gtx)
	if a.search.busy || len(f.queries) != 0 {
		t.Error("unexpected search")
	}
}

func TestSearchKeys(t *testing.T) {
	a := newApp(func() {}, Options{Maps: DefaultMaps(), State: settings.Settings{View: &settings.View{Map: "None", Zoom: 5}}})
	defer a.closeTiles()
	places := []geocode.Place{{Name: "A", Pos: geo.LonLat{Lon: 1, Lat: 1}}, {Name: "B", Pos: geo.LonLat{Lon: 2, Lat: 2}}}
	a.geocoder = &fakeSearcher{places: places}

	var r input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: r.Source(), Now: time.Now(), Constraints: layout.Exact(image.Pt(1000, 700))}
		gtx.Metric.PxPerDp, gtx.Metric.PxPerSp = 1, 1
		a.layout(gtx)
		r.Frame(&ops)
	}
	press := func(name key.Name) {
		r.Queue(key.Event{Name: name, State: key.Press})
		frame()
	}
	frame()
	r.Source().Execute(key.FocusCmd{Tag: &a.search.ed})
	frame()
	a.search.ed.SetText("x")
	press(key.NameReturn)
	waitSearch(a)
	frame()
	if !a.search.open || len(a.search.places) != 2 {
		t.Fatalf("expected the results listed, got %+v", a.search)
	}
	press(key.NameDownArrow)
	if a.search.sel != 1 {
		t.Fatalf("expected the second place selected, got %d", a.search.sel)
	}
	press(key.NameReturn)
	if a.search.open || !a.search.hasMarker || a.search.marker != places[1].Pos {
		t.Errorf("expected the second place shown, got %+v", a.search)
	}
}

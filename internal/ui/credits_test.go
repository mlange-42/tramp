package ui

import (
	"image"
	"slices"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/mlange-42/tramp/internal/settings"
	"github.com/mlange-42/tramp/internal/wms"
)

func TestShortCredits(t *testing.T) {
	cs := []credit{
		{short: "© OpenStreetMap · CyclOSM"},
		{short: "© OpenStreetMap · Waymarked Trails"},
		// Without short credit, the full one is shown.
		{full: "© Some Service"},
		{short: " © OpenStreetMap ·  · CyclOSM "},
		{},
	}
	if got, want := shortCredits(cs), "© OpenStreetMap · CyclOSM · Waymarked Trails · © Some Service"; got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
	if got := shortCredits(nil); got != "" {
		t.Errorf("expected nothing, got %q", got)
	}
}

func TestMapCredits(t *testing.T) {
	a := newApp(func() {}, Options{Maps: DefaultMaps(), Overlays: DefaultOverlays(), State: settings.Settings{
		View: &settings.View{Map: "CyclOSM", Overlays: []string{"Hiking routes", "Hillshade Germany (basemap.de)"}},
	}})
	defer a.closeTiles()
	cs := a.mapCredits()
	var uses []string
	for _, c := range cs {
		uses = append(uses, c.use+": "+c.name)
		if c.full == "" || c.short == "" || !strings.HasPrefix(c.url, "https://") {
			t.Errorf("expected a full and short credit with a link for %s, got %+v", c.name, c)
		}
	}
	want := []string{"Map: CyclOSM", "Overlay: Hillshade Germany (basemap.de)", "Overlay: Hiking routes", "Elevation: Mapterhorn"}
	if !slices.Equal(uses, want) {
		t.Errorf("expected %q, got %q", want, uses)
	}
	if got, want := shortCredits(cs), "© OpenStreetMap · CyclOSM · © basemap.de / BKG · Waymarked Trails · © Mapterhorn"; got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestBuiltinCredits(t *testing.T) {
	// All built-in layers with data have a full and a short credit, and a link.
	for _, m := range DefaultMaps() {
		if l := m.Layer; m.Section == "" && l.Name != "None" && (l.Attribution == "" || l.AttributionShort == "" || l.AttributionURL == "") {
			t.Errorf("map %s lacks credits", l.Name)
		}
	}
	for _, o := range DefaultOverlays() {
		if l := o.Layer; o.Section == "" && (l.Attribution == "" || l.AttributionShort == "" || l.AttributionURL == "") {
			t.Errorf("overlay %s lacks credits", l.Name)
		}
	}
}

func TestAllCredits(t *testing.T) {
	for _, tc := range []struct {
		routing, routingURL, search, searchURL string
		want                                   []string
	}{
		{"", "", "", "", []string{"Routing: Valhalla " + valhallaPage, "Search: Photon " + photonPage}},
		{"osrm", "", "nominatim", "", []string{"Routing: OSRM " + osrmPage, "Search: Nominatim " + nominatimURL}},
		// Other servers link to the license of the OpenStreetMap data.
		{"osrm", "http://localhost:5000", "", "http://localhost:2322/api", []string{"Routing: OSRM " + osmCopyrightURL, "Search: Photon " + osmCopyrightURL}},
	} {
		st := settings.Settings{
			Routing: settings.Routing{Service: tc.routing, URL: tc.routingURL},
			Search:  settings.Search{Service: tc.search, URL: tc.searchURL},
			// No map or elevation, to list only the services.
			Elevation: settings.Elevation{Source: "none"},
		}
		a := newApp(func() {}, Options{Maps: []Map{{Layer: wms.Layer{Name: "None", Type: wms.TypeNone}}}, State: st})
		var got []string
		for _, c := range a.allCredits() {
			got = append(got, c.use+": "+c.name+" "+c.url)
			if c.full == "" {
				t.Errorf("expected a full credit for %s", c.name)
			}
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("expected %q, got %q", tc.want, got)
		}
		a.closeTiles()
	}
}

func TestSourcesList(t *testing.T) {
	// Without map and elevations, so that the test doesn't download anything.
	a := newApp(func() {}, Options{Maps: DefaultMaps(), State: settings.Settings{
		View:      &settings.View{Map: "None", Zoom: 5},
		Elevation: settings.Elevation{Source: "none"},
	}})
	defer a.closeTiles()
	a.geocoder = nil
	var r input.Router
	var ops op.Ops
	size := image.Pt(1000, 700)
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: r.Source(), Now: time.Now(), Constraints: layout.Exact(size)}
		gtx.Metric.PxPerDp, gtx.Metric.PxPerSp = 1, 1
		a.layout(gtx)
		r.Frame(&ops)
	}
	click := func(at f32.Point) {
		r.Queue(
			pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at},
			pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: at},
		)
		frame()
		frame()
	}
	frame()
	// The link is at the right end of the status bar.
	click(f32.Pt(float32(size.X)-15, float32(size.Y)-6))
	if !a.sources.open {
		t.Fatal("expected the list open")
	}
	// The routing service is listed, with a link.
	if n := len(a.sources.credits); n != 1 || len(a.sources.links) != n || a.sources.credits[0].use != "Routing" {
		t.Errorf("expected the routing service, got %+v", a.sources.credits)
	}
	// A click outside closes it, and doesn't reach the map.
	center := a.mapView.View.Center
	click(f32.Pt(300, 300))
	if a.sources.open || a.mapView.View.Center != center {
		t.Error("expected the list closed")
	}
	// The link toggles the list.
	click(f32.Pt(float32(size.X)-15, float32(size.Y)-6))
	click(f32.Pt(float32(size.X)-15, float32(size.Y)-6))
	if a.sources.open {
		t.Error("expected the list closed by the link")
	}
}

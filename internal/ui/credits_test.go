package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/mlange-42/tramp/internal/settings"
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
	want := []string{"Map: CyclOSM", "Overlay: Hillshade Germany (basemap.de)", "Overlay: Hiking routes", "Elevation: mapterhorn"}
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

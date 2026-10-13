package ui

import (
	"strings"
	"testing"

	"github.com/mlange-42/tramp/internal/settings"
	"github.com/mlange-42/tramp/internal/track"
)

// testEleGPX has a track with recorded elevations from 100 to 120 m, and one without.
const testEleGPX = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test" xmlns="http://www.topografix.com/GPX/1/1">
  <trk><name>Recorded</name><trkseg>
    <trkpt lat="51.0" lon="12.0"><ele>100</ele></trkpt>
    <trkpt lat="51.01" lon="12.0"><ele>110</ele></trkpt>
    <trkpt lat="51.02" lon="12.0"><ele>120</ele></trkpt>
  </trkseg></trk>
  <trk><name>Flat</name><trkseg>
    <trkpt lat="51.0" lon="12.1"/>
    <trkpt lat="51.01" lon="12.1"/>
  </trkseg></trk>
</gpx>`

// terrainApp returns an app with testEleGPX opened, the elevation chart, and an elevation service
// with a constant elevation of 123 m.
func terrainApp(t *testing.T) (*App, *openFile) {
	t.Helper()
	srv, _, _ := elevationServer(t)
	a, f := editApp(t, testEleGPX)
	a.style = DefaultStyle()
	a.initColoring(nil, "", nil)
	cfg := settings.Elevation{URL: srv.URL + "/{z}/{x}/{y}.png", TileSize: 4}
	a.elev = elevationView{cfg: cfg, service: newElevationService(cfg, 0)}
	return a, f
}

// trackItemNamed returns the track item with the given name.
func trackItemNamed(t *testing.T, f *openFile, name string) *fileItem {
	t.Helper()
	for _, it := range f.items {
		if it.kind == trackItem && it.name == name {
			return it
		}
	}
	t.Fatalf("no track %s", name)
	return nil
}

func TestTrackTerrain(t *testing.T) {
	a, f := terrainApp(t)
	rec, flat := trackItemNamed(t, f, "Recorded"), trackItemNamed(t, f, "Flat")

	// Only the selected track gets its terrain.
	a.selected = rec
	a.updateProfiles()
	if rec.profile != profileLoading || flat.profile != profileNone {
		t.Fatalf("expected only the selected track requested, got %v and %v", rec.profile, flat.profile)
	}
	waitProfile(t, a, rec)
	if rec.profile != profileDone || len(rec.terrain) != 1 || len(rec.terrain[0]) != 2 || rec.terrain[0][0] != 123 {
		t.Fatalf("expected the terrain per segment, got %v", rec.terrain)
	}
	// The recorded points are kept.
	if rec.points[0][0].Ele != 100 || len(rec.points[0]) != 3 {
		t.Errorf("expected the recorded track unchanged")
	}

	// The chart shows the recorded elevation, with the terrain as a second line within the value range.
	a.updateChart()
	c := &a.chart
	if c.info == nil || c.info.metric != track.ElevationMetric || c.terrain == nil || c.vals[0][0] != 105 {
		t.Fatalf("expected recorded elevation and terrain, hint %q", c.hint)
	}
	if lo, hi, ok := c.valueRange(100); !ok || lo > 106 || hi < 123 {
		t.Errorf("expected the range to cover both lines, got %v to %v", lo, hi)
	}
	a.hover = c.hoverAt(rec.lines, 0, 0, 0.5)
	if s := a.hoverText(); !strings.HasPrefix(s, "108 m · terrain 123 m · ") {
		t.Errorf("unexpected hover text %q", s)
	}

	// Other metrics have no terrain line.
	a.colorBy.SetSelected(metricIndex(track.SlopeMetric))
	a.updateChart()
	if c.terrain != nil {
		t.Errorf("expected no terrain line for slope")
	}
}

func TestTrackTerrainWithoutElevation(t *testing.T) {
	a, f := terrainApp(t)
	flat := trackItemNamed(t, f, "Flat")
	a.selected = flat
	a.updateChart()
	if a.chart.info != nil || a.chart.hint != "No elevation data" {
		t.Fatalf("expected no chart before the profile is requested, hint %q", a.chart.hint)
	}
	a.updateProfiles()
	a.updateChart()
	if a.chart.hint != "Loading elevations…" {
		t.Errorf("expected a loading hint, got %q", a.chart.hint)
	}
	waitProfile(t, a, flat)
	// The terrain is the profile of a track without elevation.
	a.updateChart()
	if a.chart.info == nil || a.chart.terrain != nil || a.chart.vals[0][0] != 123 {
		t.Errorf("expected the terrain as profile, hint %q", a.chart.hint)
	}
}

func TestTerrainOnlyColored(t *testing.T) {
	a, f := terrainApp(t)
	rec, flat := trackItemNamed(t, f, "Recorded"), trackItemNamed(t, f, "Flat")
	a.initColoring(nil, "elevation", nil)
	// The first track was selected when the file was opened.
	a.selected = nil

	// While colored by elevation, a shown track without elevation gets its terrain without being selected.
	a.updateProfiles()
	if flat.profile != profileLoading || rec.profile != profileNone {
		t.Fatalf("expected only the track without elevation requested, got %v and %v", flat.profile, rec.profile)
	}
	waitProfile(t, a, flat)
	if !flat.terrainOnly || flat.lineValues(track.ElevationMetric) == nil || flat.lineValues(track.SlopeMetric) == nil {
		t.Fatal("expected elevation and slope from the terrain")
	}
	a.updateTracks()
	if g := trackGroup(t, a, flat); g.Values == nil || a.legend == nil {
		t.Errorf("expected the track colored on the map")
	}

	// Its profile is colored like the others, without a second line.
	a.selected = flat
	a.updateChart()
	if a.chart.coloring == nil || a.chart.terrain != nil || a.chart.vals[0][0] != 123 {
		t.Errorf("expected the colored terrain profile")
	}
}

func TestHiddenItemNotColored(t *testing.T) {
	a, f := terrainApp(t)
	rec := trackItemNamed(t, f, "Recorded")
	a.initColoring(nil, "elevation", nil)
	a.selected = rec
	a.updateTracks()
	if a.chart.coloring == nil {
		t.Fatal("expected the shown track's profile colored")
	}
	// Hidden, its profile is not colored, though the other track still gives a legend.
	rec.visible.Value = false
	a.updateProfiles()
	waitProfile(t, a, trackItemNamed(t, f, "Flat"))
	a.updateTracks()
	if a.legend == nil {
		t.Fatal("expected a legend from the other track")
	}
	if a.chart.item != rec || a.chart.coloring != nil {
		t.Errorf("expected the hidden track's profile in its own color")
	}
}

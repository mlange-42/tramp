package ui

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/settings"
	"github.com/mlange-42/tramp/internal/track"
)

const testGPX = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test" xmlns="http://www.topografix.com/GPX/1/1">
  <wpt lat="51.5" lon="12.5"><name>Hut</name></wpt>
  <wpt lat="51.6" lon="12.6"/>
  <rte><rtept lat="51.0" lon="12.0"/><rtept lat="51.1" lon="12.0"/></rte>
  <trk><name>Morning</name><trkseg>
    <trkpt lat="51.0" lon="12.0"><time>2026-10-07T09:00:00Z</time></trkpt>
    <trkpt lat="51.01" lon="12.0"><time>2026-10-07T09:00:02Z</time></trkpt>
    <trkpt lat="51.02" lon="12.0"><time>2026-10-07T09:00:04Z</time></trkpt>
    <trkpt lat="51.03" lon="12.0"><time>2026-10-07T10:30:04Z</time></trkpt>
  </trkseg></trk>
</gpx>`

const testTrackGPX = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.0" creator="test">
  <trk><trkseg>
    <trkpt lat="52.0" lon="13.0"/>
    <trkpt lat="52.001" lon="13.0"/>
  </trkseg></trk>
</gpx>`

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNewOpenFile(t *testing.T) {
	path := writeTemp(t, "mixed.gpx", testGPX)
	data, err := track.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f := newOpenFile(path, data)
	if f.name != "mixed.gpx" || f.summary != "1 track · 1 route · 2 waypoints" || !f.expandable() || !f.visible.Value {
		t.Errorf("unexpected file %q %q", f.name, f.summary)
	}
	want := [][2]string{
		{"Morning", "3.3 km · 1:30 h · 2 s"},
		{"Route", "route · 11.1 km"},
		{"Waypoints", "2 waypoints"},
	}
	if len(f.items) != len(want) {
		t.Fatalf("expected %d items, got %d", len(want), len(f.items))
	}
	for i, it := range f.items {
		if it.name != want[i][0] || it.summary != want[i][1] || !it.visible.Value || it.bounds.Empty() {
			t.Errorf("item %d: got %q %q", i, it.name, it.summary)
		}
	}
	if b := f.bounds; !b.Intersects(f.items[2].bounds) || b.Min.Y > geo.ToMercator(geo.LonLat{Lon: 12, Lat: 51}).Y {
		t.Errorf("unexpected file bounds %v", b)
	}
}

func TestFormat(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{formatDistance(512.4), "512 m"},
		{formatDistance(48_695), "48.7 km"},
		{formatDuration(35*time.Minute + 20*time.Second), "35 min"},
		{formatDuration(4*time.Hour + 50*time.Minute), "4:50 h"},
		{count(1, "route"), "1 route"},
		{count(3, "route"), "3 routes"},
		{itemName("", "Track", 1, 3), "Track 2"},
		{itemName("", "Track", 0, 1), "Track"},
		{itemName("Day 1", "Track", 0, 3), "Day 1"},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestOpenFiles(t *testing.T) {
	loaded := make(chan struct{}, 10)
	a := &App{
		invalidate: func() { loaded <- struct{}{} },
		mapView:    mapview.New(geo.LonLat{}, 0),
		pending:    map[int][]settings.File{},
	}
	mixed := writeTemp(t, "mixed.gpx", testGPX)
	plain := writeTemp(t, "plain.gpx", testTrackGPX)
	missing := filepath.Join(t.TempDir(), "missing.gpx")

	a.openFiles([]settings.File{{Path: mixed}, {Path: missing}, {Path: plain, Hidden: true}}, false)
	// Files still being read are saved.
	if s := a.fileState(); len(s) != 3 {
		t.Errorf("expected 3 pending files, got %v", s)
	}
	<-loaded
	a.addLoaded()

	if len(a.files) != 2 || a.files[0].path != mixed || a.files[1].path != plain {
		t.Fatalf("unexpected files %v", a.files)
	}
	if !a.files[0].visible.Value || a.files[1].visible.Value || !a.tracksChanged {
		t.Errorf("unexpected visibility")
	}
	if s := a.fileState(); len(s) != 2 || s[0] != (settings.File{Path: mixed}) || s[1] != (settings.File{Path: plain, Hidden: true}) {
		t.Errorf("unexpected state %v", s)
	}

	// Hidden files and items are not drawn.
	a.files[0].items[1].visible.Value = false
	a.updateTracks()

	// Opening an open file again doesn't add it, but shows it.
	a.mapView.View.Size = image.Pt(800, 600)
	zoom := a.mapView.View.Zoom
	a.openFiles([]settings.File{{Path: plain}}, true)
	<-loaded
	a.addLoaded()
	if len(a.files) != 2 {
		t.Errorf("expected 2 files, got %d", len(a.files))
	}
	if a.mapView.View.Zoom == zoom {
		t.Errorf("expected the map to show the file")
	}
}

package ui

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

	saved := []string{"#010203", "#040506", "#070809"}
	a.openFiles([]settings.File{{Path: mixed, Colors: saved}, {Path: missing}, {Path: plain, Hidden: true}}, false)
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
	// Saved colors are restored, files without get the first default color.
	want := []settings.File{
		{Path: mixed, Colors: saved},
		{Path: plain, Hidden: true, Colors: []string{formatColor(trackColors[0])}},
	}
	if s := a.fileState(); !reflect.DeepEqual(s, want) {
		t.Errorf("unexpected state %v", s)
	}
	if c := a.files[0].colors(); len(c) != 3 {
		t.Errorf("expected 3 distinct colors, got %v", c)
	}

	// A color chosen for a whole file applies to all its items.
	a.colorChanges = []colorChange{{items: a.files[0].items, color: trackColors[3]}}
	a.tracksChanged = false
	a.applyColors()
	if c := a.files[0].colors(); len(c) != 1 || c[0] != trackColors[3] || !a.tracksChanged {
		t.Errorf("expected a single changed color, got %v", c)
	}

	// Hidden files and items are not drawn, the top-most entry is drawn last.
	a.files[0].items[1].visible.Value = false
	a.files[1].visible.Value = true
	for i, it := range a.files[0].items {
		it.color = color.NRGBA{R: uint8(i), A: 0xff}
	}
	a.files[1].items[0].color = color.NRGBA{R: 9, A: 0xff}
	var order []uint8
	for _, g := range a.trackGroups() {
		order = append(order, g.Color.R)
	}
	if !slices.Equal(order, []uint8{9, 2, 0}) {
		t.Errorf("unexpected drawing order %v", order)
	}
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

func TestParseColor(t *testing.T) {
	for _, c := range trackColors {
		got, err := parseColor(formatColor(c))
		if err != nil || got != c {
			t.Errorf("round trip of %v gave %v, %v", c, got, err)
		}
	}
	for _, s := range []string{"", "e01010", "#e0101", "#e0101g", "#e01010ff"} {
		if _, err := parseColor(s); err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
}

func TestSetColors(t *testing.T) {
	f := &openFile{items: []*fileItem{{}, {}}}
	def := color.NRGBA{R: 1, A: 0xff}
	for _, c := range []struct {
		saved []string
		want  []color.NRGBA
	}{
		{nil, []color.NRGBA{def, def}},
		{[]string{"#000010", "#000020"}, []color.NRGBA{{B: 0x10, A: 0xff}, {B: 0x20, A: 0xff}}},
		// The file changed, or the colors are invalid.
		{[]string{"#000010"}, []color.NRGBA{def, def}},
		{[]string{"#000010", "blue"}, []color.NRGBA{def, def}},
	} {
		f.setColors(c.saved, def)
		for i, it := range f.items {
			if it.color != c.want[i] {
				t.Errorf("%v: item %d: got %v, want %v", c.saved, i, it.color, c.want[i])
			}
		}
	}
}

func TestItemOrder(t *testing.T) {
	f := &openFile{data: &track.File{}, items: []*fileItem{{}, {}, {}}}
	f.update()
	if o := f.order(); o != nil {
		t.Errorf("expected nil for file order, got %v", o)
	}
	f.setOrder([]int{2, 0, 1})
	if o := f.order(); !slices.Equal(o, []int{2, 0, 1}) {
		t.Errorf("unexpected order %v", o)
	}
	// Invalid orders are ignored.
	for _, o := range [][]int{{0, 1}, {0, 0, 1}, {0, 1, 3}, {-1, 0, 1}} {
		f.setOrder(o)
		if got := f.order(); !slices.Equal(got, []int{2, 0, 1}) {
			t.Errorf("%v: order changed to %v", o, got)
		}
	}
}

func TestItemOrderState(t *testing.T) {
	loaded := make(chan struct{}, 10)
	a := &App{
		invalidate: func() { loaded <- struct{}{} },
		mapView:    mapview.New(geo.LonLat{}, 0),
		pending:    map[int][]settings.File{},
	}
	path := writeTemp(t, "mixed.gpx", testGPX)
	saved := settings.File{Path: path, Colors: []string{"#000001", "#000002", "#000003"}, Order: []int{2, 0, 1}}
	a.openFiles([]settings.File{saved}, false)
	<-loaded
	a.addLoaded()

	// Colors are saved in file order, independent of the panel order.
	f := a.files[0]
	if f.items[0].name != "Waypoints" || f.items[0].color.B != 3 {
		t.Errorf("unexpected first item %q %v", f.items[0].name, f.items[0].color)
	}
	if s := a.fileState(); !reflect.DeepEqual(s, []settings.File{saved}) {
		t.Errorf("unexpected state %v", s)
	}
}

func TestDragMove(t *testing.T) {
	for _, c := range []struct {
		dy         float32
		prev, next int
		want       int
	}{
		{0, 40, 40, 0},
		{19, 40, 40, 0},
		{21, 40, 40, 1},
		{-21, 40, 40, -1},
		{-19, 40, 40, 0},
		// No neighbor on that side.
		{100, 40, 0, 0},
		{-100, 0, 40, 0},
		// After moving down past a row of 40, the pointer is 40 higher relative to the handle.
		// It must not move back up right away.
		{21 - 40, 40, 60, 0},
	} {
		if got := dragMove(c.dy, c.prev, c.next); got != c.want {
			t.Errorf("dragMove(%v, %d, %d) = %d, want %d", c.dy, c.prev, c.next, got, c.want)
		}
	}
}

func TestLineSizes(t *testing.T) {
	data, err := track.ReadFile(writeTemp(t, "mixed.gpx", testGPX))
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(func() {}, Options{
		Layers: DefaultLayers(),
		State:  settings.Settings{Style: settings.Style{TrackWidth: 4, RouteWidth: 2, WaypointSize: 10}},
	})
	defer a.closeTiles()
	a.files = []*openFile{newOpenFile("mixed.gpx", data)}
	// Drawn bottom up: waypoints, route, track.
	groups := a.trackGroups()
	if len(groups) != 3 || groups[0].DotSize != 10 || groups[1].Width != 2 || groups[2].Width != 4 {
		t.Errorf("unexpected sizes %+v", groups)
	}
	if st := a.State().Style; st.TrackWidth != 4 || st.RouteWidth != 2 || st.WaypointSize != 10 {
		t.Errorf("unexpected saved style %+v", st)
	}

	// Missing sizes fall back to the defaults.
	b := newApp(func() {}, Options{Layers: DefaultLayers()})
	defer b.closeTiles()
	if b.trackWidth != settings.DefaultTrackWidth || b.routeWidth != settings.DefaultRouteWidth || b.waypointSize != settings.DefaultWaypointSize {
		t.Errorf("unexpected default sizes %v %v %v", b.trackWidth, b.routeWidth, b.waypointSize)
	}
}

func TestViewState(t *testing.T) {
	layers := DefaultLayers()
	overlays := DefaultOverlays()
	view := &settings.View{Lon: 12.4, Lat: 51.3, Zoom: 12, Map: layers[1].Name, Overlays: []string{overlays[2].Layer.Name, "unknown"}}
	a := newApp(func() {}, Options{
		Layers:   layers,
		Overlays: overlays,
		State:    settings.Settings{View: view, Window: &settings.Window{Width: 800, Height: 600, PanelWidth: 320}},
	})
	defer a.closeTiles()
	s := a.State()
	if s.View.Map != layers[1].Name || !slices.Equal(s.View.Overlays, []string{overlays[2].Layer.Name}) || s.View.Zoom != 12 {
		t.Errorf("unexpected view %+v", s.View)
	}
	if s.Window.PanelWidth != 320 {
		t.Errorf("unexpected panel width %v", s.Window.PanelWidth)
	}
}

func TestRangeLines(t *testing.T) {
	// Two lines along the x axis: 0-20 m in two segments, and 20-30 m in one.
	pt := func(x float64) geo.Point { return geo.Point{X: x} }
	lines := []mapview.Polyline{
		mapview.NewPolyline([]geo.Point{pt(0), pt(10), pt(20)}),
		mapview.NewPolyline([]geo.Point{pt(20), pt(30)}),
	}
	dist := [][]float64{{0, 10, 20}, {20, 30}}
	vals := [][]float64{{1, 2}, {3}}

	got, gotVals := rangeLines(lines, dist, vals, 5, 25)
	if len(got) != 2 {
		t.Fatalf("got %d lines", len(got))
	}
	if want := []geo.Point{pt(5), pt(10), pt(20)}; !slices.Equal(got[0].Points, want) {
		t.Errorf("first line %v, want %v", got[0].Points, want)
	}
	if want := []geo.Point{pt(20), pt(25)}; !slices.Equal(got[1].Points, want) {
		t.Errorf("second line %v, want %v", got[1].Points, want)
	}
	if !reflect.DeepEqual(gotVals, [][]float64{{1, 2}, {3}}) {
		t.Errorf("values %v", gotVals)
	}

	// A range within a single segment.
	got, gotVals = rangeLines(lines, dist, vals, 12, 18)
	if len(got) != 1 || !slices.Equal(got[0].Points, []geo.Point{pt(12), pt(18)}) || !reflect.DeepEqual(gotVals, [][]float64{{2}}) {
		t.Errorf("got %v, values %v", got, gotVals)
	}

	// Without values.
	if _, gotVals = rangeLines(lines, dist, nil, 0, 30); gotVals != nil {
		t.Errorf("values %v, want nil", gotVals)
	}
}

func TestParseColorAlpha(t *testing.T) {
	for _, c := range []color.NRGBA{{R: 0x80, G: 0x80, B: 0x80, A: 0xc0}, trackColors[0]} {
		got, err := parseColorAlpha(formatColorAlpha(c))
		if err != nil || got != c {
			t.Errorf("round trip of %v gave %v, %v", c, got, err)
		}
	}
	if s := formatColorAlpha(trackColors[0]); len(s) != 7 {
		t.Errorf("opaque color formatted as %q, want #rrggbb", s)
	}
	for _, s := range []string{"", "#e0101", "#e01010f", "#e01010gg", "#e0101gff"} {
		if _, err := parseColorAlpha(s); err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
}

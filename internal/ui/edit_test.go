package ui

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/settings"
	"github.com/mlange-42/tramp/internal/track"
)

// testPlanGPX is a planning file, with a route and waypoints but no tracks.
const testPlanGPX = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test" xmlns="http://www.topografix.com/GPX/1/1">
  <wpt lat="51.5" lon="12.5"><name>Hut</name></wpt>
  <wpt lat="51.6" lon="12.6"/>
  <rte><rtept lat="51.0" lon="12.0"/><rtept lat="51.1" lon="12.0"/></rte>
</gpx>`

// editApp returns an app with a file of the given content opened.
func editApp(t *testing.T, content string) (*App, *openFile) {
	t.Helper()
	loaded := make(chan struct{}, 10)
	a := &App{
		invalidate: func() { loaded <- struct{}{} },
		mapView:    mapview.New(geo.LonLat{}, 0),
		pending:    map[int][]settings.File{},
	}
	a.openFiles([]settings.File{{Path: writeTemp(t, "test.gpx", content)}}, false)
	<-loaded
	a.addLoaded()
	return a, a.files[0]
}

func addRoute(d *track.File) {
	d.Routes = append(d.Routes, track.Route{Name: "New", Points: []track.Waypoint{
		{Pos: geo.LonLat{Lon: 13, Lat: 52}, Ele: math.NaN()},
		{Pos: geo.LonLat{Lon: 13.1, Lat: 52}, Ele: math.NaN()},
	}})
}

func TestRebuild(t *testing.T) {
	_, f := editApp(t, testGPX)
	// Panel order: waypoints, track, route.
	f.setOrder([]int{2, 0, 1})
	wps, route := f.items[0], f.items[2]
	wps.visible.Value = false
	route.lineValues(track.ElevationMetric)

	f.change(func(d *track.File) {
		d.Waypoints = d.Waypoints[:1]
		d.Routes[0].Points = append(d.Routes[0].Points, track.Waypoint{Pos: geo.LonLat{Lon: 12, Lat: 51.2}, Ele: math.NaN()})
		addRoute(d)
	})
	if len(f.items) != 4 || f.items[0] != wps || f.items[2] != route || f.items[3].name != "New" {
		t.Fatalf("unexpected items %v", f.items)
	}
	if wps.visible.Value || wps.summary != "1 waypoint" || !f.items[3].visible.Value {
		t.Errorf("unexpected item state")
	}
	if route.values != nil || len(route.points[0]) != 3 {
		t.Errorf("expected changed route with cleared caches")
	}
	if f.items[3].color != f.items[0].color || f.items[3].index != 2 || route.index != 1 || wps.index != 3 {
		t.Errorf("unexpected color or index of new item")
	}
	if f.summary != "1 track · 2 routes · 1 waypoint" {
		t.Errorf("unexpected summary %q", f.summary)
	}
}

func TestUndoRedo(t *testing.T) {
	a, f := editApp(t, testPlanGPX)
	a.toggleEdit(f)
	if a.editing != f || f.dirty() {
		t.Fatalf("expected clean edit session")
	}
	f.change(addRoute)
	f.change(func(d *track.File) { d.Waypoints = nil })
	if !f.dirty() || len(f.data.Routes) != 2 || len(f.items) != 2 {
		t.Fatalf("unexpected state after changes")
	}
	a.undo()
	if len(f.data.Waypoints) != 2 || len(f.items) != 3 || !f.dirty() {
		t.Errorf("expected waypoints back")
	}
	a.undo()
	if len(f.data.Routes) != 1 || f.dirty() {
		t.Errorf("expected original state, which is not dirty")
	}
	if f.undo() {
		t.Errorf("expected nothing to undo")
	}
	a.redo()
	if len(f.data.Routes) != 2 || !f.dirty() {
		t.Errorf("expected route again")
	}
	// Undo snapshots are not changed by later changes.
	f.change(func(d *track.File) { d.Routes[0].Points[0].Name = "changed" })
	a.undo()
	if f.data.Routes[0].Points[0].Name != "" {
		t.Errorf("snapshot was changed")
	}

	f.discard()
	if len(f.data.Routes) != 1 || len(f.data.Waypoints) != 2 || f.dirty() || f.undo() {
		t.Errorf("expected saved state after discard")
	}
	// Ending the session of a clean file doesn't ask.
	a.toggleEdit(f)
	if a.editing != nil {
		t.Errorf("expected editing to end")
	}
}

func TestSaveJob(t *testing.T) {
	a, f := editApp(t, testPlanGPX)
	a.toggleEdit(f)
	f.change(addRoute)
	job := a.newSaveJob(f)
	if job.creator != "test" {
		t.Errorf("expected confirmation for a foreign file, got %q", job.creator)
	}
	// Changes after preparing the job are not saved.
	f.change(func(d *track.File) { d.Routes[1].Name = "later" })

	job.creator = ""
	job.path = filepath.Join(t.TempDir(), "saved.gpx")
	apply := a.runSave(job)
	if apply == nil {
		t.Fatal("saving failed")
	}
	apply()
	if f.path != job.path || f.name != "saved.gpx" || !f.dirty() || a.newSaveJob(f).creator != "" {
		t.Errorf("unexpected state after save: %q dirty %v", f.path, f.dirty())
	}
	a.undo()
	if f.dirty() {
		t.Errorf("expected the saved state to be clean")
	}

	saved, err := track.ReadFile(job.path)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Routes) != 2 || saved.Routes[1].Name != "New" || len(saved.Waypoints) != 2 || saved.Creator != track.Creator {
		t.Errorf("unexpected saved content %+v", saved)
	}
}

func TestCloseFile(t *testing.T) {
	a, f := editApp(t, testPlanGPX)
	a.selected = f.items[0]
	a.toggleEdit(f)
	a.closeFile(f)
	if len(a.files) != 0 || a.editing != nil || a.selected != nil {
		t.Errorf("expected file closed")
	}
}

func TestEditable(t *testing.T) {
	// Files with recorded tracks are not edited.
	a, f := editApp(t, testGPX)
	if f.editable() {
		t.Errorf("expected a file with tracks not to be editable")
	}
	a.toggleEdit(f)
	if a.editing != nil {
		t.Errorf("expected no edit mode")
	}
	if _, f := editApp(t, testPlanGPX); !f.editable() {
		t.Errorf("expected a planning file to be editable")
	}
}

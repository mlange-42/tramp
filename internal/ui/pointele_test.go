package ui

import (
	"math"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/settings"
	"github.com/mlange-42/tramp/internal/track"
)

// pointEleApp returns an app editing testPlanGPX, with an elevation service with a constant elevation of 123 m.
// Its waypoint at 51.6°N has no elevation.
func pointEleApp(t *testing.T) (*App, *openFile) {
	t.Helper()
	srv, _, _ := elevationServer(t)
	a, f := editApp(t, testPlanGPX)
	cfg := settings.Elevation{URL: srv.URL + "/{z}/{x}/{y}.png", TileSize: 4}
	a.elev = elevationView{cfg: cfg, service: newElevationService(cfg, 0)}
	a.toggleEdit(f)
	return a, f
}

// waitEle waits until the waypoint i of the file has an elevation.
func waitEle(t *testing.T, a *App, f *openFile, i int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for math.IsNaN(f.data.Waypoints[i].Ele) {
		if time.Now().After(deadline) {
			t.Fatal("timeout")
		}
		a.runUITasks()
		time.Sleep(time.Millisecond)
	}
}

func TestNewPointElevation(t *testing.T) {
	a, f := pointEleApp(t)
	p := geo.LonLat{Lon: 12.7, Lat: 51.7}
	f.change(func(d *track.File) {
		d.Waypoints = append(d.Waypoints, track.Waypoint{Pos: p, Ele: math.NaN()})
	})
	// Only the new point is noted, not the one without elevation from before.
	if len(f.edit.needEle) != 1 || f.edit.needEle[0] != p {
		t.Fatalf("expected the new point noted, got %v", f.edit.needEle)
	}
	a.updatePointElevations()
	waitEle(t, a, f, 2)
	if f.data.Waypoints[2].Ele != 123 || !math.IsNaN(f.data.Waypoints[1].Ele) {
		t.Errorf("expected only the new point with elevation, got %v", f.data.Waypoints)
	}
	if len(f.edit.undo) != 1 || !f.dirty() {
		t.Errorf("expected one change to undo, got %d", len(f.edit.undo))
	}

	// Undo removes the point, and redo brings it back with its elevation.
	a.undo()
	a.redo()
	if f.data.Waypoints[2].Ele != 123 {
		t.Errorf("expected the elevation redone, got %v", f.data.Waypoints[2].Ele)
	}

	// A moved point gets the elevation of its new position, also as route point linked to it.
	f.change(func(d *track.File) {
		d.Waypoints[0].Pos, d.Waypoints[0].Ele = geo.LonLat{Lon: 12.8, Lat: 51.8}, math.NaN()
		d.Routes[0].Points[0] = d.Waypoints[0]
	})
	a.updatePointElevations()
	waitEle(t, a, f, 0)
	if f.data.Waypoints[0].Ele != 123 || f.data.Routes[0].Points[0].Ele != 123 {
		t.Errorf("expected the moved point with elevation")
	}
}

func TestPointElevationRedoneEarly(t *testing.T) {
	a, f := pointEleApp(t)
	f.change(func(d *track.File) {
		d.Waypoints = append(d.Waypoints, track.Waypoint{Pos: geo.LonLat{Lon: 12.7, Lat: 51.7}, Ele: math.NaN()})
	})
	// Undone before the elevation is looked up, and redone.
	a.undo()
	f.edit.needEle = nil
	a.redo()
	a.updatePointElevations()
	waitEle(t, a, f, 2)
}

func TestPointElevationAfterSave(t *testing.T) {
	a, f := pointEleApp(t)
	p := geo.LonLat{Lon: 12.7, Lat: 51.7}
	f.change(func(d *track.File) {
		d.Waypoints = append(d.Waypoints, track.Waypoint{Pos: p, Ele: math.NaN()})
	})
	// Saved before the elevation arrived, so the file is changed again by it.
	f.edit.saved, f.edit.savedRev = f.snapshot(), f.edit.rev
	a.applyPointElevations(f, []geo.LonLat{p}, []float64{123})
	if f.data.Waypoints[2].Ele != 123 || !f.dirty() {
		t.Errorf("expected the elevation set and the file changed")
	}
	// Points that have an elevation keep it.
	a.applyPointElevations(f, []geo.LonLat{p}, []float64{200})
	if f.data.Waypoints[2].Ele != 123 {
		t.Errorf("expected the elevation kept, got %v", f.data.Waypoints[2].Ele)
	}
}

func TestPointElevationOff(t *testing.T) {
	a, f := editApp(t, testPlanGPX)
	a.toggleEdit(f)
	f.change(func(d *track.File) {
		d.Waypoints = append(d.Waypoints, track.Waypoint{Pos: geo.LonLat{Lon: 12.7, Lat: 51.7}, Ele: math.NaN()})
	})
	a.updatePointElevations()
	if len(f.edit.needEle) != 0 {
		t.Errorf("expected the noted points dropped without elevation service")
	}
}

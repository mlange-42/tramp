package ui

import (
	"image"
	"testing"

	"gioui.org/layout"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/mapview"
)

func TestProps(t *testing.T) {
	a, f := editApp(t, testLinkedGPX)
	a.style = DefaultStyle()
	a.mapView = mapview.New(geo.LonLat{Lon: 12, Lat: 51}, 9)
	a.mapView.View.Size = image.Pt(800, 600)
	a.toggleEdit(f)
	var gtx layout.Context
	d := f.data
	fields := &a.props.fields

	// Nothing selected, no fields.
	a.selected = nil
	a.updateProps(gtx)
	for i := range fields {
		if fields[i].bound {
			t.Fatalf("field %d bound without selection", i)
		}
	}

	// A selected route point shows its route and itself.
	a.selectVertex(vertex{0, 0})
	a.updateProps(gtx)
	if !fields[routeNameField].bound || fields[pointNameField].ed.Text() != "A" {
		t.Fatalf("unexpected fields")
	}

	// Renaming a linked point renames all points linked to it.
	fields[pointNameField].ed.SetText(" Alpha ")
	a.commitProps()
	for _, v := range []vertex{{-1, 0}, {0, 0}, {0, 3}} {
		if n := waypointRef(d, v).Name; n != "Alpha" {
			t.Errorf("%v: expected Alpha, got %q", v, n)
		}
	}
	if !f.dirty() || len(linkedGroup(d, vertex{-1, 0})) != 3 {
		t.Errorf("expected a change, and points still linked")
	}

	// Typed text is applied before the selection changes.
	fields[routeNameField].ed.SetText("Loop")
	a.selectVertex(vertex{-1, 1})
	a.updateProps(gtx)
	if d.Routes[0].Name != "Loop" || fields[routeNameField].bound || fields[pointNameField].ed.Text() != "B" {
		t.Errorf("expected route renamed and waypoint B shown, got %q", d.Routes[0].Name)
	}

	// Fields follow undo.
	a.undo()
	a.updateProps(gtx)
	if d.Routes[0].Name != "" || fields[pointNameField].ed.Text() != "B" {
		t.Errorf("expected route name undone")
	}
	a.undo()
	a.selectVertex(vertex{0, 0})
	a.updateProps(gtx)
	if fields[pointNameField].ed.Text() != "A" {
		t.Errorf("expected name A after undo, got %q", fields[pointNameField].ed.Text())
	}

	// Unchanged text is no change.
	n := len(f.edit.undo)
	a.commitProps()
	if len(f.edit.undo) != n {
		t.Errorf("expected no change")
	}

	// The route can be deleted.
	a.deleteRoute()
	a.updateProps(gtx)
	if len(d.Routes) != 0 || a.editor.hasSel || fields[routeNameField].bound {
		t.Errorf("expected route deleted")
	}
	if len(d.Waypoints) != 2 {
		t.Errorf("expected waypoints kept")
	}
}

package ui

import (
	"image"
	"testing"

	"gioui.org/layout"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/symbols"
)

func TestSymbolPicker(t *testing.T) {
	a, f := editApp(t, testLinkedGPX)
	a.style = DefaultStyle()
	a.mapView = mapview.New(geo.LonLat{Lon: 12, Lat: 51}, 9)
	a.mapView.View.Size = image.Pt(800, 600)
	a.toggleEdit(f)
	var gtx layout.Context
	d := f.data
	p := &a.props.sym

	a.selectVertex(vertex{0, 0})
	a.updateProps(gtx)
	p.open, p.target = true, propTarget{0, 0}

	// A symbol is set for all linked points.
	a.applySymbol("Summit")
	for _, v := range []vertex{{-1, 0}, {0, 0}, {0, 3}} {
		if s := waypointRef(d, v).Symbol; s != "Summit" {
			t.Errorf("%v: expected Summit, got %q", v, s)
		}
	}
	if waypointRef(d, vertex{-1, 1}).Symbol != "" || len(linkedGroup(d, vertex{-1, 0})) != 3 {
		t.Errorf("expected other points unchanged, and points still linked")
	}
	// The same symbol again is no change.
	n := len(f.edit.undo)
	a.applySymbol("Summit")
	if len(f.edit.undo) != n {
		t.Errorf("expected no change")
	}
	a.undo()
	if waypointRef(d, vertex{0, 3}).Symbol != "" {
		t.Errorf("expected symbol undone")
	}

	// The picker closes when the selection changes.
	a.selectVertex(vertex{-1, 1})
	a.updateProps(gtx)
	if p.open {
		t.Errorf("expected picker closed")
	}
}

func TestCanonicalSymbol(t *testing.T) {
	for _, c := range [][2]string{{"peak", "Summit"}, {" flag blue ", "Flag, Blue"}, {" My Thing ", "My Thing"}, {"", ""}} {
		if got := canonicalSymbol(c[0]); got != c[1] {
			t.Errorf("canonicalSymbol(%q) = %q, want %q", c[0], got, c[1])
		}
	}
}

func TestSymbolRows(t *testing.T) {
	all := symbols.All()
	rows := symbolRows("", 4)
	// None first, then a heading and full rows of cells.
	if len(rows[0].cells) != 1 || rows[0].cells[0] != -1 || rows[1].heading != "Outdoors" || len(rows[2].cells) != 4 {
		t.Fatalf("unexpected first rows %+v", rows[:3])
	}
	n := 0
	for _, r := range rows {
		if len(r.cells) > 4 || (r.heading != "" && len(r.cells) > 0) {
			t.Errorf("unexpected row %+v", r)
		}
		for _, i := range r.cells {
			if i >= 0 && all[i].Icon == nil {
				t.Errorf("symbol without icon %q in grid", all[i].Name)
			}
			n++
		}
	}
	if n != len(all) { // All but "Waypoint", plus None.
		t.Errorf("expected %d cells, got %d", len(all), n)
	}

	rows = symbolRows("hut", 4)
	if len(rows) != 2 || rows[0].heading != "Outdoors" || len(rows[1].cells) != 1 || all[rows[1].cells[0]].Name != "Lodging" {
		t.Errorf("unexpected search rows %+v", rows)
	}
	if rows := symbolRows("xyz", 4); len(rows) != 0 {
		t.Errorf("expected no rows, got %+v", rows)
	}
}

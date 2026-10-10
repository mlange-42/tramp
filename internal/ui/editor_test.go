package ui

import (
	"image"
	"math"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/unit"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/mapview"
)

// editorApp returns an app editing the planning test file, with a map showing it.
func editorApp(t *testing.T) (*App, *openFile, layout.Context) {
	t.Helper()
	a, f := editApp(t, testPlanGPX)
	a.style = DefaultStyle()
	a.mapView = mapview.New(geo.LonLat{Lon: 12.3, Lat: 51.3}, 9)
	a.mapView.View.Size = image.Pt(800, 600)
	a.toggleEdit(f)
	return a, f, layout.Context{Metric: unit.Metric{PxPerDp: 1}}
}

// at returns a pointer event of the given kind at a position.
func at(a *App, kind pointer.Kind, ll geo.LonLat) mapview.PointerEvent {
	p := geo.ToMercator(ll)
	x, y := a.mapView.View.ToScreen(p)
	return mapview.PointerEvent{Kind: kind, Pos: p, Screen: f32.Pt(float32(x), float32(y))}
}

func click(a *App, gtx layout.Context, ll geo.LonLat) {
	a.editPress(gtx, at(a, pointer.Press, ll))
	a.editRelease()
}

func TestHitTest(t *testing.T) {
	a, f, _ := editorApp(t)
	view := &a.mapView.View
	route := f.data.Routes[0].Points
	p := screenPos(view, route[1].Pos).Add(f32.Pt(3, 0))
	if v, ok := hitVertex(f.data, view, p, 5, -1); !ok || v != (vertex{0, 1}) {
		t.Errorf("expected route point 1, got %v %v", v, ok)
	}
	if v, ok := hitVertex(f.data, view, screenPos(view, f.data.Waypoints[1].Pos), 5, -1); !ok || v != (vertex{-1, 1}) {
		t.Errorf("expected waypoint 1, got %v %v", v, ok)
	}
	if _, ok := hitVertex(f.data, view, p.Add(f32.Pt(10, 0)), 5, -1); ok {
		t.Errorf("expected no hit")
	}
	mid := screenPos(view, route[0].Pos).Add(screenPos(view, route[1].Pos)).Mul(0.5)
	if v, ok := hitMidpoint(f.data, view, mid, 5); !ok || v != (vertex{0, 0}) {
		t.Errorf("expected segment 0, got %v %v", v, ok)
	}
}

func TestWaypointTool(t *testing.T) {
	a, f, gtx := editorApp(t)
	a.setTool(waypointTool)
	ll := geo.LonLat{Lon: 12.2, Lat: 51.2}
	click(a, gtx, ll)
	if n := len(f.data.Waypoints); n != 3 || !math.IsNaN(f.data.Waypoints[2].Ele) || !f.dirty() {
		t.Fatalf("expected a new waypoint, got %d", n)
	}
	if !a.editor.hasSel || a.editor.sel != (vertex{-1, 2}) {
		t.Errorf("expected new waypoint selected, got %v", a.editor.sel)
	}
	a.deleteSelected()
	if len(f.data.Waypoints) != 2 || a.editor.hasSel {
		t.Errorf("expected waypoint deleted")
	}
}

func TestRouteTool(t *testing.T) {
	a, f, gtx := editorApp(t)
	a.setTool(routeTool)
	pts := []geo.LonLat{{Lon: 12.2, Lat: 51.2}, {Lon: 12.3, Lat: 51.2}, {Lon: 12.4, Lat: 51.2}}
	for _, p := range pts {
		click(a, gtx, p)
		// Routes with a single point are shown.
		a.updateTracks()
	}
	if len(f.data.Routes) != 2 || len(f.data.Routes[1].Points) != 3 {
		t.Fatalf("expected a new route with 3 points, got %v", f.data.Routes)
	}
	if it := a.selected; it == nil || it.kind != routeItem || it.nth != 1 {
		t.Errorf("expected the new route selected for the chart")
	}
	// Delete removes the last point, and selects the new end.
	a.deleteSelected()
	if len(f.data.Routes[1].Points) != 2 || a.editor.sel != (vertex{1, 1}) || !a.editor.drawing {
		t.Errorf("expected last point removed, got sel %v", a.editor.sel)
	}
	// A double click finishes the route without adding a point.
	ev := at(a, pointer.Press, pts[2])
	ev.Double = true
	a.editPress(gtx, ev)
	if a.editor.drawing || len(f.data.Routes[1].Points) != 2 {
		t.Errorf("expected drawing finished")
	}

	// Clicking the start of a route continues it at the start.
	start := f.data.Routes[0].Points[0].Pos
	click(a, gtx, start)
	if !a.editor.drawing || a.editor.drawRoute != 0 || !a.editor.drawStart || len(f.data.Routes[0].Points) != 2 {
		t.Fatalf("expected route 0 continued at the start")
	}
	click(a, gtx, geo.LonLat{Lon: 12, Lat: 50.9})
	if r := f.data.Routes[0].Points; len(r) != 3 || math.Abs(r[0].Pos.Lat-50.9) > 1e-9 || a.editor.sel != (vertex{0, 0}) {
		t.Errorf("expected a point at the start, got %v", r)
	}

	// Undoing the creation of the drawn route ends drawing.
	a.editor.hasSel = false
	a.setTool(routeTool)
	click(a, gtx, geo.LonLat{Lon: 13, Lat: 52})
	a.undo()
	a.validateEditor()
	if a.editor.drawing || a.editor.hasSel {
		t.Errorf("expected drawing ended after undo")
	}
}

func TestSelectTool(t *testing.T) {
	a, f, gtx := editorApp(t)
	route := f.data.Routes[0].Points
	from, to := route[1].Pos, geo.LonLat{Lon: 12.1, Lat: 51.1}

	// Dragging a point moves it.
	a.editPress(gtx, at(a, pointer.Press, from))
	if !a.editor.hasSel || a.editor.sel != (vertex{0, 1}) {
		t.Fatalf("expected point selected")
	}
	ev := at(a, pointer.Drag, to)
	a.editor.drag.pos, a.editor.drag.moved = ev.Pos, true
	a.editRelease()
	p := f.data.Routes[0].Points[1]
	if math.Abs(p.Pos.Lon-to.Lon) > 1e-9 || math.Abs(p.Pos.Lat-to.Lat) > 1e-9 || !math.IsNaN(p.Ele) {
		t.Errorf("expected point moved, got %v", p)
	}

	// Clicking a segment midpoint inserts a point there.
	r := f.data.Routes[0].Points
	mid := geo.ToLonLat(geo.Point{
		X: (geo.ToMercator(r[0].Pos).X + geo.ToMercator(r[1].Pos).X) / 2,
		Y: (geo.ToMercator(r[0].Pos).Y + geo.ToMercator(r[1].Pos).Y) / 2,
	})
	click(a, gtx, mid)
	if n := len(f.data.Routes[0].Points); n != 3 || a.editor.sel != (vertex{0, 1}) {
		t.Fatalf("expected inserted point, got %d points", n)
	}

	// Clicking elsewhere deselects, a click without moving doesn't change anything.
	click(a, gtx, geo.LonLat{Lon: 14, Lat: 53})
	if a.editor.hasSel {
		t.Errorf("expected no selection")
	}
	n := len(f.edit.undo)
	click(a, gtx, f.data.Waypoints[0].Pos)
	if len(f.edit.undo) != n || a.editor.sel != (vertex{-1, 0}) {
		t.Errorf("expected waypoint selected without change")
	}

	// Deleting all points of a route deletes the route.
	for range 3 {
		a.selectVertex(vertex{0, 0})
		a.deleteSelected()
	}
	if len(f.data.Routes) != 0 || a.selected != nil {
		t.Errorf("expected route deleted")
	}
}

// testLinkedGPX has a route built from waypoints, as written by devices that build routes from stored waypoints.
// The route starts and ends at the same waypoint.
const testLinkedGPX = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test" xmlns="http://www.topografix.com/GPX/1/1">
  <wpt lat="51.0" lon="12.0"><name>A</name></wpt>
  <wpt lat="51.1" lon="12.0"><name>B</name></wpt>
  <rte>
    <rtept lat="51.0" lon="12.0"><name>A</name></rtept>
    <rtept lat="51.1" lon="12.0"><name>B</name></rtept>
    <rtept lat="51.1" lon="12.1"><name>B</name></rtept>
    <rtept lat="51.0" lon="12.0"><name>A</name></rtept>
  </rte>
</gpx>`

func TestLinkedPoints(t *testing.T) {
	a, f := editApp(t, testLinkedGPX)
	a.style = DefaultStyle()
	a.mapView = mapview.New(geo.LonLat{Lon: 12, Lat: 51}, 9)
	a.mapView.View.Size = image.Pt(800, 600)
	a.toggleEdit(f)
	gtx := layout.Context{Metric: unit.Metric{PxPerDp: 1}}
	d := f.data

	if g := linkedGroup(d, vertex{0, 3}); len(g) != 3 || g[0] != (vertex{-1, 0}) {
		t.Errorf("unexpected group of A: %v", g)
	}
	// A route point with the name of a waypoint at another position is not linked.
	if g := linkedGroup(d, vertex{0, 2}); len(g) != 1 {
		t.Errorf("expected no links, got %v", g)
	}
	if l := linkedRoutePoints(d); len(l) != 3 || l[vertex{0, 2}] {
		t.Errorf("unexpected linked route points %v", l)
	}

	// Of linked points, the waypoint is hit, or the point of the route selected for the chart.
	p := screenPos(&a.mapView.View, d.Waypoints[1].Pos)
	if v, _ := hitVertex(d, &a.mapView.View, p, 5, -1); v != (vertex{-1, 1}) {
		t.Errorf("expected waypoint, got %v", v)
	}
	if v, _ := hitVertex(d, &a.mapView.View, p, 5, 0); v != (vertex{0, 1}) {
		t.Errorf("expected route point, got %v", v)
	}
	// The route tool continues routes at their ends only.
	if _, ok := hitRouteEnd(d, &a.mapView.View, p, 5, -1); ok {
		t.Errorf("expected no route end")
	}

	// Moving a waypoint moves the linked route points.
	to := geo.LonLat{Lon: 12.05, Lat: 50.95}
	a.editPress(gtx, at(a, pointer.Press, d.Waypoints[0].Pos))
	a.editor.drag.pos, a.editor.drag.moved = geo.ToMercator(to), true
	a.editRelease()
	for _, v := range []vertex{{-1, 0}, {0, 0}, {0, 3}} {
		if ll, _ := vertexPos(d, v); math.Abs(ll.Lon-to.Lon) > 1e-9 || math.Abs(ll.Lat-to.Lat) > 1e-9 {
			t.Errorf("%v not moved: %v", v, ll)
		}
	}
	if g := linkedGroup(d, vertex{-1, 0}); len(g) != 3 {
		t.Errorf("expected points still linked, got %v", g)
	}

	// Deleting a linked route point keeps the waypoint.
	a.selectVertex(vertex{0, 1})
	a.deleteSelected()
	if len(d.Waypoints) != 2 || len(d.Routes[0].Points) != 3 {
		t.Errorf("expected only the route point deleted")
	}
}

func TestRouteSnap(t *testing.T) {
	a, f, gtx := editorApp(t)
	d := f.data
	a.setTool(routeTool)
	hut, unnamed := d.Waypoints[0].Pos, d.Waypoints[1].Pos

	// Clicking waypoints adds linked copies of them. An unnamed waypoint gets a name.
	click(a, gtx, hut)
	click(a, gtx, unnamed)
	r := d.Routes[1].Points
	if len(r) != 2 || r[0].Name != "Hut" || d.Waypoints[1].Name != "WP001" || r[1].Name != "WP001" {
		t.Fatalf("unexpected route %v, waypoint %q", r, d.Waypoints[1].Name)
	}
	if len(linkedGroup(d, vertex{-1, 1})) != 2 {
		t.Errorf("expected the route point linked")
	}

	// With Shift, the point is placed without snapping.
	ev := at(a, pointer.Press, hut)
	ev.Modifiers = key.ModShift
	a.editPress(gtx, ev)
	a.editRelease()
	if p := d.Routes[1].Points[2]; p.Name != "" || len(linkedGroup(d, vertex{1, 2})) != 1 {
		t.Errorf("expected an unlinked point, got %v", p)
	}

	// Undo also takes back the generated name.
	a.undo()
	a.undo()
	if d.Waypoints[1].Name != "" {
		t.Errorf("expected generated name undone, got %q", d.Waypoints[1].Name)
	}
	if n := uniqueName(d); n != "WP001" {
		t.Errorf("unexpected name %q", n)
	}
}

// dragTo simulates dragging from a press at from to a position, and releasing.
func dragTo(a *App, gtx layout.Context, from, to geo.LonLat, mods key.Modifiers) {
	press := at(a, pointer.Press, from)
	press.Modifiers = mods
	a.editPress(gtx, press)
	ev := at(a, pointer.Drag, to)
	d := &a.editor.drag
	d.raw, d.screen, d.moved = ev.Pos, ev.Screen, true
	a.editor.shift = mods.Contain(key.ModShift)
	a.updateDragSnap(gtx)
	a.editRelease()
}

func TestDragSnap(t *testing.T) {
	a, f, gtx := editorApp(t)
	d := f.data
	hut, unnamed := d.Waypoints[0].Pos, d.Waypoints[1].Pos
	near := geo.LonLat{Lon: unnamed.Lon + 0.001, Lat: unnamed.Lat}
	a.selected = f.routeItem(0)

	// A route point dropped near a waypoint snaps to it, and is linked.
	dragTo(a, gtx, d.Routes[0].Points[1].Pos, near, 0)
	if p := d.Routes[0].Points[1]; p.Pos != unnamed || p.Name != "WP001" || len(linkedGroup(d, vertex{0, 1})) != 2 {
		t.Errorf("expected point snapped and linked, got %v", p)
	}

	// With Shift, it doesn't snap.
	dragTo(a, gtx, d.Routes[0].Points[0].Pos, hut, key.ModShift)
	if p := d.Routes[0].Points[0]; p.Name != "" || len(linkedGroup(d, vertex{0, 0})) != 1 {
		t.Errorf("expected point not snapped, got %v", p)
	}
	a.undo()

	// A point inserted by dragging a midpoint snaps, too.
	r := d.Routes[0].Points
	p0, p1 := geo.ToMercator(r[0].Pos), geo.ToMercator(r[1].Pos)
	mid := geo.ToLonLat(geo.Point{X: (p0.X + p1.X) / 2, Y: (p0.Y + p1.Y) / 2})
	dragTo(a, gtx, mid, hut, 0)
	if p := d.Routes[0].Points[1]; len(d.Routes[0].Points) != 3 || p.Name != "Hut" || p.Pos != hut {
		t.Errorf("expected inserted point snapped to Hut, got %v", d.Routes[0].Points)
	}
}

func TestUnlinkDrag(t *testing.T) {
	a, f := editApp(t, testLinkedGPX)
	a.style = DefaultStyle()
	a.mapView = mapview.New(geo.LonLat{Lon: 12, Lat: 51}, 9)
	a.mapView.View.Size = image.Pt(800, 600)
	a.toggleEdit(f)
	gtx := layout.Context{Metric: unit.Metric{PxPerDp: 1}}
	d := f.data
	a.selected = nil

	// Shift-dragging the shared handle of B moves only the route point, which is then unlinked.
	b := d.Waypoints[1].Pos
	to := geo.LonLat{Lon: 12.05, Lat: 51.15}
	dragTo(a, gtx, b, to, key.ModShift)
	if d.Waypoints[1].Pos != b || len(linkedGroup(d, vertex{-1, 1})) != 1 {
		t.Errorf("expected waypoint B kept and unlinked")
	}
	if p := d.Routes[0].Points[1]; math.Abs(p.Pos.Lon-to.Lon) > 1e-9 || p.Name != "" {
		t.Errorf("expected route point moved without name, got %v", p)
	}

	// Shift pressed during the drag unlinks while it is held, and the point doesn't snap.
	a.undo()
	a.editPress(gtx, at(a, pointer.Press, b))
	ev := at(a, pointer.Drag, d.Waypoints[0].Pos)
	dr := &a.editor.drag
	dr.raw, dr.screen, dr.moved = ev.Pos, ev.Screen, true
	a.editor.shift = true
	a.updateDragSnap(gtx)
	if dr.snapped || len(dr.group) != 1 || dr.v != (vertex{0, 1}) {
		t.Errorf("expected unlinked drag without snapping, got %+v", *dr)
	}
	// Releasing Shift moves the linked points together again, still without snapping.
	a.editor.shift = false
	a.updateDragSnap(gtx)
	if dr.snapped || len(dr.group) != 2 || dr.unlinked {
		t.Errorf("expected linked drag again, got %+v", *dr)
	}
	a.editRelease()
	if g := linkedGroup(d, vertex{-1, 1}); len(g) != 2 || d.Waypoints[1].Name != "B" {
		t.Errorf("expected points still linked, got %v", g)
	}

	// Without Shift, the group moves together.
	a.undo()
	dragTo(a, gtx, b, to, 0)
	if g := linkedGroup(d, vertex{-1, 1}); len(g) != 2 || math.Abs(d.Waypoints[1].Pos.Lon-to.Lon) > 1e-9 {
		t.Errorf("expected group moved together, got %v", g)
	}
}

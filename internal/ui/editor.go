package ui

import (
	"fmt"
	"math"
	"slices"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/track"
)

// editTool is a tool for editing on the map.
type editTool int

const (
	// selectTool selects, moves, inserts and deletes points.
	selectTool editTool = iota
	// waypointTool adds waypoints.
	waypointTool
	// routeTool draws routes.
	routeTool
	numTools
)

var tools = [numTools]struct {
	name, key, help string
}{
	{"Select", "S", `Select tool (S)

Click a point to select it, drag it to move it.
Del or Backspace deletes the selected point.
Drag or click the small circles between route points to insert a point.
Route points dropped on a waypoint snap to it and are linked to it. Hold Shift to not snap.
Hold Shift while dragging a linked route point to move it alone.
Released with Shift held, it is unlinked from its waypoint, and loses the waypoint's name.
Esc or a right click clears the selection.

Route points with the same name and position as a waypoint are linked to it and move with it.`},
	{"Waypoint", "W", `Waypoint tool (W)

Click on the map to add a waypoint.`},
	{"Route", "R", `Route tool (R)

Click on the map to add points to a new route.
Clicking a waypoint adds it to the route, linked to it. Unnamed waypoints get a name.
Hold Shift to place a point without snapping to waypoints.
Esc, Enter, a right click or a double click finishes the route.
Del or Backspace removes the last point.

Click the first or last point of a route, or select the route in the side panel, to continue it.`},
}

// dragSlop is how far the pointer must move, in pixels, before a press becomes a drag.
const dragSlop = 3

// vertex is a waypoint of the edited file if route is negative, or otherwise a point of a route.
type vertex struct {
	route, point int
}

// editor is the state of editing on the map.
type editor struct {
	tool     editTool
	toolBtns [numTools]iconButton
	// undoBtn and redoBtn undo and redo changes.
	undoBtn, redoBtn iconButton

	// sel is the selected vertex, if hasSel.
	sel    vertex
	hasSel bool
	// drawing is set while the route tool adds points to route drawRoute, at its start if drawStart.
	drawing   bool
	drawRoute int
	drawStart bool
	// lastSelected is the item selected for the chart at the last frame, for noticing a new selection.
	lastSelected *fileItem
	// shift is set while the Shift key is held, which disables snapping to waypoints.
	shift bool

	drag editDrag
}

// editDrag is a press on a vertex or a segment midpoint in the select tool, which may become a drag.
type editDrag struct {
	active bool
	// insert is set for inserting a point between v.point and v.point+1 of route v.route,
	// and otherwise v is moved, together with the points linked to it in group.
	insert bool
	v      vertex
	group  []vertex
	// pos is the position to move or insert at: raw, the pointer position, or the snapped waypoint, if snapped.
	pos, raw geo.Point
	snap     int
	snapped  bool
	// start is the screen position of the press, screen the current one.
	start, screen f32.Point
	moved         bool
	// unlinked is set while a linked route point is moved alone, see [App.unlinkDrag].
	// pressed is the vertex that was pressed, links the vertices linked to it.
	unlinked bool
	pressed  vertex
	links    []vertex
}

// canSnap reports whether the dragged point can snap to a waypoint: a new point, or an unlinked route point.
// Linked points don't snap, so a point being unlinked isn't linked to another waypoint right away.
func (d *editDrag) canSnap() bool {
	return d.insert || d.v.route >= 0 && len(d.links) == 1
}

// reset clears all state except the tool.
func (e *editor) reset() {
	e.sel, e.hasSel = vertex{}, false
	e.drawing = false
	e.drag = editDrag{}
}

// vertexPos returns the position of a vertex, and false if it doesn't exist.
func vertexPos(d *track.File, v vertex) (geo.LonLat, bool) {
	if v.route < 0 {
		if v.point < 0 || v.point >= len(d.Waypoints) {
			return geo.LonLat{}, false
		}
		return d.Waypoints[v.point].Pos, true
	}
	if v.route >= len(d.Routes) || v.point < 0 || v.point >= len(d.Routes[v.route].Points) {
		return geo.LonLat{}, false
	}
	return d.Routes[v.route].Points[v.point].Pos, true
}

// screenPos returns the screen position of a position.
func screenPos(view *mapview.View, p geo.LonLat) f32.Point {
	x, y := view.ToScreen(geo.ToMercator(p))
	return f32.Pt(float32(x), float32(y))
}

// dist2 returns the squared distance between two screen positions.
func dist2(a, b f32.Point) float32 {
	d := a.Sub(b)
	return d.X*d.X + d.Y*d.Y
}

// hitMidpoint returns the route segment whose midpoint on the screen is nearest to p, within radius pixels,
// as the vertex at the start of the segment.
func hitMidpoint(d *track.File, view *mapview.View, p f32.Point, radius float32) (vertex, bool) {
	best, found := radius*radius, false
	var hit vertex
	for r := range d.Routes {
		pts := d.Routes[r].Points
		for i := 1; i < len(pts); i++ {
			m := screenPos(view, pts[i-1].Pos).Add(screenPos(view, pts[i].Pos)).Mul(0.5)
			if d2 := dist2(m, p); d2 <= best {
				best, hit, found = d2, vertex{route: r, point: i - 1}, true
			}
		}
	}
	return hit, found
}

// routeItem returns the item of route r of the file, or nil.
func (f *openFile) routeItem(r int) *fileItem {
	for _, it := range f.items {
		if it.kind == routeItem && it.nth == r {
			return it
		}
	}
	return nil
}

// newWaypoint returns a waypoint at a map position, without elevation.
func newWaypoint(p geo.Point) track.Waypoint {
	return track.Waypoint{Pos: geo.ToLonLat(p), Ele: math.NaN()}
}

// handleRadius returns the radius for hitting vertices, in pixels.
func (a *App) handleRadius(gtx layout.Context) float32 {
	return float32(gtx.Dp(a.style.HoverRadius))
}

// updateEditor handles the tools, keys and map input for editing.
func (a *App) updateEditor(gtx layout.Context) {
	e := &a.editor
	events := a.mapView.Primary()
	secondary := a.mapView.SecondaryClicked()
	for i := range e.toolBtns {
		if e.toolBtns[i].Clicked(gtx) {
			a.setTool(editTool(i))
		}
	}
	if e.undoBtn.Clicked(gtx) {
		a.undo()
	}
	if e.redoBtn.Clicked(gtx) {
		a.redo()
	}
	f := a.editing
	if f == nil {
		e.reset()
		e.lastSelected = a.selected
		a.mapView.Cursor = pointer.CursorDefault
		return
	}
	a.validateEditor()

	// Selecting a route of the file in the panel continues it with the route tool.
	if a.selected != e.lastSelected && e.tool == routeTool && !e.drawing {
		if it := a.selected; it != nil && it.kind == routeItem && slices.Contains(f.items, it) {
			e.drawing, e.drawRoute, e.drawStart = true, it.nth, false
		}
	}

	a.updateEditKeys(gtx)
	// Clicking the map ends typing in a property field, which applies it.
	if len(events) > 0 && a.propsFocused(gtx) {
		gtx.Execute(key.FocusCmd{})
	}
	// A right click without panning clears the selection.
	if secondary {
		e.drag = editDrag{}
		e.drawing, e.hasSel = false, false
	}
	for _, ev := range events {
		switch ev.Kind {
		case pointer.Press:
			a.editPress(gtx, ev)
		case pointer.Drag:
			if d := &e.drag; d.active {
				d.pos, d.raw, d.screen = ev.Pos, ev.Pos, ev.Screen
				d.moved = d.moved || dist2(ev.Screen, d.start) > dragSlop*dragSlop
				e.shift = ev.Modifiers.Contain(key.ModShift)
			}
		case pointer.Release:
			e.shift = ev.Modifiers.Contain(key.ModShift)
			a.unlinkDrag()
			a.editRelease()
		case pointer.Cancel:
			e.drag = editDrag{}
		}
	}
	a.updateDragSnap(gtx)
	e.lastSelected = a.selected
	a.mapView.Cursor = a.editCursor(gtx)
}

// validateEditor drops the selection and the drawn route if they no longer exist, e.g. after undo.
func (a *App) validateEditor() {
	e, d := &a.editor, a.editing.data
	if _, ok := vertexPos(d, e.sel); e.hasSel && !ok {
		e.hasSel = false
	}
	if e.drawing && e.drawRoute >= len(d.Routes) {
		e.drawing = false
	}
	if e.drag.active {
		v := e.drag.v
		if e.drag.insert {
			v.point++
		}
		if _, ok := vertexPos(d, v); !ok {
			e.drag = editDrag{}
		}
		for _, g := range e.drag.group {
			if _, ok := vertexPos(d, g); !ok {
				e.drag = editDrag{}
			}
		}
	}
}

// updateEditKeys handles the keys for editing.
// Key events go to the first handler asking for them, so while typing in a property field,
// only Esc is taken, for reverting the field.
func (a *App) updateEditKeys(gtx layout.Context) {
	filters := []event.Filter{
		key.Filter{Name: key.NameEscape},
		key.Filter{Name: key.NameShift, Optional: key.ModShift},
	}
	typing := a.propsFocused(gtx)
	if !typing {
		filters = append(filters,
			key.Filter{Name: key.NameReturn},
			key.Filter{Name: key.NameDeleteForward},
			key.Filter{Name: key.NameDeleteBackward},
		)
		for _, t := range tools {
			filters = append(filters, key.Filter{Name: key.Name(t.key)})
		}
	}
	e := &a.editor
	for {
		ev, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		ke, ok := ev.(key.Event)
		if ok && ke.Name == key.NameShift {
			e.shift = ke.State == key.Press
			continue
		}
		if !ok || ke.State != key.Press {
			continue
		}
		if typing {
			a.revertProps(gtx)
			continue
		}
		switch ke.Name {
		case key.NameEscape:
			switch {
			case e.drag.active:
				e.drag = editDrag{}
			case e.drawing:
				e.drawing = false
			default:
				e.hasSel = false
			}
		case key.NameReturn:
			e.drawing = false
		case key.NameDeleteForward, key.NameDeleteBackward:
			a.deleteSelected()
		default:
			for i, t := range tools {
				if ke.Name == key.Name(t.key) {
					a.setTool(editTool(i))
				}
			}
		}
	}
}

// setTool selects a tool. The route tool continues the route of a selected end point.
func (a *App) setTool(t editTool) {
	e := &a.editor
	e.tool = t
	e.drawing = false
	e.drag = editDrag{}
	if t != routeTool || a.editing == nil || !e.hasSel || e.sel.route < 0 {
		return
	}
	n := len(a.editing.data.Routes[e.sel.route].Points)
	if e.sel.point == n-1 || e.sel.point == 0 {
		e.drawing, e.drawRoute, e.drawStart = true, e.sel.route, e.sel.point == 0 && n > 1
	}
}

// editPress handles a press of the primary button on the map.
func (a *App) editPress(gtx layout.Context, ev mapview.PointerEvent) {
	e, f := &a.editor, a.editing
	view := &a.mapView.View
	radius := a.handleRadius(gtx)
	switch e.tool {
	case selectTool:
		if v, ok := hitVertex(f.data, view, ev.Screen, radius, a.preferredRoute()); ok {
			a.selectVertex(v)
			e.shift = ev.Modifiers.Contain(key.ModShift)
			links := linkedGroup(f.data, v)
			e.drag = editDrag{active: true, v: v, group: links, pressed: v, links: links, pos: ev.Pos, raw: ev.Pos, start: ev.Screen, screen: ev.Screen}
			a.unlinkDrag()
		} else if v, ok := hitMidpoint(f.data, view, ev.Screen, radius); ok {
			e.drag = editDrag{active: true, insert: true, v: v, pos: ev.Pos, raw: ev.Pos, start: ev.Screen, screen: ev.Screen}
		} else {
			e.hasSel = false
		}

	case waypointTool:
		f.change(func(d *track.File) { d.Waypoints = append(d.Waypoints, newWaypoint(ev.Pos)) })
		a.changed()
		a.selectVertex(vertex{route: -1, point: len(f.data.Waypoints) - 1})

	case routeTool:
		if ev.Double && e.drawing {
			// The first click of the double click added the last point.
			e.drawing = false
			return
		}
		e.shift = ev.Modifiers.Contain(key.ModShift)
		snap, _ := a.snapTarget(gtx, ev.Screen)
		if !e.drawing {
			// Clicking an end point of a route continues it.
			if v, ok := hitRouteEnd(f.data, view, ev.Screen, radius, a.preferredRoute()); ok {
				n := len(f.data.Routes[v.route].Points)
				a.selectVertex(v)
				e.drawing, e.drawRoute, e.drawStart = true, v.route, v.point == 0 && n > 1
				return
			}
			f.change(func(d *track.File) {
				d.Routes = append(d.Routes, track.Route{Points: []track.Waypoint{routePoint(d, ev.Pos, snap)}})
			})
			a.changed()
			e.drawing, e.drawRoute, e.drawStart = true, len(f.data.Routes)-1, false
			a.selectVertex(vertex{route: e.drawRoute, point: 0})
			return
		}
		r, start := e.drawRoute, e.drawStart
		f.change(func(d *track.File) {
			pts := &d.Routes[r].Points
			if start {
				*pts = slices.Insert(*pts, 0, routePoint(d, ev.Pos, snap))
			} else {
				*pts = append(*pts, routePoint(d, ev.Pos, snap))
			}
		})
		a.changed()
		v := vertex{route: r}
		if !start {
			v.point = len(f.data.Routes[r].Points) - 1
		}
		a.selectVertex(v)
	}
}

// unlinkDrag makes a drag of linked points move only one of the route points while Shift is held,
// and all of them again when it is released. Released with Shift, the point is unlinked from its waypoint.
func (a *App) unlinkDrag() {
	e := &a.editor
	d := &e.drag
	if !d.active || d.insert || len(d.links) < 2 || e.shift == d.unlinked {
		return
	}
	d.unlinked = e.shift
	if d.unlinked {
		d.v = unlinkTarget(d.links, d.pressed, a.preferredRoute())
		d.group = []vertex{d.v}
	} else {
		d.v, d.group = d.pressed, d.links
	}
	a.selectVertex(d.v)
}

// unlinkTarget returns the route point of a linked group to move alone:
// v if it is a route point, otherwise a point of route prefer, or the first route point of the group.
func unlinkTarget(group []vertex, v vertex, prefer int) vertex {
	if v.route >= 0 {
		return v
	}
	for _, g := range group {
		if g.route >= 0 && g.route == prefer {
			return g
		}
	}
	return group[1]
}

// updateDragSnap snaps a dragged route point to a waypoint near the pointer, unless Shift is held.
func (a *App) updateDragSnap(gtx layout.Context) {
	d := &a.editor.drag
	a.unlinkDrag()
	if !d.active || !d.moved {
		return
	}
	d.pos, d.snapped = d.raw, false
	if !d.canSnap() {
		return
	}
	if wi, ok := a.snapTarget(gtx, d.screen); ok {
		d.pos, d.snap, d.snapped = geo.ToMercator(a.editing.data.Waypoints[wi].Pos), wi, true
	}
}

// snapTarget returns the waypoint to snap to at screen position p, or -1.
// Shift disables snapping.
func (a *App) snapTarget(gtx layout.Context, p f32.Point) (int, bool) {
	e, f := &a.editor, a.editing
	if f == nil || e.shift {
		return -1, false
	}
	v, ok := nearestVertex(f.data, &a.mapView.View, p, a.handleRadius(gtx), func(v vertex) int {
		if v.route < 0 {
			return 0
		}
		return -1
	})
	if !ok {
		return -1, false
	}
	return v.point, true
}

// hoverSnap returns the waypoint the route tool snaps to at the pointer, or -1.
func (a *App) hoverSnap(gtx layout.Context) (int, bool) {
	if !a.mapView.HoverValid || a.editor.tool != routeTool {
		return -1, false
	}
	x, y := a.mapView.View.ToScreen(a.mapView.Hover)
	return a.snapTarget(gtx, f32.Pt(float32(x), float32(y)))
}

// routePoint returns a new route point at map position p, or, if snap is a waypoint,
// a copy of that waypoint, which is then linked to it, see [linked].
// An unnamed waypoint gets a generated name first, as linking needs a name.
func routePoint(d *track.File, p geo.Point, snap int) track.Waypoint {
	if snap < 0 || snap >= len(d.Waypoints) {
		return newWaypoint(p)
	}
	w := &d.Waypoints[snap]
	if w.Name == "" {
		w.Name = uniqueName(d)
	}
	return *w
}

// uniqueName returns a waypoint name not used by any point of the file, like "WP007".
func uniqueName(d *track.File) string {
	used := map[string]bool{}
	for _, w := range d.Waypoints {
		used[w.Name] = true
	}
	for _, r := range d.Routes {
		for _, w := range r.Points {
			used[w.Name] = true
		}
	}
	for i := 1; ; i++ {
		if name := fmt.Sprintf("WP%03d", i); !used[name] {
			return name
		}
	}
}

// editRelease handles a release of the primary button, ending a move or insert in the select tool.
// A click on a segment midpoint inserts a point there.
func (a *App) editRelease() {
	e, f := &a.editor, a.editing
	d := e.drag
	e.drag = editDrag{}
	if !d.active {
		return
	}
	v := d.v
	snap := -1
	if d.snapped {
		snap = d.snap
	}
	switch {
	case d.insert:
		pts := f.data.Routes[v.route].Points
		pos := d.pos
		if !d.moved {
			p0, p1 := geo.ToMercator(pts[v.point].Pos), geo.ToMercator(pts[v.point+1].Pos)
			pos = geo.Point{X: (p0.X + p1.X) / 2, Y: (p0.Y + p1.Y) / 2}
		}
		f.change(func(fd *track.File) {
			p := &fd.Routes[v.route].Points
			*p = slices.Insert(*p, v.point+1, routePoint(fd, pos, snap))
		})
		a.changed()
		a.selectVertex(vertex{route: v.route, point: v.point + 1})
	case d.moved && d.snapped:
		// The route point becomes a copy of the waypoint, linked to it.
		f.change(func(fd *track.File) {
			fd.Routes[v.route].Points[v.point] = routePoint(fd, d.pos, snap)
		})
		a.changed()
	case d.moved:
		f.change(func(fd *track.File) {
			for _, g := range d.group {
				w := &fd.Waypoints
				if g.route >= 0 {
					w = &fd.Routes[g.route].Points
				}
				// The elevation of the old position doesn't apply to the new one.
				(*w)[g.point].Pos = geo.ToLonLat(d.pos)
				(*w)[g.point].Ele = math.NaN()
			}
			// An unlinked point doesn't keep the waypoint's name and other attributes.
			if d.unlinked {
				p := &fd.Routes[v.route].Points[v.point]
				p.Name, p.Desc, p.Symbol, p.Type = "", "", "", ""
			}
		})
		a.changed()
	}
}

// preferredRoute returns the index of the route selected for the chart, if it is in the edited file, or -1.
func (a *App) preferredRoute() int {
	if it := a.selected; it != nil && it.kind == routeItem && a.editing != nil && a.editing.routeItem(it.nth) == it {
		return it.nth
	}
	return -1
}

// selectVertex selects a vertex, and its route for the chart.
func (a *App) selectVertex(v vertex) {
	e := &a.editor
	e.sel, e.hasSel = v, true
	if v.route < 0 {
		return
	}
	if it := a.editing.routeItem(v.route); it != nil && it != a.selected {
		a.selected = it
		a.tracksChanged = true
	}
	// Selecting a route this way doesn't continue it.
	e.lastSelected = a.selected
}

// deleteSelected deletes the selected vertex. A route without points is deleted.
// While drawing, the new end of the route is selected, so that points can be deleted one after another.
func (a *App) deleteSelected() {
	e, f := &a.editor, a.editing
	if !e.hasSel || e.drag.active {
		return
	}
	v := e.sel
	e.hasSel = false
	removeRoute := false
	f.change(func(d *track.File) {
		if v.route < 0 {
			d.Waypoints = slices.Delete(d.Waypoints, v.point, v.point+1)
			return
		}
		p := &d.Routes[v.route].Points
		*p = slices.Delete(*p, v.point, v.point+1)
		if len(*p) == 0 {
			d.Routes = slices.Delete(d.Routes, v.route, v.route+1)
			removeRoute = true
		}
	})
	a.changed()
	if v.route < 0 || !e.drawing || e.drawRoute != v.route {
		return
	}
	if removeRoute {
		e.drawing = false
		return
	}
	n := len(f.data.Routes[v.route].Points)
	if e.drawStart {
		a.selectVertex(vertex{route: v.route})
	} else {
		a.selectVertex(vertex{route: v.route, point: n - 1})
	}
}

// editCursor returns the cursor over the map while editing.
func (a *App) editCursor(gtx layout.Context) pointer.Cursor {
	e := &a.editor
	if e.tool != selectTool {
		return pointer.CursorCrosshair
	}
	if e.drag.active {
		return pointer.CursorGrabbing
	}
	if !a.mapView.HoverValid {
		return pointer.CursorDefault
	}
	sx, sy := a.mapView.View.ToScreen(a.mapView.Hover)
	p := f32.Pt(float32(sx), float32(sy))
	if _, ok := hitVertex(a.editing.data, &a.mapView.View, p, a.handleRadius(gtx), -1); ok {
		return pointer.CursorGrab
	}
	if _, ok := hitMidpoint(a.editing.data, &a.mapView.View, p, a.handleRadius(gtx)); ok {
		return pointer.CursorPointer
	}
	return pointer.CursorDefault
}

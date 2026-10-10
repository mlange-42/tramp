package ui

import (
	"image"
	"image/color"
	"slices"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/mlange-42/tramp/internal/geo"
)

// layoutEditMap draws the points of the edited file as handles on the map, the midpoints of route segments
// for inserting points with the select tool, and the lines to a point being moved, inserted or added.
// Route points linked to a waypoint share its handle, see [linked].
func (a *App) layoutEditMap(gtx layout.Context) {
	f := a.editing
	if f == nil {
		return
	}
	e, st := &a.editor, a.style
	view := &a.mapView.View
	d := f.data
	r := float32(gtx.Dp(st.EditHandleSize)) / 2
	visible := func(p f32.Point) bool {
		return p.X >= -r && p.Y >= -r && p.X <= float32(gtx.Constraints.Max.X)+r && p.Y <= float32(gtx.Constraints.Max.Y)+r
	}
	// pos returns the screen position of a vertex, which may be moved.
	moving := e.drag.active && !e.drag.insert && e.drag.moved
	pos := func(v vertex, ll geo.LonLat) f32.Point {
		if moving && slices.Contains(e.drag.group, v) {
			x, y := view.ToScreen(e.drag.pos)
			return f32.Pt(float32(x), float32(y))
		}
		return screenPos(view, ll)
	}

	a.drawRubberBand(gtx)

	if e.tool == selectTool {
		mr := float32(gtx.Dp(st.EditMidSize)) / 2
		for ri := range d.Routes {
			pts := d.Routes[ri].Points
			for i := 1; i < len(pts); i++ {
				p0, p1 := pos(vertex{ri, i - 1}, pts[i-1].Pos), pos(vertex{ri, i}, pts[i].Pos)
				m := p0.Add(p1).Mul(0.5)
				// Midpoints of short segments would cover the points.
				if !visible(m) || dist2(p0, p1) < 16*r*r {
					continue
				}
				fillCircle(gtx, m, mr+1, st.EditHandleRing)
				fillCircle(gtx, m, mr, st.EditMidFill)
			}
		}
	}

	handle := func(p f32.Point, fill color.NRGBA, rr float32) {
		if visible(p) {
			fillCircle(gtx, p, rr+float32(gtx.Dp(1.5)), st.EditHandleRing)
			fillCircle(gtx, p, rr, fill)
		}
	}
	linkedPts := linkedRoutePoints(d)
	for ri := range d.Routes {
		for i, w := range d.Routes[ri].Points {
			if v := (vertex{ri, i}); !linkedPts[v] {
				handle(pos(v, w.Pos), st.EditHandleFill, r)
			}
		}
	}
	for i, w := range d.Waypoints {
		v := vertex{-1, i}
		handle(pos(v, w.Pos), st.EditHandleFill, r)
	}
	// The selected point is drawn last, so that it is not covered by a linked point.
	if ll, ok := vertexPos(d, e.sel); e.hasSel && ok {
		handle(pos(e.sel, ll), st.EditActive, r*1.3)
	}
}

// drawRubberBand draws the lines to a point being moved or inserted, or from the end of the drawn route to the pointer.
func (a *App) drawRubberBand(gtx layout.Context) {
	e, d := &a.editor, a.editing.data
	view := &a.mapView.View
	toScreen := func(p geo.Point) f32.Point {
		x, y := view.ToScreen(p)
		return f32.Pt(float32(x), float32(y))
	}
	var lines [][]f32.Point
	switch {
	case e.drag.active && e.drag.moved && e.drag.insert:
		v := e.drag.v
		route := d.Routes[v.route].Points
		lines = append(lines, []f32.Point{screenPos(view, route[v.point].Pos), toScreen(e.drag.pos), screenPos(view, route[v.point+1].Pos)})
	case e.drag.active && e.drag.moved:
		at := toScreen(e.drag.pos)
		for _, v := range e.drag.group {
			if v.route < 0 {
				continue
			}
			route := d.Routes[v.route].Points
			if v.point > 0 {
				lines = append(lines, []f32.Point{screenPos(view, route[v.point-1].Pos), at})
			}
			if v.point < len(route)-1 {
				lines = append(lines, []f32.Point{at, screenPos(view, route[v.point+1].Pos)})
			}
		}
	case e.drawing && !e.drag.active && a.mapView.HoverValid:
		route := d.Routes[e.drawRoute].Points
		end := route[len(route)-1]
		if e.drawStart {
			end = route[0]
		}
		lines = append(lines, []f32.Point{screenPos(view, end.Pos), toScreen(a.mapView.Hover)})
	}
	if len(lines) == 0 {
		return
	}
	var path clip.Path
	path.Begin(gtx.Ops)
	for _, l := range lines {
		path.MoveTo(l[0])
		for _, p := range l[1:] {
			path.LineTo(p)
		}
	}
	paint.FillShape(gtx.Ops, a.style.EditBand, clip.Stroke{Path: path.End(), Width: float32(gtx.Dp(2))}.Op())
}

// fillCircle draws a filled circle.
func fillCircle(gtx layout.Context, c f32.Point, r float32, col color.NRGBA) {
	rect := image.Rect(int(c.X-r+0.5), int(c.Y-r+0.5), int(c.X+r+0.5), int(c.Y+r+0.5))
	paint.FillShape(gtx.Ops, col, clip.Ellipse(rect).Op(gtx.Ops))
}

// layoutTools draws the toggle between the edit tools.
func (a *App) layoutTools(gtx layout.Context) layout.Dimensions {
	st := a.style
	e := &a.editor
	children := make([]layout.FlexChild, 0, 2*numTools)
	for i := range tools {
		if i > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Width: 1}.Layout))
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			avail := gtx.Constraints.Max.X
			dims := e.toolBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				macro := op.Record(gtx.Ops)
				dims := st.ButtonInset.Layout(gtx, st.Label(tools[i].name).Layout)
				call := macro.Stop()
				bg := st.StatusBg
				if editTool(i) == e.tool {
					bg = st.SelectedBg
				}
				rr := gtx.Dp(st.CornerRadius)
				paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: dims.Size}, rr).Op(gtx.Ops))
				call.Add(gtx.Ops)
				if editTool(i) != e.tool {
					pointer.CursorPointer.Add(gtx.Ops)
				}
				return dims
			})
			e.toolTips[i].Layout(gtx, st, e.toolBtns[i].Hovered(), dims.Size, avail, tools[i].help)
			return dims
		}))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

// layoutUndoRedo draws the undo and redo buttons, disabled if there is nothing to undo or redo.
func (a *App) layoutUndoRedo(gtx layout.Context) layout.Dimensions {
	st, e, f := a.style, &a.editor, a.editing
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return e.undoBtn.Layout(gtx, st, "Undo", "Undo the last change (Ctrl+Z)", len(f.edit.undo) > 0)
		}),
		layout.Rigid(layout.Spacer{Width: st.Spacing}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return e.redoBtn.Layout(gtx, st, "Redo", "Redo the last undone change (Ctrl+Y or Ctrl+Shift+Z)", len(f.edit.redo) > 0)
		}),
	)
}

// editHint returns a hint on using the current tool, for the status bar.
func (a *App) editHint() string {
	e := &a.editor
	switch e.tool {
	case waypointTool:
		return "Click to add a waypoint"
	case routeTool:
		if e.drawing {
			return "Click to add points · Esc, Enter or double-click to finish · Del removes the last point"
		}
		return "Click to start a route, or on a route end to continue it"
	}
	if e.hasSel {
		return "Drag to move · Del to delete · Esc to deselect"
	}
	return "Click a point to select it · drag points to move · drag the small circles to insert"
}

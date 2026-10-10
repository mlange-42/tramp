package ui

import (
	"fmt"
	"image"
	"slices"
	"strings"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/mlange-42/tramp/internal/track"
)

// propTarget is the object a property field edits: route route itself if point is negative,
// otherwise a point of that route, or waypoint point if route is negative.
type propTarget struct {
	route, point int
}

// isRoute reports whether the target is a route itself, not one of its points.
func (t propTarget) isRoute() bool { return t.route >= 0 && t.point < 0 }

// vertex returns the vertex of a point target.
func (t propTarget) vertex() vertex { return vertex{route: t.route, point: t.point} }

// Slots of the property fields.
const (
	routeNameField = iota
	routeDescField
	pointNameField
	pointDescField
	numPropFields
)

// propField is a text field for the name or description of a route, a route point or a waypoint.
type propField struct {
	ed     widget.Editor
	target propTarget
	desc   bool
	bound  bool
	// loaded is the value at the last load or commit, for noticing unapplied changes.
	loaded string
	// focused is the focus at the last frame, for noticing when the field loses it.
	focused bool
}

// props is the properties box in the side panel for the selection of the edited file.
type props struct {
	fields      [numPropFields]propField
	deleteRoute tooltipButton
}

// propValue returns the name or description of a target, and false if it doesn't exist.
func propValue(d *track.File, t propTarget, desc bool) (string, bool) {
	var name, dsc string
	switch {
	case t.isRoute():
		if t.route >= len(d.Routes) {
			return "", false
		}
		name, dsc = d.Routes[t.route].Name, d.Routes[t.route].Desc
	default:
		if _, ok := vertexPos(d, t.vertex()); !ok {
			return "", false
		}
		w := waypointRef(d, t.vertex())
		name, dsc = w.Name, w.Desc
	}
	if desc {
		return dsc, true
	}
	return name, true
}

// waypointRef returns the waypoint or route point of a vertex, which must exist.
func waypointRef(d *track.File, v vertex) *track.Waypoint {
	if v.route < 0 {
		return &d.Waypoints[v.point]
	}
	return &d.Routes[v.route].Points[v.point]
}

// setProp sets the name or description of a target. For a point, it is set for all points linked to it,
// so that they stay linked, see [linked].
func setProp(d *track.File, t propTarget, desc bool, v string) {
	if t.isRoute() {
		if desc {
			d.Routes[t.route].Desc = v
		} else {
			d.Routes[t.route].Name = v
		}
		return
	}
	for _, g := range linkedGroup(d, t.vertex()) {
		if w := waypointRef(d, g); desc {
			w.Desc = v
		} else {
			w.Name = v
		}
	}
}

// propTargets returns the targets of the route and the point fields, if they are shown:
// the route and point of a selected route point, a selected waypoint, or the route selected in the panel.
func (a *App) propTargets() (route, point propTarget, hasRoute, hasPoint bool) {
	e := &a.editor
	switch {
	case e.hasSel && e.sel.route >= 0:
		return propTarget{e.sel.route, -1}, propTarget(e.sel), true, true
	case e.hasSel:
		return propTarget{}, propTarget(e.sel), false, true
	}
	if r := a.preferredRoute(); r >= 0 {
		return propTarget{r, -1}, propTarget{}, true, false
	}
	return propTarget{}, propTarget{}, false, false
}

// updateProps processes input on the property fields, and loads them with the values of the selection.
// Changes are applied when the field loses the focus or Enter is pressed, or before the selection changes.
func (a *App) updateProps(gtx layout.Context) {
	p := &a.props
	if p.deleteRoute.btn.Clicked(gtx) {
		a.deleteRoute()
	}
	f := a.editing
	route, point, hasRoute, hasPoint := propTarget{}, propTarget{}, false, false
	if f != nil {
		route, point, hasRoute, hasPoint = a.propTargets()
	}
	for i := range p.fields {
		fl := &p.fields[i]
		for {
			ev, ok := fl.ed.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.SubmitEvent); ok {
				a.commitField(fl)
				gtx.Execute(key.FocusCmd{})
			}
		}
		focused := gtx.Focused(&fl.ed)
		if fl.focused && !focused {
			a.commitField(fl)
		}
		fl.focused = focused

		want, show := point, hasPoint
		if i == routeNameField || i == routeDescField {
			want, show = route, hasRoute
		}
		desc := i == routeDescField || i == pointDescField
		if !show {
			if fl.bound && fl.ed.Text() != fl.loaded {
				a.commitField(fl)
			}
			fl.bound = false
			continue
		}
		if !fl.bound || fl.target != want {
			if fl.bound && fl.ed.Text() != fl.loaded {
				a.commitField(fl)
			}
			fl.target, fl.desc, fl.bound = want, desc, true
			fl.load(f.data)
		} else if !focused {
			// The data may have changed, e.g. by undo.
			if v, _ := propValue(f.data, fl.target, fl.desc); v != fl.ed.Text() {
				fl.load(f.data)
			}
		}
	}
}

// load sets the field to the value of its target.
func (fl *propField) load(d *track.File) {
	v, _ := propValue(d, fl.target, fl.desc)
	fl.ed.SingleLine, fl.ed.Submit = true, true
	fl.ed.SetText(v)
	fl.loaded = v
}

// commitField applies the text of the field to its target, as a change that can be undone.
func (a *App) commitField(fl *propField) {
	f := a.editing
	if f == nil || !fl.bound {
		return
	}
	v := strings.TrimSpace(fl.ed.Text())
	cur, ok := propValue(f.data, fl.target, fl.desc)
	fl.loaded = fl.ed.Text()
	if !ok || cur == v {
		return
	}
	t, desc := fl.target, fl.desc
	f.change(func(d *track.File) { setProp(d, t, desc, v) })
	a.changed()
}

// commitProps applies all property fields, e.g. before saving.
func (a *App) commitProps() {
	for i := range a.props.fields {
		if fl := &a.props.fields[i]; fl.bound && fl.ed.Text() != fl.loaded {
			a.commitField(fl)
		}
	}
}

// propsFocused reports whether a property field has the keyboard focus.
func (a *App) propsFocused(gtx layout.Context) bool {
	for i := range a.props.fields {
		if gtx.Focused(&a.props.fields[i].ed) {
			return true
		}
	}
	return false
}

// revertProps discards the unapplied text of the property fields and gives up the focus.
func (a *App) revertProps(gtx layout.Context) {
	for i := range a.props.fields {
		if fl := &a.props.fields[i]; fl.bound {
			fl.ed.SetText(fl.loaded)
		}
	}
	gtx.Execute(key.FocusCmd{})
}

// deleteRoute deletes the route shown in the properties box.
func (a *App) deleteRoute() {
	f := a.editing
	route, _, hasRoute, _ := a.propTargets()
	if f == nil || !hasRoute || route.route >= len(f.data.Routes) {
		return
	}
	r := route.route
	for i := range a.props.fields {
		a.props.fields[i].bound = false
	}
	f.change(func(d *track.File) { d.Routes = slices.Delete(d.Routes, r, r+1) })
	a.editor.reset()
	a.changed()
}

// layoutProps draws the properties box for the selection of the edited file, if there is one.
func (a *App) layoutProps(gtx layout.Context) layout.Dimensions {
	f := a.editing
	if f == nil {
		return layout.Dimensions{}
	}
	_, point, hasRoute, hasPoint := a.propTargets()
	if !hasRoute && !hasPoint {
		return layout.Dimensions{}
	}
	st, p := a.style, &a.props
	d := f.data

	var children []layout.FlexChild
	heading := func(txt string, right layout.Widget) {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: st.Spacing, Bottom: st.Spacing / 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						l := st.Label(txt)
						l.Font.Weight = font.Bold
						l.MaxLines = 1
						return l.Layout(gtx)
					}),
					layout.Rigid(right),
				)
			})
		}))
	}
	field := func(label string, i int) {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: st.Spacing / 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutPropField(gtx, label, &p.fields[i])
			})
		}))
	}
	none := func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }

	if hasRoute {
		heading("Route", func(gtx layout.Context) layout.Dimensions {
			return p.deleteRoute.Layout(gtx, st, "Delete", "Delete the whole route", true)
		})
		field("Name", routeNameField)
		field("Desc", routeDescField)
	}
	if hasPoint {
		title := "Waypoint"
		if point.route >= 0 {
			title = fmt.Sprintf("Point %d of %d", point.point+1, len(d.Routes[point.route].Points))
		}
		if len(linkedGroup(d, point.vertex())) > 1 {
			title += " · linked"
		}
		heading(title, none)
		field("Name", pointNameField)
		field("Desc", pointDescField)
	}
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{Top: st.Spacing}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
	call := macro.Stop()
	// A divider above the box.
	paint.FillShape(gtx.Ops, st.DividerColor, clip.Rect{Max: image.Pt(dims.Size.X, max(1, gtx.Dp(1)))}.Op())
	call.Add(gtx.Ops)
	return dims
}

// propLabelWidth is the width of the labels of the property fields.
const propLabelWidth unit.Dp = 40

// layoutPropField draws a labeled text field.
func (a *App) layoutPropField(gtx layout.Context, label string, fl *propField) layout.Dimensions {
	st := a.style
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(propLabelWidth)
			l := st.SmallLabel(label)
			l.Color = st.HintFg
			return l.Layout(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			border := max(1, gtx.Dp(1))
			macro := op.Record(gtx.Ops)
			dims := layout.UniformInset(3).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				ed := material.Editor(st.Theme, &fl.ed, "")
				ed.TextSize = st.TextSize
				return ed.Layout(gtx)
			})
			call := macro.Stop()
			outer := image.Rectangle{Max: dims.Size}
			col := st.PanelBorder
			if gtx.Focused(&fl.ed) {
				col = st.EditActive
			}
			paint.FillShape(gtx.Ops, col, clip.Rect(outer).Op())
			paint.FillShape(gtx.Ops, st.PanelBg, clip.Rect(outer.Inset(border)).Op())
			call.Add(gtx.Ops)
			return dims
		}),
	)
}

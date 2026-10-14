package ui

import (
	"image"
	"math"
	"slices"
	"sort"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/track"
)

// trackHover is a position on the selected item, under the pointer in the chart or near it on the map.
// It is marked in both the chart and the map.
type trackHover struct {
	valid bool
	// x is the position on the chart's horizontal axis, see [chartData.xs].
	x   float64
	pos geo.Point
	// value is the value of the chart's metric at the position, NaN if there is none.
	// terrain is the terrain elevation, NaN if not shown.
	value, terrain float64
	// preview is set for the new end of a route being drawn, at the pointer. Then x is the length
	// of the route with it in meters, whatever the chart's axis, see [App.previewHover].
	preview bool
}

// updateHover finds the hovered position on the selected item: from the pointer position in the chart,
// or otherwise from the pointer on the map if it is close enough to the item.
func (a *App) updateHover(gtx layout.Context) {
	a.hover = trackHover{}
	if h, ok := a.previewHover(gtx); ok {
		a.hover = h
		return
	}
	c := &a.chart
	it := a.selected
	if c.info == nil || it == nil || it != c.item {
		return
	}
	switch {
	case c.hovering:
		if i, j, t, ok := locate(c.xs, c.toX(c.hoverX)); ok {
			a.hover = c.hoverAt(it.lines, i, j, t)
		}
	case a.mapView.HoverValid && a.shown(it):
		radius := float64(gtx.Dp(a.style.HoverRadius)) * a.mapView.View.Resolution()
		if i, j, t, ok := nearestSegment(it.lines, a.mapView.Hover, radius); ok {
			a.hover = c.hoverAt(it.lines, i, j, t)
		}
	}
}

// previewHover returns the hover info for the new end of the route being drawn, at the pointer or the waypoint
// it snaps to: the length of the route with it, and the elevation there, or the slope to it from the current end,
// if the chart shows these for the route.
func (a *App) previewHover(gtx layout.Context) (trackHover, bool) {
	e, f := &a.editor, a.editing
	if f == nil || !e.drawing || e.drag.active || !a.mapView.HoverValid {
		return trackHover{}, false
	}
	route := &f.data.Routes[e.drawRoute]
	if len(route.Points) == 0 {
		return trackHover{}, false
	}
	end := route.Points[len(route.Points)-1]
	if e.drawStart {
		end = route.Points[0]
	}
	pos := a.mapView.Hover
	if wi, ok := a.hoverSnap(gtx); ok {
		pos = geo.ToMercator(f.data.Waypoints[wi].Pos)
	}
	d := geo.Distance(end.Pos, geo.ToLonLat(pos))
	h := trackHover{valid: true, preview: true, pos: pos, x: route.Length() + d, value: math.NaN(), terrain: math.NaN()}

	c := &a.chart
	it := f.routeItem(e.drawRoute)
	if c.info == nil || it == nil || c.item != it {
		return h, true
	}
	ele, state := a.hoverElevation()
	if state != elevKnown {
		return h, true
	}
	switch c.info.metric {
	case track.ElevationMetric:
		h.value = ele
	case track.SlopeMetric:
		// The profile of the route has the elevations at its ends.
		if len(it.points) == 1 && len(it.points[0]) > 0 && d >= minSlopeDistance {
			pts := it.points[0]
			endEle := pts[len(pts)-1].Ele
			if e.drawStart {
				// Drawn backwards, the new segment runs from the pointer to the start.
				endEle = pts[0].Ele
				h.value = (endEle - ele) / d * 100
			} else {
				h.value = (ele - endEle) / d * 100
			}
		}
	}
	return h, true
}

// minSlopeDistance is the shortest new segment of a route being drawn that a slope is shown for, in meters.
const minSlopeDistance = 10

// hoverAt returns the hovered position at fraction t of segment j of line i of the given lines,
// which belong to the shown item.
func (c *chart) hoverAt(lines []mapview.Polyline, i, j int, t float64) trackHover {
	x := c.xs[i]
	a, b := lines[i].Points[j], lines[i].Points[j+1]
	h := trackHover{
		valid: true,
		x:     x[j] + (x[j+1]-x[j])*t,
		pos:   geo.Point{X: a.X + (b.X-a.X)*t, Y: a.Y + (b.Y-a.Y)*t},
		value: math.NaN(),
	}
	h.value = segmentValue(c.vals, i, j, t)
	h.terrain = math.NaN()
	if c.terrain != nil {
		h.terrain = segmentValue(c.terrain, i, j, t)
	}
	return h
}

// segmentValue returns the value at fraction t of segment j of line i, interpolated between the points
// like in the chart, see [columnMeans]. It is NaN if the segment has no value.
func segmentValue(vals [][]float64, i, j int, t float64) float64 {
	if v := vals[i]; j < len(v) && !math.IsNaN(v[j]) {
		v0, v1 := pointValue(v, j), pointValue(v, j+1)
		return v0 + (v1-v0)*t
	}
	return math.NaN()
}

// locate returns the line i and segment j at position d on the horizontal axis, given the non-decreasing
// position of each point, and the fraction t of the segment before d. It reports false if d is not on any line.
func locate(xs [][]float64, d float64) (i, j int, t float64, ok bool) {
	for i, line := range xs {
		if len(line) < 2 || d < line[0] || d > line[len(line)-1] {
			continue
		}
		j := min(max(sort.SearchFloat64s(line, d)-1, 0), len(line)-2)
		if l := line[j+1] - line[j]; l > 0 {
			t = math.Min(math.Max((d-line[j])/l, 0), 1)
		}
		return i, j, t, true
	}
	return 0, 0, 0, false
}

// nearestSegment returns the line i and segment j closest to p, and the fraction t of the segment
// before the closest point. It reports false if no segment is within radius.
//
// It checks all segments of lines whose bounds are near p, on every frame while the pointer is on the map.
// If this gets too slow for long tracks, we may have to add an acceleration structure,
// like a k-d tree or a grid of segments, built once per item.
func nearestSegment(lines []mapview.Polyline, p geo.Point, radius float64) (i, j int, t float64, ok bool) {
	best := radius * radius
	for li, l := range lines {
		b := l.Bounds
		if p.X < b.Min.X-radius || p.X > b.Max.X+radius || p.Y < b.Min.Y-radius || p.Y > b.Max.Y+radius {
			continue
		}
		for k := 0; k+1 < len(l.Points); k++ {
			a, b := l.Points[k], l.Points[k+1]
			dx, dy := b.X-a.X, b.Y-a.Y
			s := 0.0
			if l2 := dx*dx + dy*dy; l2 > 0 {
				s = math.Min(math.Max(((p.X-a.X)*dx+(p.Y-a.Y)*dy)/l2, 0), 1)
			}
			ex, ey := a.X+dx*s-p.X, a.Y+dy*s-p.Y
			if d2 := ex*ex + ey*ey; d2 <= best {
				best, i, j, t, ok = d2, li, k, s, true
			}
		}
	}
	return i, j, t, ok
}

// hoverPart is a part of the hover label: a text with an icon in front, telling what it is.
type hoverPart struct {
	icon *widget.Icon
	text string
}

// hoverParts returns the parts of the label for the hovered position: the value, if any, the terrain elevation,
// if shown, and the position on the horizontal axis.
func (a *App) hoverParts() []hoverPart {
	c := &a.chart
	var parts []hoverPart
	if !math.IsNaN(a.hover.value) {
		parts = append(parts, hoverPart{c.info.icon, c.info.formatValue(a.hover.value)})
	}
	if !math.IsNaN(a.hover.terrain) {
		parts = append(parts, hoverPart{iconTerrain, c.info.formatValue(a.hover.terrain)})
	}
	if a.hover.preview {
		return append(parts, hoverPart{iconDistance, formatDistance(a.hover.x)})
	}
	return append(parts, hoverPart{chartAxes[c.xAxis].icon, c.formatX(a.hover.x)})
}

// layoutHoverParts draws the parts of the hover label in a row.
func (a *App) layoutHoverParts(gtx layout.Context) layout.Dimensions {
	st := a.style
	parts := a.hoverParts()
	children := make([]layout.FlexChild, 0, 2*len(parts))
	for i, p := range parts {
		if i > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Width: st.Spacing * 2}.Layout))
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := gtx.Sp(st.SmallTextSize)
			layoutIcon(gtx, p.icon, image.Point{}, size, st.Theme.Fg)
			return layout.Dimensions{Size: image.Pt(size+gtx.Dp(2), size)}
		}), layout.Rigid(st.SmallLabel(p.text).Layout))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

// recordHoverLabel records the label for the hovered position on its background, at the origin.
func (a *App) recordHoverLabel(gtx layout.Context) (op.CallOp, image.Point) {
	st := a.style
	pad := gtx.Dp(2)
	macro := op.Record(gtx.Ops)
	lgtx := gtx
	lgtx.Constraints = layout.Constraints{Max: gtx.Constraints.Max}
	inner := op.Record(gtx.Ops)
	dims := a.layoutHoverParts(lgtx)
	label := inner.Stop()
	size := dims.Size.Add(image.Pt(2*pad, 0))
	paint.FillShape(gtx.Ops, st.LegendBg, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(st.CornerRadius)).Op(gtx.Ops))
	stack := op.Offset(image.Pt(pad, 0)).Push(gtx.Ops)
	label.Add(gtx.Ops)
	stack.Pop()
	return macro.Stop(), size
}

// layoutHoverChart marks the hovered position in the chart with a vertical line and a dot on the value,
// with the label at the top of the plot.
func (a *App) layoutHoverChart(gtx layout.Context) {
	c := &a.chart
	h := a.hover
	plot := c.plot
	if !h.valid || h.preview && c.xAxis.timed() || h.x < c.from || h.x > c.to || plot.Dx() <= 0 {
		return
	}
	st := a.style
	x := plot.Min.X + int(math.Round((h.x-c.from)/(c.to-c.from)*float64(plot.Dx())))
	w := max(1, gtx.Dp(1))
	paint.FillShape(gtx.Ops, st.HoverColor, clip.Rect(image.Rect(x-w/2, plot.Min.Y, x-w/2+w, plot.Max.Y)).Op())
	if y, ok := c.valueY(h.value); ok && y >= float32(plot.Min.Y) && y <= float32(plot.Max.Y) {
		a.drawHoverDot(gtx, image.Pt(x, int(math.Round(float64(y)))))
	}

	call, size := a.recordHoverLabel(gtx)
	lx := min(max(x-size.X/2, plot.Min.X), plot.Max.X-size.X)
	defer op.Offset(image.Pt(lx, plot.Min.Y)).Push(gtx.Ops).Pop()
	call.Add(gtx.Ops)
}

// layoutHoverMap marks the hovered position on the map with a dot, with the label beside it.
func (a *App) layoutHoverMap(gtx layout.Context) {
	h := a.hover
	if !h.valid {
		return
	}
	fx, fy := a.mapView.View.ToScreen(h.pos)
	p := image.Pt(int(math.Round(fx)), int(math.Round(fy)))
	a.drawHoverDot(gtx, p)

	// The label is above right of the dot, clear of the mouse cursor, and flipped at the edges of the map.
	call, size := a.recordHoverLabel(gtx)
	gap := gtx.Dp(a.style.HoverDotSize)
	lx, ly := p.X+gap, p.Y-gap-size.Y
	if lx+size.X > gtx.Constraints.Max.X {
		lx = p.X - gap - size.X
	}
	if ly < 0 {
		ly = p.Y + gap
	}
	defer op.Offset(image.Pt(lx, ly)).Push(gtx.Ops).Pop()
	call.Add(gtx.Ops)
}

// drawHoverDot draws the dot marking the hovered position, centered on p.
func (a *App) drawHoverDot(gtx layout.Context, p image.Point) {
	st := a.style
	r := gtx.Dp(st.HoverDotSize) / 2
	ring := max(1, gtx.Dp(2))
	outer := image.Rectangle{Min: p.Sub(image.Pt(r+ring, r+ring)), Max: p.Add(image.Pt(r+ring, r+ring))}
	inner := image.Rectangle{Min: p.Sub(image.Pt(r, r)), Max: p.Add(image.Pt(r, r))}
	paint.FillShape(gtx.Ops, st.HoverRing, clip.Ellipse(outer).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, st.HoverColor, clip.Ellipse(inner).Op(gtx.Ops))
}

// layoutWaypointHover draws the name of the waypoint or route point marker under the pointer on the map,
// for the shown files except the edited one, whose names are always shown, see [App.layoutEditMap].
// Not drawn for items whose names are all shown, see [App.layoutPointNames].
func (a *App) layoutWaypointHover(gtx layout.Context) {
	if !a.mapView.HoverValid || !a.showing(showHoverNames) {
		return
	}
	view := &a.mapView.View
	hx, hy := view.ToScreen(a.mapView.Hover)
	hover := f32.Pt(float32(hx), float32(hy))
	hit := float32(gtx.Dp(a.style.HoverRadius))
	var (
		best   = float32(math.Inf(1))
		name   string
		at     f32.Point
		radius float32
	)
	for _, it := range a.shownItems() {
		if a.namesShown(it) || (a.editing != nil && slices.Contains(a.editing.items, it)) {
			continue
		}
		dots, names := a.itemDots(it)
		for i, d := range dots {
			if i >= len(names) || names[i] == "" {
				continue
			}
			r := a.dotRadius(gtx, d.Icon != nil)
			x, y := view.ToScreen(d.Pos)
			p := f32.Pt(float32(x), float32(y))
			if d2 := dist2(p, hover); d2 <= max(r, hit)*max(r, hit) && d2 < best {
				best, name, at, radius = d2, names[i], p, r
			}
		}
	}
	if name == "" {
		return
	}
	a.drawPointLabel(gtx, at, radius, name, func(f32.Point) bool { return true })
}

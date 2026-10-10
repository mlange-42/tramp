package ui

import (
	"image"
	"math"
	"sort"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/mapview"
)

// trackHover is a position on the selected item, under the pointer in the chart or near it on the map.
// It is marked in both the chart and the map.
type trackHover struct {
	valid bool
	// dist is the distance along the item, in meters.
	dist float64
	pos  geo.Point
	// value is the value of the chart's metric at the position, NaN if there is none.
	value float64
}

// updateHover finds the hovered position on the selected item: from the pointer position in the chart,
// or otherwise from the pointer on the map if it is close enough to the item.
func (a *App) updateHover(gtx layout.Context) {
	a.hover = trackHover{}
	c := &a.chart
	it := a.selected
	if c.info == nil || it == nil || it != c.item {
		return
	}
	switch {
	case c.hovering:
		if i, j, t, ok := locate(c.dist, c.toDist(c.hoverX)); ok {
			a.hover = c.hoverAt(it.lines, i, j, t)
		}
	case a.mapView.HoverValid && a.shown(it):
		radius := float64(gtx.Dp(a.style.HoverRadius)) * a.mapView.View.Resolution()
		if i, j, t, ok := nearestSegment(it.lines, a.mapView.Hover, radius); ok {
			a.hover = c.hoverAt(it.lines, i, j, t)
		}
	}
}

// hoverAt returns the hovered position at fraction t of segment j of line i of the given lines,
// which belong to the shown item.
func (c *chart) hoverAt(lines []mapview.Polyline, i, j int, t float64) trackHover {
	d := c.dist[i]
	a, b := lines[i].Points[j], lines[i].Points[j+1]
	h := trackHover{
		valid: true,
		dist:  d[j] + (d[j+1]-d[j])*t,
		pos:   geo.Point{X: a.X + (b.X-a.X)*t, Y: a.Y + (b.Y-a.Y)*t},
		value: math.NaN(),
	}
	// Values are interpolated between points like in the chart, see [columnMeans].
	if v := c.vals[i]; j < len(v) && !math.IsNaN(v[j]) {
		v0, v1 := pointValue(v, j), pointValue(v, j+1)
		h.value = v0 + (v1-v0)*t
	}
	return h
}

// locate returns the line i and segment j at distance d along the item, given the distance of each point,
// and the fraction t of the segment before d. It reports false if d is not on any line.
func locate(dist [][]float64, d float64) (i, j int, t float64, ok bool) {
	for i, line := range dist {
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

// hoverText returns the label for the hovered position: the value, if any, and the distance along the item.
func (a *App) hoverText() string {
	dist := formatDistance(a.hover.dist)
	if math.IsNaN(a.hover.value) {
		return dist
	}
	return a.chart.info.formatValue(a.hover.value) + " · " + dist
}

// recordHoverLabel records the label for the hovered position on its background, at the origin.
func (a *App) recordHoverLabel(gtx layout.Context) (op.CallOp, image.Point) {
	st := a.style
	pad := gtx.Dp(2)
	macro := op.Record(gtx.Ops)
	lgtx := gtx
	lgtx.Constraints = layout.Constraints{Max: gtx.Constraints.Max}
	inner := op.Record(gtx.Ops)
	dims := st.SmallLabel(a.hoverText()).Layout(lgtx)
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
	if !h.valid || h.dist < c.from || h.dist > c.to || plot.Dx() <= 0 {
		return
	}
	st := a.style
	x := plot.Min.X + int(math.Round((h.dist-c.from)/(c.to-c.from)*float64(plot.Dx())))
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

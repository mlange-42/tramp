package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
	"sort"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/track"
)

const (
	// chartFillAlpha is the opacity of the filled area below the chart line.
	chartFillAlpha = 0xb0
	// chartQuantile is the share of pixel columns cut off at each end of the value axis,
	// so that spikes like GPS jumps don't squeeze the rest of the chart.
	chartQuantile = 0.005
	// chartScrollPerDouble is the scroll distance for zooming the chart in or out by a factor of two.
	// A mouse wheel notch scrolls 120 units on most platforms.
	chartScrollPerDouble = 240
	// chartMinSpan is the shortest distance the chart can be zoomed to, in meters.
	chartMinSpan = 20
)

// chart shows a metric of a track or route over the distance along it.
//
// The values are averaged per pixel column, so that the drawing doesn't depend on the number of points.
// The drawing is cached and only rebuilt when the content, the size or the shown range changes.
//
// The chart is zoomed with the scroll wheel and panned by dragging with the secondary (right) mouse button,
// both only along the distance axis.
type chart struct {
	chartData
	version int

	// item is the shown item. The shown range is kept while the item stays the same.
	item *fileItem
	// from and to are the shown distance range, in meters.
	from, to float64
	// plot is the plot area at the last layout, for converting pointer positions.
	plot     image.Rectangle
	dragging bool
	dragID   pointer.ID
	lastX    float32

	// yRange caches the value range of the whole item, see [chart.valueRange].
	yRange chartYRange

	cache op.Ops
	call  op.CallOp
	key   chartKey
}

// chartData is the content of the chart.
type chartData struct {
	// info is the shown metric, nil if there is nothing to show.
	info *metricInfo
	// dist is the distance along the item for each point, continuing over all lines, in meters.
	dist [][]float64
	// vals are the values per line segment, see [fileItem.lineValues]. Nil if there are none.
	vals [][]float64
	// total is the length of the item, in meters.
	total float64
	// coloring colors the area by value. If nil, it is drawn in color.
	coloring *mapview.Coloring
	color    color.NRGBA
	// hint is shown instead of the chart if there is nothing to show.
	hint string
}

type chartKey struct {
	version  int
	size     image.Point
	pxPerDp  float32
	from, to float64
}

// updateChart shows the selected item in the chart, with the metric tracks are colored by.
// Without coloring, the chart shows the elevation.
func (a *App) updateChart() {
	c := &a.chart
	c.version++
	c.chartData = chartData{}
	it := a.selected
	if it == nil {
		c.hint = "Click a track or route to show its profile"
		return
	}
	idx := a.colorBy.Selected()
	if metrics[idx].metric == track.NoMetric {
		idx = metricIndex(track.ElevationMetric)
	} else if a.legend != nil {
		c.coloring = a.legend.coloring()
	}
	info := &metrics[idx]
	c.vals = it.lineValues(info.metric)
	if c.vals == nil {
		c.hint = fmt.Sprintf("No %s data", info.key)
		return
	}
	c.info = info
	c.dist = it.distances()
	for _, d := range c.dist {
		if len(d) > 0 {
			c.total = d[len(d)-1]
		}
	}
	c.color = it.color
	c.color.A = chartFillAlpha
	if c.total <= 0 {
		c.info = nil
		c.hint = "Too short for a profile"
	}
	if it != c.item {
		c.item = it
		c.from, c.to = 0, c.total
	}
	c.setRange(c.from, c.to-c.from)
}

// clampSpan limits a span of the distance range to between [chartMinSpan] and the whole item.
func (c *chart) clampSpan(span float64) float64 {
	return math.Min(math.Max(span, math.Min(chartMinSpan, c.total)), c.total)
}

// setRange shows the given distance range, with the span limited by [chart.clampSpan]
// and shifted to lie within the item.
func (c *chart) setRange(from, span float64) {
	span = c.clampSpan(span)
	if !(span > 0) {
		c.from, c.to = 0, c.total
		return
	}
	from = math.Min(math.Max(from, 0), c.total-span)
	c.from, c.to = from, from+span
}

// toDist converts a horizontal pointer position to a distance along the item.
func (c *chart) toDist(x float32) float64 {
	return c.from + float64(x-float32(c.plot.Min.X))/float64(c.plot.Dx())*(c.to-c.from)
}

// update applies zooming and panning.
func (c *chart) update(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  c,
			Kinds:   pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Scroll,
			ScrollY: pointer.ScrollRange{Min: math.MinInt32, Max: math.MaxInt32},
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok || c.info == nil || c.plot.Dx() <= 0 {
			continue
		}
		switch e.Kind {
		case pointer.Press:
			if !c.dragging && e.Buttons.Contain(pointer.ButtonSecondary) {
				c.dragging, c.dragID, c.lastX = true, e.PointerID, e.Position.X
				gtx.Execute(pointer.GrabCmd{Tag: c, ID: e.PointerID})
			}
		case pointer.Drag:
			if c.dragging && e.PointerID == c.dragID {
				c.setRange(c.from+c.toDist(c.lastX)-c.toDist(e.Position.X), c.to-c.from)
				c.lastX = e.Position.X
			}
		case pointer.Release, pointer.Cancel:
			// Releasing another button doesn't end the drag.
			if e.PointerID == c.dragID && (e.Kind == pointer.Cancel || !e.Buttons.Contain(pointer.ButtonSecondary)) {
				c.dragging = false
			}
		case pointer.Scroll:
			// Zoom around the distance under the pointer.
			d := c.toDist(e.Position.X)
			span := c.to - c.from
			ns := c.clampSpan(span * math.Pow(2, float64(e.Scroll.Y)/chartScrollPerDouble))
			c.setRange(d-(d-c.from)/span*ns, ns)
		}
	}
}

// metricIndex returns the index of the metric in [metrics].
func metricIndex(m track.Metric) int {
	for i, info := range metrics {
		if info.metric == m {
			return i
		}
	}
	return 0
}

// layoutChart draws the chart of the selected item below the map.
func (a *App) layoutChart(gtx layout.Context) layout.Dimensions {
	st := a.style
	paint.Fill(gtx.Ops, st.SideBg)
	c := &a.chart
	if c.info == nil {
		st.SideInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			l := st.SmallLabel(c.hint)
			l.Color = st.HintFg
			return l.Layout(gtx)
		})
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	c.plot = chartPlot(gtx, st)
	c.update(gtx)
	key := chartKey{version: c.version, size: gtx.Constraints.Max, pxPerDp: gtx.Metric.PxPerDp, from: c.from, to: c.to}
	if key != c.key {
		c.key = key
		c.cache.Reset()
		cgtx := gtx
		cgtx.Ops = &c.cache
		macro := op.Record(cgtx.Ops)
		c.draw(cgtx, st)
		c.call = macro.Stop()
	}
	c.call.Add(gtx.Ops)

	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, c)
	if c.dragging {
		pointer.CursorGrabbing.Add(gtx.Ops)
	}
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

// chartPlot returns the plot area of the chart for the maximum constraints.
// Below it is a narrow strip for the distance labels.
func chartPlot(gtx layout.Context, st *Style) image.Rectangle {
	size := gtx.Constraints.Max
	inset := gtx.Dp(st.Spacing / 2)
	return image.Rect(inset, inset, size.X-inset, size.Y-chartLabelHeight(gtx, st))
}

// chartLabelHeight returns the height of the strip for the distance labels.
func chartLabelHeight(gtx layout.Context, st *Style) int {
	return int(math.Ceil(float64(gtx.Sp(st.SmallTextSize)) * 1.3))
}

// draw draws the chart into the maximum constraints.
// Tick labels are drawn inside the plot for the y axis, and in a narrow strip below it for the x axis.
func (c *chart) draw(gtx layout.Context, st *Style) {
	plot := chartPlot(gtx, st)
	labelH := chartLabelHeight(gtx, st)
	if plot.Dx() < 2 || plot.Dy() < 2 {
		return
	}
	lo, hi, ok := c.valueRange(plot.Dx())
	if !ok {
		return
	}
	cols := columnMeans(c.dist, c.vals, c.from, c.to, plot.Dx())
	scale := c.info.scale
	tickPx := float64(gtx.Dp(st.ChartTickSpacing))

	// Value axis in the shown unit, rounded to nice ticks. Values are shown without decimals.
	yStep := max(1, niceStep((hi-lo)*scale/max(1, float64(plot.Dy())/tickPx)))
	yLo := math.Floor(lo*scale/yStep) * yStep
	yHi := math.Ceil(hi*scale/yStep) * yStep
	if yHi <= yLo {
		yHi = yLo + yStep
	}
	toY := func(v float64) float32 {
		return float32(plot.Max.Y) - float32((v*scale-yLo)/(yHi-yLo))*float32(plot.Dy())
	}
	// The area is filled from zero if it is in the range, otherwise from the bottom.
	base := float32(plot.Max.Y)
	if yLo < 0 && yHi > 0 {
		base = toY(0)
	}

	c.drawGrid(gtx, st, plot, labelH, yLo, yHi, yStep)
	// Values cut off from the range are clipped.
	stack := clip.Rect(plot).Push(gtx.Ops)
	c.drawArea(gtx, plot, cols, toY, base)
	c.drawLine(gtx, st, plot, cols, toY)
	stack.Pop()
	c.drawYLabels(gtx, st, plot, yLo, yHi, yStep)
}

// drawGrid draws the grid lines, and the distance labels below the plot.
func (c *chart) drawGrid(gtx layout.Context, st *Style, plot image.Rectangle, labelH int, yLo, yHi, yStep float64) {
	w := max(1, gtx.Dp(1))
	for _, v := range ticks(yLo, yHi, yStep) {
		y := plot.Max.Y - int(math.Round((v-yLo)/(yHi-yLo)*float64(plot.Dy())))
		paint.FillShape(gtx.Ops, st.ChartGrid, clip.Rect(image.Rect(plot.Min.X, y, plot.Max.X, y+w)).Op())
	}

	// Distance labels are wider than value labels, so they need more space.
	span := c.to - c.from
	xStep := niceStep(span / max(1, float64(plot.Dx())/float64(3*gtx.Dp(st.ChartTickSpacing))))
	right := math.MinInt
	for _, d := range ticks(c.from, c.to, xStep) {
		x := plot.Min.X + int(math.Round((d-c.from)/span*float64(plot.Dx())))
		paint.FillShape(gtx.Ops, st.ChartGrid, clip.Rect(image.Rect(x, plot.Min.Y, x+w, plot.Max.Y)).Op())

		l := st.SmallLabel(formatTick(d, xStep))
		l.Color = st.HintFg
		macro := op.Record(gtx.Ops)
		lgtx := gtx
		lgtx.Constraints = layout.Constraints{Max: image.Pt(plot.Dx(), labelH)}
		dims := l.Layout(lgtx)
		call := macro.Stop()
		lx := min(max(x-dims.Size.X/2, plot.Min.X), plot.Max.X-dims.Size.X)
		if lx < right+gtx.Dp(st.Spacing) {
			continue
		}
		right = lx + dims.Size.X
		stack := op.Offset(image.Pt(lx, plot.Max.Y)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		stack.Pop()
	}
}

// drawYLabels draws the value labels at the left of the plot, above their grid lines.
func (c *chart) drawYLabels(gtx layout.Context, st *Style, plot image.Rectangle, yLo, yHi, yStep float64) {
	pad := gtx.Dp(2)
	for _, v := range ticks(yLo, yHi, yStep) {
		y := plot.Max.Y - int(math.Round((v-yLo)/(yHi-yLo)*float64(plot.Dy())))
		l := st.SmallLabel(fmt.Sprintf(c.info.format+" %s", v, c.info.unit))
		if v == 0 {
			// Avoid "+0" for signed formats.
			l.Text = "0 " + c.info.unit
		}
		macro := op.Record(gtx.Ops)
		lgtx := gtx
		lgtx.Constraints = layout.Constraints{Max: plot.Size()}
		dims := l.Layout(lgtx)
		call := macro.Stop()
		top := y - dims.Size.Y
		if top < plot.Min.Y {
			continue
		}
		r := image.Rect(plot.Min.X, top, plot.Min.X+dims.Size.X+2*pad, y)
		paint.FillShape(gtx.Ops, st.LegendBg, clip.Rect(r).Op())
		stack := op.Offset(image.Pt(r.Min.X+pad, top)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		stack.Pop()
	}
}

// drawArea fills each pixel column from the base to its value, colored by value or in the chart's color.
func (c *chart) drawArea(gtx layout.Context, plot image.Rectangle, cols []float64, toY func(float64) float32, base float32) {
	nBins := 1
	if c.coloring != nil {
		nBins = len(c.coloring.Colors)
	}
	// Only one path can be built at a time, so the columns are collected per color first.
	bins := make([][]int, nBins)
	for i, v := range cols {
		if math.IsNaN(v) {
			continue
		}
		b := 0
		if c.coloring != nil {
			b = c.coloring.Index(v)
		}
		bins[b] = append(bins[b], i)
	}
	for b, idx := range bins {
		if len(idx) == 0 {
			continue
		}
		var p clip.Path
		p.Begin(gtx.Ops)
		for _, i := range idx {
			x0, x1 := float32(plot.Min.X+i), float32(plot.Min.X+i+1)
			y := toY(cols[i])
			p.MoveTo(f32.Pt(x0, base))
			p.LineTo(f32.Pt(x0, y))
			p.LineTo(f32.Pt(x1, y))
			p.LineTo(f32.Pt(x1, base))
			p.Close()
		}
		col := c.color
		if c.coloring != nil {
			col = c.coloring.Colors[b]
			col.A = chartFillAlpha
		}
		paint.FillShape(gtx.Ops, col, clip.Outline{Path: p.End()}.Op())
	}
}

// drawLine draws a line through the column values, interrupted where there are none.
// Like the track lines on the map, the line is drawn as one rectangle per step,
// which is much faster than [clip.Stroke].
func (c *chart) drawLine(gtx layout.Context, st *Style, plot image.Rectangle, cols []float64, toY func(float64) float32) {
	hw := gtx.Metric.PxPerDp * float32(st.ChartLineWidth) / 2
	var path clip.Path
	path.Begin(gtx.Ops)
	prev, havePrev := f32.Point{}, false
	for i, v := range cols {
		if math.IsNaN(v) {
			havePrev = false
			continue
		}
		p := f32.Pt(float32(plot.Min.X+i)+0.5, toY(v))
		if havePrev {
			segmentRect(&path, prev, p, hw)
		}
		prev, havePrev = p, true
	}
	paint.FillShape(gtx.Ops, st.ChartLine, clip.Outline{Path: path.End()}.Op())
}

// segmentRect adds a rectangle of half width hw around the segment from a to b,
// extended by hw at both ends so that neighbors overlap at joints.
func segmentRect(path *clip.Path, a, b f32.Point, hw float32) {
	d := b.Sub(a)
	length := float32(math.Hypot(float64(d.X), float64(d.Y)))
	if length == 0 {
		return
	}
	d = d.Mul(hw / length)
	n := f32.Pt(-d.Y, d.X)
	a, b = a.Sub(d), b.Add(d)
	path.MoveTo(a.Add(n))
	path.LineTo(b.Add(n))
	path.LineTo(b.Sub(n))
	path.LineTo(a.Sub(n))
	path.Close()
}

// columnMeans returns the mean value for n equal-width columns over the distance from from to to,
// weighted by the length of the line segments in each column. Columns without values are NaN.
func columnMeans(dist, vals [][]float64, from, to float64, n int) []float64 {
	sum := make([]float64, n)
	weight := make([]float64, n)
	scale := float64(n) / (to - from)
	for i, line := range vals {
		d := dist[i]
		if len(line) == 0 || d[0] > to || d[len(d)-1] < from {
			continue
		}
		// Start at the last segment that begins before the range.
		start := max(0, sort.SearchFloat64s(d, from)-1)
		for j := start; j < len(line) && d[j] <= to; j++ {
			v := line[j]
			if math.IsNaN(v) {
				continue
			}
			x0, x1 := (d[j]-from)*scale, (d[j+1]-from)*scale
			for col := max(0, int(x0)); col < n && float64(col) < x1; col++ {
				if w := math.Min(x1, float64(col+1)) - math.Max(x0, float64(col)); w > 0 {
					sum[col] += v * w
					weight[col] += w
				}
			}
		}
	}
	for i := range sum {
		if weight[i] > 0 {
			sum[i] /= weight[i]
		} else {
			sum[i] = math.NaN()
		}
	}
	return sum
}

// chartYRange is the value range of the whole item for a plot width.
type chartYRange struct {
	version, width int
	lo, hi         float64
	ok             bool
}

// valueRange returns the range of the value axis: the range of the column values of the whole item,
// for the given plot width. It doesn't change with zooming or panning.
func (c *chart) valueRange(width int) (lo, hi float64, ok bool) {
	r := &c.yRange
	if r.version != c.version || r.width != width {
		*r = chartYRange{version: c.version, width: width}
		r.lo, r.hi, r.ok = columnRange(columnMeans(c.dist, c.vals, 0, c.total, width))
	}
	return r.lo, r.hi, r.ok
}

// columnRange returns the range of the column values without the outer [chartQuantile] at each end,
// ignoring NaN.
func columnRange(cols []float64) (lo, hi float64, ok bool) {
	vals := make([]float64, 0, len(cols))
	for _, v := range cols {
		if !math.IsNaN(v) {
			vals = append(vals, v)
		}
	}
	if len(vals) == 0 {
		return 0, 0, false
	}
	slices.Sort(vals)
	at := func(q float64) float64 { return vals[int(math.Round(q*float64(len(vals)-1)))] }
	return at(chartQuantile), at(1 - chartQuantile), true
}

// niceStep returns the smallest of 1, 2 or 5 times a power of ten that is at least raw.
func niceStep(raw float64) float64 {
	if !(raw > 0) || math.IsInf(raw, 0) {
		return 1
	}
	pow := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 5} {
		if m*pow >= raw*(1-1e-9) {
			return m * pow
		}
	}
	return 10 * pow
}

// ticks returns the multiples of step from lo to hi, inclusive.
func ticks(lo, hi, step float64) []float64 {
	var out []float64
	for i := math.Ceil(lo/step - 1e-9); i*step <= hi+step*1e-9; i++ {
		out = append(out, i*step)
	}
	return out
}

// formatTick formats a distance tick, in meters below 1 km, otherwise in kilometers
// with as many decimals as the tick step needs.
func formatTick(m, step float64) string {
	switch {
	case m == 0:
		return "0"
	case m < 1000:
		return fmt.Sprintf("%.0f m", m)
	case step >= 1000:
		return fmt.Sprintf("%.0f km", m/1000)
	}
	decimals := int(math.Ceil(-math.Log10(step/1000) - 1e-9))
	return fmt.Sprintf("%.*f km", decimals, m/1000)
}

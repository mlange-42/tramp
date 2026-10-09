package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"

	"gioui.org/f32"
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
)

// chart shows a metric of a track or route over the distance along it.
//
// The values are averaged per pixel column, so that the drawing doesn't depend on the number of points.
// The drawing is cached and only rebuilt when the content or the size changes.
type chart struct {
	chartData
	version int

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
	version int
	size    image.Point
	pxPerDp float32
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
	key := chartKey{version: c.version, size: gtx.Constraints.Max, pxPerDp: gtx.Metric.PxPerDp}
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
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

// draw draws the chart into the maximum constraints.
// Tick labels are drawn inside the plot for the y axis, and in a narrow strip below it for the x axis.
func (c *chart) draw(gtx layout.Context, st *Style) {
	size := gtx.Constraints.Max
	inset := gtx.Dp(st.Spacing / 2)
	labelH := int(math.Ceil(float64(gtx.Sp(st.SmallTextSize)) * 1.3))
	plot := image.Rect(inset, inset, size.X-inset, size.Y-labelH)
	if plot.Dx() < 2 || plot.Dy() < 2 {
		return
	}
	cols := columnMeans(c.dist, c.vals, c.total, plot.Dx())
	lo, hi, ok := columnRange(cols)
	if !ok {
		return
	}
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
	xStep := niceStep(c.total / max(1, float64(plot.Dx())/float64(3*gtx.Dp(st.ChartTickSpacing))))
	right := math.MinInt
	for _, d := range ticks(0, c.total, xStep) {
		x := plot.Min.X + int(math.Round(d/c.total*float64(plot.Dx())))
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

// columnMeans returns the mean value for n equal-width columns over the distance from 0 to total,
// weighted by the length of the line segments in each column. Columns without values are NaN.
func columnMeans(dist, vals [][]float64, total float64, n int) []float64 {
	sum := make([]float64, n)
	weight := make([]float64, n)
	scale := float64(n) / total
	for i, line := range vals {
		d := dist[i]
		for j, v := range line {
			if math.IsNaN(v) {
				continue
			}
			x0, x1 := d[j]*scale, d[j+1]*scale
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

// formatTick formats a distance tick, in meters or kilometers depending on the tick step.
func formatTick(m, step float64) string {
	if m == 0 {
		return "0"
	}
	if step < 1000 {
		return fmt.Sprintf("%g m", m)
	}
	return fmt.Sprintf("%g km", m/1000)
}

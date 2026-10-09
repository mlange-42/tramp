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
	// colorBins is the number of colors a gradient is split into for drawing.
	colorBins = 32
	// rangeQuantile is the share of values cut off at each end of the color range,
	// so that outliers like GPS jumps don't squeeze the colors of all other values.
	rangeQuantile = 0.02
)

// casingColor is drawn below colored lines.
var casingColor = color.NRGBA{A: 0xcc}

// metricInfo describes how a metric is shown.
type metricInfo struct {
	metric track.Metric
	// key is the name in the settings file.
	key  string
	name string
	unit string
	// scale converts values to the shown unit.
	scale float64
	// format formats a value in the shown unit.
	format string
	// gradient is the name of the default gradient.
	gradient string
	// symmetric centers the range on zero, for diverging gradients.
	symmetric bool
}

// metrics are the metrics to color tracks by, starting with no coloring.
var metrics = []metricInfo{
	{metric: track.NoMetric, key: "none", name: "None"},
	{metric: track.SpeedMetric, key: "speed", name: "Speed", unit: "km/h", scale: 3.6, format: "%.0f", gradient: "Turbo"},
	{metric: track.ElevationMetric, key: "elevation", name: "Elevation", unit: "m", scale: 1, format: "%.0f", gradient: "Viridis"},
	{metric: track.SlopeMetric, key: "slope", name: "Slope", unit: "%", scale: 1, format: "%+.0f", gradient: "Blue–Red", symmetric: true},
}

// colorMetric returns the selected metric to color tracks by.
func (a *App) colorMetric() track.Metric {
	return metrics[a.colorBy.Selected()].metric
}

// updateColoring processes input on the coloring controls.
func (a *App) updateColoring(gtx layout.Context) {
	if a.colorBy.Update(gtx) {
		a.gradientSel.SetSelected(a.metricGradients[a.colorBy.Selected()])
		a.tracksChanged = true
	}
	if a.gradientSel.Update(gtx) {
		a.metricGradients[a.colorBy.Selected()] = a.gradientSel.Selected()
		a.tracksChanged = true
	}
}

// legend describes the current coloring by value.
type legend struct {
	info     *metricInfo
	gradient *Gradient
	// min and max are the value range, in the metric's original unit.
	min, max float64
}

// newLegend returns the coloring for the selected metric and the values of the groups,
// or nil if tracks are not colored or there are no values.
func (a *App) newLegend(groups []mapview.LineGroup) *legend {
	info := &metrics[a.colorBy.Selected()]
	if info.metric == track.NoMetric {
		return nil
	}
	var vals []float64
	for _, g := range groups {
		for _, line := range g.Values {
			for _, v := range line {
				if !math.IsNaN(v) {
					vals = append(vals, v)
				}
			}
		}
	}
	lo, hi, ok := valueRange(vals, info.symmetric)
	if !ok {
		return nil
	}
	return &legend{info: info, gradient: &a.gradients[a.gradientSel.Selected()], min: lo, max: hi}
}

// valueRange returns the range of the values without the outer [rangeQuantile] at each end.
// If symmetric, the range is centered on zero.
func valueRange(vals []float64, symmetric bool) (lo, hi float64, ok bool) {
	if len(vals) == 0 {
		return 0, 0, false
	}
	slices.Sort(vals)
	at := func(q float64) float64 { return vals[int(math.Round(q*float64(len(vals)-1)))] }
	lo, hi = at(rangeQuantile), at(1-rangeQuantile)
	if symmetric {
		m := math.Max(math.Abs(lo), math.Abs(hi))
		lo, hi = -m, m
	}
	if hi <= lo {
		lo, hi = lo-1, hi+1
	}
	return lo, hi, true
}

func (l *legend) coloring() *mapview.Coloring {
	return &mapview.Coloring{Min: l.min, Max: l.max, Colors: l.gradient.Bins(colorBins), Casing: casingColor}
}

// label formats a value in the legend's unit.
func (l *legend) label(v float64) string {
	return fmt.Sprintf(l.info.format, v*l.info.scale)
}

// layoutLegend draws the color bar with the value range in the bottom left corner of the map.
func (a *App) layoutLegend(gtx layout.Context) {
	l := a.legend
	if l == nil {
		return
	}
	st := a.style
	layout.SW.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(st.Spacing).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			macro := op.Record(gtx.Ops)
			dims := layout.UniformInset(st.Spacing).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(st.SmallLabel(fmt.Sprintf("%s (%s)", l.info.name, l.info.unit)).Layout),
					layout.Rigid(layout.Spacer{Height: st.Spacing / 2}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layoutGradientBar(gtx, l.gradient, image.Pt(gtx.Dp(st.LegendWidth), gtx.Dp(10)))
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(st.LegendWidth)
						gtx.Constraints.Max.X = gtx.Constraints.Min.X
						return layout.Flex{}.Layout(gtx,
							layout.Rigid(st.SmallLabel(l.label(l.min)).Layout),
							layout.Flexed(1, layout.Spacer{}.Layout),
							layout.Rigid(st.SmallLabel(l.label(l.max)).Layout),
						)
					}),
				)
			})
			call := macro.Stop()
			rr := gtx.Dp(st.CornerRadius)
			paint.FillShape(gtx.Ops, st.LegendBg, clip.UniformRRect(image.Rectangle{Max: dims.Size}, rr).Op(gtx.Ops))
			call.Add(gtx.Ops)
			return dims
		})
	})
}

// layoutGradientBar draws a horizontal bar with the gradient.
func layoutGradientBar(gtx layout.Context, g *Gradient, size image.Point) layout.Dimensions {
	n := len(g.Stops) - 1
	for i := range n {
		x0 := size.X * i / n
		x1 := size.X * (i + 1) / n
		r := image.Rect(x0, 0, x1, size.Y)
		stack := clip.Rect(r).Push(gtx.Ops)
		paint.LinearGradientOp{
			Stop1:  f32.Pt(float32(x0), 0),
			Color1: g.Stops[i],
			Stop2:  f32.Pt(float32(x1), 0),
			Color2: g.Stops[i+1],
		}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		stack.Pop()
	}
	return layout.Dimensions{Size: size}
}

// metricNames are the options of the color-by drop-down.
var metricNames = names(metrics, func(m metricInfo) string { return m.name })

func names[T any](items []T, name func(T) string) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = name(it)
	}
	return out
}

// layoutColoring draws the coloring drop-downs in the toolbar.
func (a *App) layoutColoring(gtx layout.Context) layout.Dimensions {
	st := a.style
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(st.Label("Color by").Layout),
		layout.Rigid(layout.Spacer{Width: st.Spacing}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.colorBy.Layout(gtx, st, metricNames)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if a.colorMetric() == track.NoMetric {
				return layout.Dimensions{}
			}
			return layout.Inset{Left: st.Spacing}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.gradientSel.Layout(gtx, st, a.gradientNames)
			})
		}),
	)
}

// initColoring sets the available gradients and the coloring from the settings.
// Without gradients, the built-in ones are used.
// Gradient names that don't exist fall back to the metric's default, or the first gradient.
func (a *App) initColoring(grads []Gradient, colorBy string, saved map[string]string) {
	if len(grads) == 0 {
		grads = builtinGradients
	}
	a.gradients = grads
	a.gradientNames = names(grads, func(g Gradient) string { return g.Name })
	a.metricGradients = make([]int, len(metrics))
	for i, m := range metrics {
		a.metricGradients[i] = max(0, a.gradientIndex(m.gradient))
		if j := a.gradientIndex(saved[m.key]); j >= 0 {
			a.metricGradients[i] = j
		}
	}
	a.colorBy.SetSelected(max(0, slices.IndexFunc(metrics, func(m metricInfo) bool { return m.key == colorBy })))
	a.gradientSel.SetSelected(a.metricGradients[a.colorBy.Selected()])
}

// coloringState returns the coloring for saving.
func (a *App) coloringState() (colorBy string, grads map[string]string) {
	grads = map[string]string{}
	for i, m := range metrics {
		if m.metric != track.NoMetric {
			grads[m.key] = a.gradients[a.metricGradients[i]].Name
		}
	}
	return metrics[a.colorBy.Selected()].key, grads
}

// gradientIndex returns the index of the gradient with the given name, or -1.
func (a *App) gradientIndex(name string) int {
	return slices.IndexFunc(a.gradients, func(g Gradient) bool { return g.Name == name })
}

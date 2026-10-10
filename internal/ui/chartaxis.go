package ui

import (
	"fmt"
	"image"
	"math"
	"time"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

// chartAxis is what the chart shows along the horizontal axis.
type chartAxis int

const (
	// distanceAxis is the distance along the item.
	distanceAxis chartAxis = iota
	// tripTimeAxis is the time since the first point.
	tripTimeAxis
	// clockTimeAxis is the local time of day. Internally, it is the time since the first point, like [tripTimeAxis].
	clockTimeAxis
)

// chartAxes are the names of the axes in the toggle, and their keys in the settings file.
var chartAxes = []struct{ name, key string }{
	{"Dist", "distance"},
	{"Time", "trip_time"},
	{"Clock", "clock_time"},
}

// timeSteps are the tick steps for the time axes up to a day, in seconds.
var timeSteps = []float64{1, 2, 5, 10, 15, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200, 10800, 21600, 43200, 86400}

// chartAxisByKey returns the axis with the given key in the settings file, or [distanceAxis] if unknown.
func chartAxisByKey(key string) chartAxis {
	for i, ax := range chartAxes {
		if ax.key == key {
			return chartAxis(i)
		}
	}
	return distanceAxis
}

// timed reports whether the axis shows time.
func (x chartAxis) timed() bool {
	return x != distanceAxis
}

// updateAxis applies clicks on the axis toggle. It reports whether the selected axis changed.
func (c *chart) updateAxis(gtx layout.Context) bool {
	changed := false
	for i := range c.axisBtns {
		if c.axisBtns[i].Clicked(gtx) && chartAxis(i) != c.axis {
			c.axis = chartAxis(i)
			changed = true
		}
	}
	return changed
}

// layoutAxisToggle draws the toggle between the horizontal axes.
// The time axes are disabled if the item has no times.
func (a *App) layoutAxisToggle(gtx layout.Context) layout.Dimensions {
	st := a.style
	c := &a.chart
	pad := gtx.Dp(4)
	children := make([]layout.FlexChild, len(chartAxes))
	for i := range chartAxes {
		children[i] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			ax := chartAxis(i)
			enabled := !ax.timed() || c.timeOK
			if !enabled {
				gtx = gtx.Disabled()
			}
			return c.axisBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := st.SmallLabel(chartAxes[i].name)
				if !enabled {
					l.Color = st.HintFg
				}
				macro := op.Record(gtx.Ops)
				dims := l.Layout(gtx)
				call := macro.Stop()
				size := image.Pt(dims.Size.X+2*pad, gtx.Constraints.Max.Y)
				bg := st.StatusBg
				if ax == c.xAxis {
					bg = st.SelectedBg
				}
				paint.FillShape(gtx.Ops, bg, clip.Rect{Max: size}.Op())
				stack := op.Offset(image.Pt(pad, (size.Y-dims.Size.Y)/2)).Push(gtx.Ops)
				call.Add(gtx.Ops)
				stack.Pop()
				if enabled && ax != c.xAxis {
					pointer.CursorPointer.Add(gtx.Ops)
				}
				return layout.Dimensions{Size: size}
			})
		})
	}
	return layout.Flex{}.Layout(gtx, children...)
}

// xTicks returns the ticks of the horizontal axis from c.from to c.to, at least raw apart, and their labels.
func (c *chart) xTicks(raw float64) ([]float64, []string) {
	var xs []float64
	var labels []string
	switch c.xAxis {
	case tripTimeAxis:
		step := niceTimeStep(raw)
		xs = ticks(c.from, c.to, step)
		for _, x := range xs {
			labels = append(labels, formatElapsed(x, step < 60))
		}
	case clockTimeAxis:
		// Ticks are at round times of day, not at round times since the start.
		step := niceTimeStep(raw)
		start := c.start.Local()
		_, zone := start.Zone()
		off := float64(start.Unix()) + float64(start.Nanosecond())/1e9 + float64(zone)
		for _, v := range ticks(c.from+off, c.to+off, step) {
			xs = append(xs, v-off)
			labels = append(labels, formatClock(c.clock(v-off), step))
		}
	default:
		step := niceStep(raw)
		xs = ticks(c.from, c.to, step)
		for _, x := range xs {
			labels = append(labels, formatTick(x, step))
		}
	}
	return xs, labels
}

// clock returns the local time at x on a time axis.
func (c *chart) clock(x float64) time.Time {
	return c.start.Add(time.Duration(x * float64(time.Second))).Local()
}

// formatX formats a position on the horizontal axis, for the hover label.
func (c *chart) formatX(x float64) string {
	switch c.xAxis {
	case tripTimeAxis:
		return formatElapsed(x, true)
	case clockTimeAxis:
		return c.clock(x).Format("15:04:05")
	}
	return formatDistance(x)
}

// niceTimeStep returns the smallest round time step that is at least raw seconds.
func niceTimeStep(raw float64) float64 {
	for _, s := range timeSteps {
		if s >= raw*(1-1e-9) {
			return s
		}
	}
	return niceStep(raw/86400) * 86400
}

// formatElapsed formats a duration in seconds as hours and minutes, and seconds if requested.
func formatElapsed(sec float64, seconds bool) string {
	s := int(math.Round(sec))
	if seconds {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	s = int(math.Round(sec / 60))
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// formatClock formats a time of day for a tick step in seconds: as a date for steps of days,
// and with seconds for steps below a minute.
func formatClock(t time.Time, step float64) string {
	switch {
	case step >= 86400:
		return t.Format("Jan 2")
	case step < 60:
		return t.Format("15:04:05")
	}
	return t.Format("15:04")
}

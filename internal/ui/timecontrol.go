package ui

import (
	"image"
	"time"

	"gioui.org/gesture"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
)

// timeStep is the step of the time control, on full hours.
const timeStep = time.Hour

// timeControl selects the time shown by all overlays that change over time.
// Each overlay shows its valid time closest to the selected time.
type timeControl struct {
	// selected is the selected time, or zero to follow the current time.
	selected time.Time

	prev, next, now iconButton

	// drag handles clicks and drags on the time bar. While dragging, dragTime is the time under the pointer,
	// which is selected on release.
	drag     gesture.Drag
	dragging bool
	dragTime time.Time
	// barWidth is the width of the time bar in pixels at the last layout, and barFirst and barLast
	// are the times at its ends.
	barWidth          int
	barFirst, barLast time.Time
}

// target returns the selected time, or now if none is selected.
func (c *timeControl) target(now time.Time) time.Time {
	if c.selected.IsZero() {
		return now
	}
	return c.selected
}

// step returns the full hour before (dir < 0) or after (dir > 0) the target time.
func (c *timeControl) step(now time.Time, dir int) time.Time {
	return c.stepFrom(c.target(now), dir)
}

// stepFrom returns the full hour before (dir < 0) or after (dir > 0) t.
func (c *timeControl) stepFrom(t time.Time, dir int) time.Time {
	base := t.Truncate(timeStep)
	if dir > 0 {
		return base.Add(timeStep)
	}
	if base.Equal(t) {
		return base.Add(-timeStep)
	}
	return base
}

// timeAt returns the full hour at a position in pixels on the time bar, within the times of the bar.
func (c *timeControl) timeAt(x float32) time.Time {
	if c.barWidth <= 0 || !c.barLast.After(c.barFirst) {
		return c.barFirst
	}
	f := min(max(float64(x)/float64(c.barWidth), 0), 1)
	t := c.barFirst.Add(time.Duration(f * float64(c.barLast.Sub(c.barFirst)))).Round(timeStep)
	if t.Before(c.barFirst) {
		t = t.Add(timeStep)
	}
	if t.After(c.barLast) {
		t = t.Add(-timeStep)
	}
	return t
}

// timeRange are the valid times of the enabled overlays with times.
type timeRange struct {
	// first and last are the first and the last time of any overlay.
	first, last time.Time
	// common is set if there are times for which all overlays have data, from commonFirst to commonLast.
	common                  bool
	commonFirst, commonLast time.Time
}

// timeRanges returns the valid times of the enabled overlays with times, or false if there are none.
func (a *App) timeRanges() (timeRange, bool) {
	var r timeRange
	ok := false
	for i := range a.overlays {
		o := &a.overlays[i]
		if o.tiles == nil || o.times.dim == nil {
			continue
		}
		f, l, valid := o.times.dim.Bounds()
		if !valid {
			continue
		}
		if !ok || f.Before(r.first) {
			r.first = f
		}
		if !ok || l.After(r.last) {
			r.last = l
		}
		if !ok || f.After(r.commonFirst) {
			r.commonFirst = f
		}
		if !ok || l.Before(r.commonLast) {
			r.commonLast = l
		}
		ok = true
	}
	r.common = ok && !r.commonLast.Before(r.commonFirst)
	return r, ok
}

// timeBounds returns the first and the last valid time of all enabled overlays with times.
func (a *App) timeBounds() (first, last time.Time, ok bool) {
	r, ok := a.timeRanges()
	return r.first, r.last, ok
}

// selectAllTimes selects a time for all overlays, or the current time if t is zero.
func (a *App) selectAllTimes(t, now time.Time) {
	a.timeCtl.selected = t
	target := a.timeCtl.target(now)
	for i := range a.overlays {
		o := &a.overlays[i]
		if o.tiles != nil && !o.times.waiting {
			a.selectTime(o, target, true)
		}
	}
}

// timeSteps returns the times one step before and after the selected time, and whether they are
// within the valid times of the enabled overlays. ok is false if no enabled overlay has times.
func (a *App) timeSteps(now time.Time) (prev, next time.Time, canPrev, canNext, ok bool) {
	first, last, ok := a.timeBounds()
	if !ok {
		return
	}
	prev, next = a.timeCtl.step(now, -1), a.timeCtl.step(now, 1)
	return prev, next, !prev.Before(first), !next.After(last), true
}

// updateTimeControl handles the buttons and the time bar of the time strip. It runs before the map
// is drawn, so that the map shows the new time in the same frame.
func (a *App) updateTimeControl(gtx layout.Context) {
	c, now := &a.timeCtl, gtx.Now
	for {
		e, ok := c.drag.Update(gtx.Metric, gtx.Source, gesture.Horizontal)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press, pointer.Drag:
			c.dragging, c.dragTime = true, c.timeAt(e.Position.X)
		case pointer.Release:
			if c.dragging {
				a.selectAllTimes(c.dragTime, now)
			}
			c.dragging = false
		case pointer.Cancel:
			c.dragging = false
		}
	}
	// The strip takes the pointer events in its area, so that they don't reach the map below.
	for {
		if _, ok := gtx.Event(pointer.Filter{Target: c, Kinds: pointer.Press | pointer.Release | pointer.Drag}); !ok {
			break
		}
	}

	prev, next, canPrev, canNext, ok := a.timeSteps(now)
	if !ok {
		return
	}
	if c.prev.Clicked(gtx) && canPrev {
		a.selectAllTimes(prev, now)
	}
	if c.next.Clicked(gtx) && canNext {
		a.selectAllTimes(next, now)
	}
	if c.now.Clicked(gtx) {
		a.selectAllTimes(time.Time{}, now)
	}
}

// layoutTimeline draws the time strip at the bottom of the map if an enabled overlay has times,
// and returns its height. It has buttons for an hour earlier and later and back to the current time,
// the selected time, and a bar over the times of the overlays to click or drag on.
func (a *App) layoutTimeline(gtx layout.Context) int {
	r, ok := a.timeRanges()
	if !ok {
		return 0
	}
	c, st, now := &a.timeCtl, a.style, gtx.Now
	_, _, canPrev, canNext, _ := a.timeSteps(now)

	label := "Now"
	switch {
	case c.dragging:
		label = formatDataTime(c.dragTime, now)
	case !c.selected.IsZero():
		label = formatDataTime(c.selected, now)
	}
	button := func(b *iconButton, ic *widget.Icon, tip string, enabled bool) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			col := st.Theme.Fg
			if !enabled {
				col = st.HintFg
			}
			b.tip.above = true
			return b.LayoutFlat(gtx, st, ic, col, tip)
		})
	}

	macro := op.Record(gtx.Ops)
	gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, 0)
	dims := layout.Inset{Left: st.Spacing, Right: st.Spacing * 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			button(&c.prev, iconPrev, "One hour earlier", canPrev),
			// Always there, so that the bar doesn't move.
			button(&c.now, iconNow, "Back to the current time", !c.selected.IsZero()),
			button(&c.next, iconNext, "One hour later", canNext),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// A fixed width, so that the bar doesn't move while stepping.
				gtx.Constraints.Min.X = gtx.Sp(st.SmallTextSize) * 6
				return layout.Center.Layout(gtx, st.SmallLabel(label).Layout)
			}),
			layout.Rigid(layout.Spacer{Width: st.Spacing}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return a.layoutTimeBar(gtx, r)
			}),
		)
	})
	call := macro.Stop()

	defer op.Offset(image.Pt(0, gtx.Constraints.Max.Y-dims.Size.Y)).Push(gtx.Ops).Pop()
	area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
	paint.ColorOp{Color: st.LegendBg}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	event.Op(gtx.Ops, c)
	pointer.CursorDefault.Add(gtx.Ops)
	area.Pop()
	call.Add(gtx.Ops)
	return dims.Size.Y
}

// layoutTimeBar draws the bar over the times of all enabled overlays: day labels above,
// the range where all overlays have data highlighted, the current time, and a handle at the selected time.
func (a *App) layoutTimeBar(gtx layout.Context, r timeRange) layout.Dimensions {
	c, st, now := &a.timeCtl, a.style, gtx.Now
	w := gtx.Constraints.Max.X
	c.barWidth, c.barFirst, c.barLast = w, r.first, r.last
	span := r.last.Sub(r.first)
	xOf := func(t time.Time) int {
		if span <= 0 {
			return 0
		}
		return int(float64(w) * float64(t.Sub(r.first)) / float64(span))
	}

	labelH := gtx.Sp(st.SmallTextSize) * 3 / 2
	// The flex gives the bar its width as minimum, which the day labels must not take.
	lgtx := gtx
	lgtx.Constraints.Min = image.Point{}
	trackH, handleR := gtx.Dp(6), gtx.Dp(6)
	trackY := labelH + handleR - trackH/2
	h := trackY + trackH/2 + handleR + gtx.Dp(2)

	area := clip.Rect{Max: image.Pt(w, h)}.Push(gtx.Ops)
	c.drag.Add(gtx.Ops)
	pointer.CursorPointer.Add(gtx.Ops)
	area.Pop()

	rounded := func(rect image.Rectangle) clip.Op {
		return clip.UniformRRect(rect, min(rect.Dx(), rect.Dy())/2).Op(gtx.Ops)
	}
	paint.FillShape(gtx.Ops, st.DividerColor, rounded(image.Rect(0, trackY, w, trackY+trackH)))
	if r.common {
		x0 := xOf(r.commonFirst)
		paint.FillShape(gtx.Ops, st.SelectedBg, rounded(image.Rect(x0, trackY, max(xOf(r.commonLast), x0+trackH), trackY+trackH)))
	}

	// A tick at each midnight, and the weekday and date after it if there is room.
	dayW := xOf(r.first.Add(24*time.Hour)) - xOf(r.first)
	f := r.first.Local()
	for day := time.Date(f.Year(), f.Month(), f.Day(), 0, 0, 0, 0, time.Local); !day.After(r.last); day = day.AddDate(0, 0, 1) {
		x := xOf(day)
		if !day.Before(r.first) {
			paint.FillShape(gtx.Ops, st.HintFg, clip.Rect(image.Rect(x, labelH/3, x+gtx.Dp(1), trackY+trackH)).Op())
		}
		lbl := st.SmallLabel(day.Format("Mon 2"))
		lbl.Color = st.HintFg
		m := op.Record(gtx.Ops)
		ld := lbl.Layout(lgtx)
		lc := m.Stop()
		lx := max(x, 0) + gtx.Dp(3)
		if ld.Size.X+gtx.Dp(6) <= dayW && lx+ld.Size.X <= min(x+dayW, w) {
			s := op.Offset(image.Pt(lx, 0)).Push(gtx.Ops)
			lc.Add(gtx.Ops)
			s.Pop()
		}
	}

	if !now.Before(r.first) && !now.After(r.last) {
		x := xOf(now)
		paint.FillShape(gtx.Ops, st.GPSColor, clip.Rect(image.Rect(x-gtx.Dp(1), trackY-gtx.Dp(4), x+gtx.Dp(1), trackY+trackH+gtx.Dp(4))).Op())
	}

	sel := c.target(now)
	if c.dragging {
		sel = c.dragTime
	}
	if !sel.Before(r.first) && !sel.After(r.last) {
		cx, cy := xOf(sel), trackY+trackH/2
		handle := image.Rect(cx-handleR, cy-handleR, cx+handleR, cy+handleR)
		paint.FillShape(gtx.Ops, st.Theme.ContrastBg, clip.Ellipse(handle).Op(gtx.Ops))
	}
	return layout.Dimensions{Size: image.Pt(w, h)}
}

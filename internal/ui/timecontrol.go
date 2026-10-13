package ui

import (
	"time"

	"gioui.org/layout"
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

// timeBounds returns the first and the last valid time of all enabled overlays with times.
func (a *App) timeBounds() (first, last time.Time, ok bool) {
	for i := range a.overlays {
		o := &a.overlays[i]
		if o.tiles == nil || o.times.dim == nil {
			continue
		}
		f, l, valid := o.times.dim.Bounds()
		if !valid {
			continue
		}
		if !ok || f.Before(first) {
			first = f
		}
		if !ok || l.After(last) {
			last = l
		}
		ok = true
	}
	return first, last, ok
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

// layoutTimeControl draws the time selection for the legend box, if an enabled overlay has times:
// buttons for an hour earlier and later, the selected time, and a button back to the current time.
func (a *App) layoutTimeControl(gtx layout.Context) layout.Dimensions {
	first, last, ok := a.timeBounds()
	if !ok {
		return layout.Dimensions{}
	}
	c, st, now := &a.timeCtl, a.style, gtx.Now
	prev, next := c.step(now, -1), c.step(now, 1)
	canPrev, canNext := !prev.Before(first), !next.After(last)
	if c.prev.Clicked(gtx) && canPrev {
		a.selectAllTimes(prev, now)
	}
	if c.next.Clicked(gtx) && canNext {
		a.selectAllTimes(next, now)
	}
	if c.now.Clicked(gtx) {
		a.selectAllTimes(time.Time{}, now)
	}

	label := "Now"
	if !c.selected.IsZero() {
		label = formatDataTime(c.selected, now)
	}
	button := func(b *iconButton, ic *widget.Icon, tip string, enabled bool) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			col := st.Theme.Fg
			if !enabled {
				col = st.HintFg
			}
			return b.LayoutFlat(gtx, st, ic, col, tip)
		})
	}
	children := []layout.FlexChild{
		button(&c.prev, iconPrev, "One hour earlier", canPrev),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// A fixed width, so that the buttons don't move while stepping.
			gtx.Constraints.Min.X = gtx.Sp(st.SmallTextSize) * 5
			return layout.Center.Layout(gtx, st.SmallLabel(label).Layout)
		}),
		button(&c.next, iconNext, "One hour later", canNext),
	}
	if !c.selected.IsZero() {
		children = append(children, button(&c.now, iconNow, "Back to the current time", true))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

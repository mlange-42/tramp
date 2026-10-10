package ui

import (
	"image"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
)

const (
	// tooltipDelay is how long the pointer must rest on a widget before its tooltip shows.
	tooltipDelay = 600 * time.Millisecond
	// tooltipWidth is the largest width of a tooltip.
	tooltipWidth unit.Dp = 340
)

// tooltip shows a text below a widget while the pointer rests on it.
type tooltip struct {
	hovered bool
	since   time.Time
}

// Layout draws the tooltip for a widget of the given size, if it has been hovered long enough.
// avail is the width from the left edge of the widget to the right edge of the window,
// for keeping the tooltip inside. It is drawn on top of everything else.
func (t *tooltip) Layout(gtx layout.Context, st *Style, hovered bool, size image.Point, avail int, text string) {
	if !hovered {
		t.hovered = false
		return
	}
	if !t.hovered {
		t.hovered, t.since = true, gtx.Now
	}
	if show := t.since.Add(tooltipDelay); gtx.Now.Before(show) {
		gtx.Execute(op.InvalidateCmd{At: show})
		return
	}

	gtx.Constraints = layout.Constraints{Max: image.Pt(min(gtx.Dp(tooltipWidth), max(avail, size.X)), gtx.Constraints.Max.Y)}
	macro := op.Record(gtx.Ops)
	dims := layoutPanel(gtx, st, t, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(st.Spacing/2).Layout(gtx, st.SmallLabel(text).Layout)
	})
	call := macro.Stop()

	x := min(0, avail-dims.Size.X)
	macro = op.Record(gtx.Ops)
	stack := op.Offset(image.Pt(x, size.Y+gtx.Dp(2))).Push(gtx.Ops)
	call.Add(gtx.Ops)
	stack.Pop()
	op.Defer(gtx.Ops, macro.Stop())
}

// tooltipButton is a button with a tooltip.
type tooltipButton struct {
	btn widget.Clickable
	tip tooltip
}

// Layout draws the button, disabled if not enabled, and its tooltip.
func (b *tooltipButton) Layout(gtx layout.Context, st *Style, label, tip string, enabled bool) layout.Dimensions {
	avail := gtx.Constraints.Max.X
	bgtx := gtx
	if !enabled {
		bgtx = gtx.Disabled()
	}
	dims := st.Button(&b.btn, label).Layout(bgtx)
	b.tip.Layout(gtx, st, b.btn.Hovered(), dims.Size, avail, tip)
	return dims
}

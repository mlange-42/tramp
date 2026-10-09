package ui

import (
	"image"

	"gioui.org/f32"
	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// Split shows a panel next to the main content, with a draggable divider to resize it.
// The panel is to the left of the main content, or above it for a vertical split.
// With End, it is to the right or below instead.
type Split struct {
	// Axis is the direction in which panel and main content are arranged.
	Axis layout.Axis
	// End places the panel after the main content.
	End bool
	// Size of the panel along the axis. It is clamped to leave room for both sides.
	Size unit.Dp
	// MinSize is the minimum size of the panel. If zero, [Style.MinPaneSize] is used.
	MinSize unit.Dp
	// Collapsible lets the panel be closed by dragging the divider below 2/3 of the minimum size.
	// The divider stays visible, for dragging the panel open again.
	Collapsible bool
	// Collapsed is whether the panel is closed. Size is kept for the next time it is open.
	Collapsed bool

	drag gesture.Drag
	// grab is the distance of the pointer from the divider position when the drag started.
	grab float32
	// px is the panel size in pixels at the last layout, n the total size along the axis.
	px, n int
}

// pos returns the position of the divider along the axis, in pixels.
func (s *Split) pos(bar int) int {
	if s.End {
		return s.n - s.px - bar
	}
	return s.px
}

// update applies divider drags.
func (s *Split) update(gtx layout.Context, st *Style) {
	axis := gesture.Horizontal
	if s.Axis == layout.Vertical {
		axis = gesture.Vertical
	}
	bar := gtx.Dp(st.DividerWidth)
	var moved bool
	var p float32
	for {
		e, ok := s.drag.Update(gtx.Metric, gtx.Source, axis)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			s.grab = s.along(e.Position) - float32(s.pos(bar))
		case pointer.Drag:
			moved, p = true, s.along(e.Position)-s.grab
		}
	}
	if moved {
		if s.End {
			p = float32(s.n-bar) - p
		}
		size := unit.Dp(p / gtx.Metric.PxPerDp)
		// Well below the minimum size, a collapsible panel snaps closed.
		collapsed := s.Collapsible && size < s.minSize(st)*2/3
		if collapsed != s.Collapsed {
			s.Collapsed = collapsed
			// Others may depend on the panel being open, and only notice in the next frame.
			gtx.Execute(op.InvalidateCmd{})
		}
		if !s.Collapsed {
			s.Size = max(s.minSize(st), size)
		}
	}
}

// minSize returns the minimum size of the panel.
func (s *Split) minSize(st *Style) unit.Dp {
	if s.MinSize > 0 {
		return s.MinSize
	}
	return st.MinPaneSize
}

// along returns the coordinate of p along the axis.
func (s *Split) along(p f32.Point) float32 {
	if s.Axis == layout.Vertical {
		return p.Y
	}
	return p.X
}

// Layout draws the panel and the main content, filling the maximum constraints.
func (s *Split) Layout(gtx layout.Context, st *Style, panel, main layout.Widget) layout.Dimensions {
	s.update(gtx, st)

	size := gtx.Constraints.Max
	// Sizes in (along axis, across axis) coordinates.
	n, cross := s.Axis.Convert(size).X, s.Axis.Convert(size).Y
	s.n = n
	bar := gtx.Dp(st.DividerWidth)
	// Keep the minimum size for the main content first, then for the panel.
	s.px = min(max(min(gtx.Dp(s.Size), n-bar-gtx.Dp(st.MinPaneSize)), gtx.Dp(s.minSize(st))), max(0, n-bar))
	if s.Collapsed {
		s.px = 0
	}

	// rect converts a range along the axis to a rectangle spanning the cross axis.
	rect := func(from, to int) image.Rectangle {
		return image.Rectangle{Min: s.Axis.Convert(image.Pt(from, 0)), Max: s.Axis.Convert(image.Pt(to, cross))}
	}
	d := s.pos(bar)
	if s.End {
		s.layoutPane(gtx, rect(0, d), main)
		s.layoutPane(gtx, rect(d+bar, n), panel)
	} else {
		s.layoutPane(gtx, rect(0, d), panel)
		s.layoutPane(gtx, rect(d+bar, n), main)
	}
	// The handle of a closed panel at the edge would be half outside, so keep it inside.
	grip := gtx.Dp(st.DividerGrip)
	handleAt := d
	if s.Collapsed {
		if s.End {
			handleAt = min(d, n-bar/2-grip+grip/2)
		} else {
			handleAt = max(d, grip/2-bar/2)
		}
	}
	paint.FillShape(gtx.Ops, st.DividerColor, clip.Rect(rect(d, d+bar)).Op())

	// The handle is wider than the divider for easier grabbing, and on top of both panes.
	// It is not offset, so that pointer positions don't depend on the moving divider.
	defer clip.Rect(rect(handleAt+bar/2-grip/2, handleAt+bar/2+grip-grip/2)).Push(gtx.Ops).Pop()
	s.drag.Add(gtx.Ops)
	if s.Axis == layout.Vertical {
		pointer.CursorRowResize.Add(gtx.Ops)
	} else {
		pointer.CursorColResize.Add(gtx.Ops)
	}

	return layout.Dimensions{Size: size}
}

// layoutPane draws w into the given rectangle, clipped. Empty panes are not drawn.
func (s *Split) layoutPane(gtx layout.Context, r image.Rectangle, w layout.Widget) {
	if r.Empty() {
		return
	}
	defer op.Offset(r.Min).Push(gtx.Ops).Pop()
	defer clip.Rect{Max: r.Size()}.Push(gtx.Ops).Pop()
	gtx.Constraints = layout.Exact(r.Size())
	w(gtx)
}

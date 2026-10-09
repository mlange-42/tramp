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
		s.Size = max(st.MinPaneSize, unit.Dp(p/gtx.Metric.PxPerDp))
	}
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
	minPx := gtx.Dp(st.MinPaneSize)
	// Keep the minimum size for the main content first, then for the panel.
	s.px = min(max(min(gtx.Dp(s.Size), n-bar-minPx), minPx), max(0, n-bar))

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
	paint.FillShape(gtx.Ops, st.DividerColor, clip.Rect(rect(d, d+bar)).Op())

	// The handle is wider than the divider for easier grabbing, and on top of both panes.
	// It is not offset, so that pointer positions don't depend on the moving divider.
	grip := gtx.Dp(st.DividerGrip)
	defer clip.Rect(rect(d+bar/2-grip/2, d+bar/2+grip-grip/2)).Push(gtx.Ops).Pop()
	s.drag.Add(gtx.Ops)
	if s.Axis == layout.Vertical {
		pointer.CursorRowResize.Add(gtx.Ops)
	} else {
		pointer.CursorColResize.Add(gtx.Ops)
	}

	return layout.Dimensions{Size: size}
}

// layoutPane draws w into the given rectangle, clipped.
func (s *Split) layoutPane(gtx layout.Context, r image.Rectangle, w layout.Widget) {
	defer op.Offset(r.Min).Push(gtx.Ops).Pop()
	defer clip.Rect{Max: r.Size()}.Push(gtx.Ops).Pop()
	gtx.Constraints = layout.Exact(r.Size())
	w(gtx)
}

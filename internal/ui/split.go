package ui

import (
	"image"

	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// Split shows a side panel to the left of the main content, with a draggable divider to resize it.
type Split struct {
	// Width of the side panel. It is clamped to leave room for both sides.
	Width unit.Dp

	drag gesture.Drag
	// grabX is the distance of the pointer from the panel edge when the drag started.
	grabX float32
	// px is the panel width in pixels at the last layout.
	px int
}

// update applies divider drags.
func (s *Split) update(gtx layout.Context, st *Style) {
	var moved bool
	var x float32
	for {
		e, ok := s.drag.Update(gtx.Metric, gtx.Source, gesture.Horizontal)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			s.grabX = e.Position.X - float32(s.px)
		case pointer.Drag:
			moved, x = true, e.Position.X
		}
	}
	if moved {
		s.Width = max(st.MinPaneWidth, unit.Dp((x-s.grabX)/gtx.Metric.PxPerDp))
	}
}

// Layout draws the side panel and the main content, filling the maximum constraints.
func (s *Split) Layout(gtx layout.Context, st *Style, panel, main layout.Widget) layout.Dimensions {
	s.update(gtx, st)

	size := gtx.Constraints.Max
	bar := gtx.Dp(st.DividerWidth)
	minPx := gtx.Dp(st.MinPaneWidth)
	// Keep the minimum width for the main content first, then for the panel.
	s.px = min(max(min(gtx.Dp(s.Width), size.X-bar-minPx), minPx), max(0, size.X-bar))

	s.layoutPane(gtx, image.Rect(0, 0, s.px, size.Y), panel)
	paint.FillShape(gtx.Ops, st.DividerColor, clip.Rect(image.Rect(s.px, 0, s.px+bar, size.Y)).Op())
	s.layoutPane(gtx, image.Rect(s.px+bar, 0, size.X, size.Y), main)

	// The handle is wider than the divider for easier grabbing, and on top of both panes.
	// It is not offset, so that pointer positions don't depend on the moving divider.
	grip := gtx.Dp(st.DividerGrip)
	handle := image.Rect(s.px+bar/2-grip/2, 0, s.px+bar/2+grip-grip/2, size.Y)
	defer clip.Rect(handle).Push(gtx.Ops).Pop()
	s.drag.Add(gtx.Ops)
	pointer.CursorColResize.Add(gtx.Ops)

	return layout.Dimensions{Size: size}
}

// layoutPane draws w into the given rectangle, clipped.
func (s *Split) layoutPane(gtx layout.Context, r image.Rectangle, w layout.Widget) {
	defer op.Offset(r.Min).Push(gtx.Ops).Pop()
	defer clip.Rect{Max: r.Size()}.Push(gtx.Ops).Pop()
	gtx.Constraints = layout.Exact(r.Size())
	w(gtx)
}

package ui

import (
	"image"
	"image/color"
	"strconv"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// popup is a header button that opens a panel below it.
// The panel closes when clicking anywhere outside of it.
type popup struct {
	header widget.Clickable
	open   bool
	// panel is the event tag for the panel background.
	panel int
}

func (p *popup) update(gtx layout.Context) {
	if p.header.Clicked(gtx) {
		p.open = !p.open
	}
	for {
		_, ok := gtx.Event(pointer.Filter{Target: p, Kinds: pointer.Press})
		if !ok {
			break
		}
		p.open = false
	}
}

func (p *popup) layout(gtx layout.Context, st *Style, label string, panel layout.Widget) layout.Dimensions {
	dims := p.layoutHeader(gtx, st, label)
	if !p.open {
		return dims
	}

	macro := op.Record(gtx.Ops)
	// Catch clicks outside the panel, including on the header.
	scrim := clip.Rect{Min: image.Pt(-1e6, -1e6), Max: image.Pt(1e6, 1e6)}.Push(gtx.Ops)
	event.Op(gtx.Ops, p)
	scrim.Pop()

	op.Offset(image.Pt(0, dims.Size.Y)).Add(gtx.Ops)
	gtx.Constraints = layout.Constraints{
		Min: image.Pt(dims.Size.X, 0),
		Max: image.Pt(max(dims.Size.X, gtx.Dp(400)), gtx.Dp(600)),
	}
	layoutPanel(gtx, st, &p.panel, panel)
	op.Defer(gtx.Ops, macro.Stop())

	return dims
}

func (p *popup) layoutHeader(gtx layout.Context, st *Style, label string) layout.Dimensions {
	th := st.Theme
	btn := material.ButtonLayout(th, &p.header)
	btn.CornerRadius = st.CornerRadius
	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return st.ButtonInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := st.Label(label)
					l.Color = th.ContrastFg
					return l.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: st.Spacing}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutArrow(gtx, float32(gtx.Sp(st.TextSize))*0.6, th.ContrastFg, p.open)
				}),
			)
		})
	})
}

// layoutArrow draws a small triangle of the given width, pointing down, or up if open.
func layoutArrow(gtx layout.Context, w float32, col color.NRGBA, up bool) layout.Dimensions {
	h := w / 2
	var path clip.Path
	path.Begin(gtx.Ops)
	if up {
		path.MoveTo(f32.Pt(0, h))
		path.LineTo(f32.Pt(w, h))
		path.LineTo(f32.Pt(w/2, 0))
	} else {
		path.MoveTo(f32.Pt(0, 0))
		path.LineTo(f32.Pt(w, 0))
		path.LineTo(f32.Pt(w/2, h))
	}
	path.Close()
	paint.FillShape(gtx.Ops, col, clip.Outline{Path: path.End()}.Op())
	return layout.Dimensions{Size: image.Pt(int(w), int(h))}
}

// layoutPanel draws content on a bordered background.
// The background blocks pointer events for widgets below, using tag.
func layoutPanel(gtx layout.Context, st *Style, tag event.Tag, content layout.Widget) layout.Dimensions {
	border := gtx.Dp(1)
	macro := op.Record(gtx.Ops)
	dims := layout.UniformInset(st.PanelInset).Layout(gtx, content)
	call := macro.Stop()

	outer := image.Rectangle{Max: dims.Size}
	paint.FillShape(gtx.Ops, st.PanelBorder, clip.Rect(outer).Op())
	paint.FillShape(gtx.Ops, st.PanelBg, clip.Rect(outer.Inset(border)).Op())

	area := clip.Rect(outer).Push(gtx.Ops)
	event.Op(gtx.Ops, tag)
	area.Pop()

	call.Add(gtx.Ops)
	return dims
}

// Select is a drop-down list for choosing one of several options.
type Select struct {
	popup
	enum widget.Enum
}

// Selected returns the index of the selected option.
func (s *Select) Selected() int {
	i, _ := strconv.Atoi(s.enum.Value)
	return i
}

// SetSelected sets the selected option.
func (s *Select) SetSelected(i int) {
	s.enum.Value = strconv.Itoa(i)
}

// Update processes input and reports whether the selection changed.
func (s *Select) Update(gtx layout.Context) bool {
	s.update(gtx)
	if s.enum.Update(gtx) {
		s.open = false
		return true
	}
	return false
}

// Layout draws the drop-down, with the selected option as its label.
func (s *Select) Layout(gtx layout.Context, st *Style, options []string) layout.Dimensions {
	return s.layout(gtx, st, options[s.Selected()], func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, len(options))
		for i, opt := range options {
			rb := material.RadioButton(st.Theme, &s.enum, strconv.Itoa(i), opt)
			rb.Size = st.IconSize
			rb.TextSize = st.TextSize
			children[i] = layout.Rigid(rb.Layout)
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

// MultiSelect is a drop-down list with a checkbox per option.
type MultiSelect struct {
	popup
	checks []widget.Bool
}

// NewMultiSelect creates a multi-select for n options.
func NewMultiSelect(n int) *MultiSelect {
	return &MultiSelect{checks: make([]widget.Bool, n)}
}

// Checked reports whether option i is checked.
func (m *MultiSelect) Checked(i int) bool {
	return m.checks[i].Value
}

// Update processes input and returns the indices of options that changed.
func (m *MultiSelect) Update(gtx layout.Context) []int {
	m.update(gtx)
	var changed []int
	for i := range m.checks {
		if m.checks[i].Update(gtx) {
			changed = append(changed, i)
		}
	}
	return changed
}

// Layout draws the drop-down with the given header label.
func (m *MultiSelect) Layout(gtx layout.Context, st *Style, label string, options []string) layout.Dimensions {
	return m.layout(gtx, st, label, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, len(options))
		for i, opt := range options {
			cb := material.CheckBox(st.Theme, &m.checks[i], opt)
			cb.Size = st.IconSize
			cb.TextSize = st.TextSize
			children[i] = layout.Rigid(cb.Layout)
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

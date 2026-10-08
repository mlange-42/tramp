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
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

var (
	popupBackground = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	popupBorder     = color.NRGBA{R: 0xa0, G: 0xa0, B: 0xa0, A: 0xff}
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

func (p *popup) layout(gtx layout.Context, th *material.Theme, label string, panel layout.Widget) layout.Dimensions {
	dims := p.layoutHeader(gtx, th, label)
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
	layoutPanel(gtx, &p.panel, panel)
	op.Defer(gtx.Ops, macro.Stop())

	return dims
}

func (p *popup) layoutHeader(gtx layout.Context, th *material.Theme, label string) layout.Dimensions {
	btn := material.ButtonLayout(th, &p.header)
	btn.CornerRadius = unit.Dp(4)
	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 6, Bottom: 6, Left: 10, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := material.Body2(th, label)
					l.Color = th.ContrastFg
					return l.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: 6}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layoutArrow(gtx, th.ContrastFg, p.open)
				}),
			)
		})
	})
}

// layoutArrow draws a small triangle pointing down, or up if open.
func layoutArrow(gtx layout.Context, col color.NRGBA, up bool) layout.Dimensions {
	w := float32(gtx.Dp(10))
	h := float32(gtx.Dp(5))
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
func layoutPanel(gtx layout.Context, tag event.Tag, content layout.Widget) layout.Dimensions {
	border := gtx.Dp(1)
	macro := op.Record(gtx.Ops)
	dims := layout.UniformInset(unit.Dp(4)).Layout(gtx, content)
	call := macro.Stop()

	outer := image.Rectangle{Max: dims.Size}
	paint.FillShape(gtx.Ops, popupBorder, clip.Rect(outer).Op())
	paint.FillShape(gtx.Ops, popupBackground, clip.Rect(outer.Inset(border)).Op())

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
func (s *Select) Layout(gtx layout.Context, th *material.Theme, options []string) layout.Dimensions {
	return s.layout(gtx, th, options[s.Selected()], func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, len(options))
		for i, opt := range options {
			children[i] = layout.Rigid(material.RadioButton(th, &s.enum, strconv.Itoa(i), opt).Layout)
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
func (m *MultiSelect) Layout(gtx layout.Context, th *material.Theme, label string, options []string) layout.Dimensions {
	return m.layout(gtx, th, label, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, len(options))
		for i, opt := range options {
			children[i] = layout.Rigid(material.CheckBox(th, &m.checks[i], opt).Layout)
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

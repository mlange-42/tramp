package ui

import (
	"fmt"
	"image"
	"image/color"
	"strconv"

	"gioui.org/f32"
	"gioui.org/font"
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
	// help is the tooltip of the header, if not empty, and icon an icon before the label, if not nil.
	help string
	tip  tooltip
	icon *widget.Icon
	// sections are headings in the list of options, in order of their index.
	sections []Section
	// tips are the tooltips of the options, if not empty, with their state in optTips.
	tips    []string
	optTips []tooltip
	// avail is the width from the header to the right edge of the window, for the option tooltips.
	avail int
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

// layout draws the header with the given label, and the panel if open.
// The header is as wide as the widest of the labels in sizes, so that it doesn't change size.
func (p *popup) layout(gtx layout.Context, st *Style, label string, sizes []string, panel layout.Widget) layout.Dimensions {
	p.avail = gtx.Constraints.Max.X
	dims := p.layoutHeader(gtx, st, label, maxLabelWidth(gtx, st, sizes))
	if p.help != "" {
		p.tip.Layout(gtx, st, p.header.Hovered() && !p.open, dims.Size, p.avail, p.help)
	}
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

func (p *popup) layoutHeader(gtx layout.Context, st *Style, label string, labelWidth int) layout.Dimensions {
	th := st.Theme
	btn := material.ButtonLayout(th, &p.header)
	btn.CornerRadius = st.CornerRadius
	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return st.ButtonInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.icon == nil {
						return layout.Dimensions{}
					}
					size := gtx.Sp(st.TextSize) * 5 / 4
					layoutIcon(gtx, p.icon, image.Point{}, size, th.ContrastFg)
					return layout.Dimensions{Size: image.Pt(size+gtx.Dp(st.Spacing/2), size)}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = labelWidth
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

// maxLabelWidth returns the width of the widest label, without drawing anything.
func maxLabelWidth(gtx layout.Context, st *Style, labels []string) int {
	macro := op.Record(gtx.Ops)
	defer macro.Stop()

	gtx.Constraints = layout.Constraints{Max: gtx.Constraints.Max}
	w := 0
	for _, l := range labels {
		w = max(w, st.Label(l).Layout(gtx).Size.X)
	}
	return w
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

// layoutOptions draws n options below each other, with the section headings before them.
// hovered reports whether option i is hovered, for showing its tooltip.
func (p *popup) layoutOptions(gtx layout.Context, st *Style, n int, option func(i int) layout.Widget, hovered func(i int) bool) layout.Dimensions {
	if len(p.optTips) != n {
		p.optTips = make([]tooltip, n)
	}
	children := make([]layout.FlexChild, 0, n+len(p.sections))
	s := 0
	for i := range n {
		for ; s < len(p.sections) && p.sections[s].Index <= i; s++ {
			children = append(children, layout.Rigid(sectionHeading(st, p.sections[s].Name, len(children) == 0)))
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := option(i)(gtx)
			if i < len(p.tips) && p.tips[i] != "" {
				p.optTips[i].Layout(gtx, st, hovered(i), dims.Size, p.avail, p.tips[i])
			}
			return dims
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// sectionHeading returns a heading for a section in a drop-down list, with space above unless first.
func sectionHeading(st *Style, name string, first bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		inset := layout.Inset{Bottom: st.Spacing / 2}
		if !first {
			inset.Top = st.Spacing
		}
		return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			l := st.Label(name)
			l.Color = st.HintFg
			l.TextSize = st.SmallTextSize
			l.Font.Weight = font.Bold
			return l.Layout(gtx)
		})
	}
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
	return s.layout(gtx, st, options[s.Selected()], options, func(gtx layout.Context) layout.Dimensions {
		return s.layoutOptions(gtx, st, len(options), func(i int) layout.Widget {
			rb := material.RadioButton(st.Theme, &s.enum, strconv.Itoa(i), options[i])
			rb.Size = st.IconSize
			rb.TextSize = st.TextSize
			return rb.Layout
		}, func(i int) bool {
			k, ok := s.enum.Hovered()
			return ok && k == strconv.Itoa(i)
		})
	})
}

// MultiSelect is a drop-down list with a checkbox per option.
// Its header shows the title and the number of checked options.
type MultiSelect struct {
	popup
	title  string
	checks []widget.Bool
}

// NewMultiSelect creates a multi-select with the given title for n options.
func NewMultiSelect(title string, n int) *MultiSelect {
	return &MultiSelect{title: title, checks: make([]widget.Bool, n)}
}

// Checked reports whether option i is checked.
func (m *MultiSelect) Checked(i int) bool {
	return m.checks[i].Value
}

// SetChecked sets whether option i is checked.
func (m *MultiSelect) SetChecked(i int, checked bool) {
	m.checks[i].Value = checked
}

// label returns the header label for n checked options.
func (m *MultiSelect) label(n int) string {
	if n == 0 {
		return m.title
	}
	return fmt.Sprintf("%s (%d)", m.title, n)
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

// Layout draws the drop-down.
func (m *MultiSelect) Layout(gtx layout.Context, st *Style, options []string) layout.Dimensions {
	checked := 0
	for i := range m.checks {
		if m.checks[i].Value {
			checked++
		}
	}
	// The label is widest with all options checked.
	sizes := []string{m.label(0), m.label(len(m.checks))}
	return m.layout(gtx, st, m.label(checked), sizes, func(gtx layout.Context) layout.Dimensions {
		return m.layoutOptions(gtx, st, len(options), func(i int) layout.Widget {
			cb := material.CheckBox(st.Theme, &m.checks[i], options[i])
			cb.Size = st.IconSize
			cb.TextSize = st.TextSize
			return cb.Layout
		}, func(i int) bool { return m.checks[i].Hovered() })
	})
}

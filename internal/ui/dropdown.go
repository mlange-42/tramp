package ui

import (
	"fmt"
	"image"
	"image/color"
	"strconv"

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
	// lead is drawn before the label in place of icon, if not nil.
	lead layout.Widget
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
// The header is as wide as its content, so it changes its width with the label.
func (p *popup) layout(gtx layout.Context, st *Style, label string, panel layout.Widget) layout.Dimensions {
	p.avail = gtx.Constraints.Max.X
	dims := p.layoutHeader(gtx, st, label)
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

// layoutHeader draws the header flat like the toolbar buttons: highlighted while hovered or open,
// and as high as them. The arrow follows the label.
func (p *popup) layoutHeader(gtx layout.Context, st *Style, label string) layout.Dimensions {
	return p.header.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		col := st.Theme.Fg
		macro := op.Record(gtx.Ops)
		gtx.Constraints.Min = image.Point{}
		content := layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if p.lead != nil {
					return p.lead(gtx)
				}
				if p.icon == nil {
					return layout.Dimensions{}
				}
				size := gtx.Sp(st.TextSize) * 5 / 4
				layoutIcon(gtx, p.icon, image.Point{}, size, col)
				return layout.Dimensions{Size: image.Pt(size+gtx.Dp(st.Spacing/2), size)}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := st.Label(label)
				l.Color = col
				return l.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutChevron(gtx, st, col, p.open)
			}),
		)
		call := macro.Stop()

		pad := gtx.Dp(st.Spacing)
		size := image.Pt(content.Size.X+2*pad, max(gtx.Dp(st.ToolIconSize+2*st.ToolIconPad), content.Size.Y))
		layoutButtonBg(gtx, st, size, p.header.Hovered(), p.open)
		defer op.Offset(image.Pt(pad, (size.Y-content.Size.Y)/2)).Push(gtx.Ops).Pop()
		call.Add(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		return layout.Dimensions{Size: size}
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

// layoutChevron draws a chevron after a drop-down label, pointing down, or up if open.
func layoutChevron(gtx layout.Context, st *Style, col color.NRGBA, open bool) layout.Dimensions {
	ic := iconExpand
	if open {
		ic = iconCollapse
	}
	size := gtx.Sp(st.TextSize) * 5 / 4
	layoutIcon(gtx, ic, image.Pt(gtx.Dp(2), 0), size, col)
	return layout.Dimensions{Size: image.Pt(size+gtx.Dp(2), size)}
}

// layoutButtonBg draws the background of a flat button of the given size:
// none at rest, a light tint while hovered and a stronger one while active.
func layoutButtonBg(gtx layout.Context, st *Style, size image.Point, hovered, active bool) {
	var bg color.NRGBA
	switch {
	case active:
		bg = st.SelectedBg
	case hovered:
		bg = st.DragBg
	default:
		return
	}
	paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(st.CornerRadius)).Op(gtx.Ops))
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
	// icons are shown between the radio button and the label of each option, if not nil.
	icons []*widget.Icon
	// preview draws a preview of option i in place of an icon, in the options and the header, if not nil.
	preview func(gtx layout.Context, i int) layout.Dimensions
	// more is drawn below the options in the panel, if not nil.
	more layout.Widget
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
	if s.preview != nil {
		s.lead = func(gtx layout.Context) layout.Dimensions { return s.preview(gtx, s.Selected()) }
	}
	return s.layout(gtx, st, options[s.Selected()], func(gtx layout.Context) layout.Dimensions {
		if s.more == nil {
			return s.layoutList(gtx, st, options)
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.layoutList(gtx, st, options) }),
			layout.Rigid(s.more),
		)
	})
}

// layoutList draws the options, for the panel of this drop-down or of another one.
func (s *Select) layoutList(gtx layout.Context, st *Style, options []string) layout.Dimensions {
	return s.layoutOptions(gtx, st, len(options), func(i int) layout.Widget {
		if s.icons != nil || s.preview != nil {
			return func(gtx layout.Context) layout.Dimensions {
				return s.layoutIconOption(gtx, st, i, options[i])
			}
		}
		rb := material.RadioButton(st.Theme, &s.enum, strconv.Itoa(i), options[i])
		rb.Size = st.IconSize
		rb.TextSize = st.TextSize
		return rb.Layout
	}, func(i int) bool {
		k, ok := s.enum.Hovered()
		return ok && k == strconv.Itoa(i)
	})
}

// layoutIconOption draws option i like a material radio button, with its icon or preview between the button and the label.
func (s *Select) layoutIconOption(gtx layout.Context, st *Style, i int, label string) layout.Dimensions {
	th := st.Theme
	key := strconv.Itoa(i)
	hk, hovered := s.enum.Hovered()
	hovered = hovered && hk == key
	return s.enum.Layout(gtx, key, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Dp(st.IconSize)
		pad := gtx.Dp(2)
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				box := size * 4 / 3
				if hovered {
					bg := th.ContrastBg
					bg.A = 70
					paint.FillShape(gtx.Ops, bg, clip.Ellipse{Max: image.Pt(box, box)}.Op(gtx.Ops))
				}
				ic := th.Icon.RadioUnchecked
				if s.enum.Value == key {
					ic = th.Icon.RadioChecked
				}
				layoutIcon(gtx, ic, image.Pt((box-size)/2, (box-size)/2), size, th.ContrastBg)
				return layout.Dimensions{Size: image.Pt(box, box)}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if s.preview != nil {
					return layout.Inset{Left: 2, Right: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.preview(gtx, i)
					})
				}
				ic := s.icons[i]
				is := gtx.Sp(st.TextSize) * 5 / 4
				if ic != nil {
					layoutIcon(gtx, ic, image.Pt(pad, 0), is, th.Fg)
				}
				return layout.Dimensions{Size: image.Pt(is+2*pad, is)}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(2).Layout(gtx, st.Label(label).Layout)
			}),
		)
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
	return m.layout(gtx, st, m.label(checked), func(gtx layout.Context) layout.Dimensions {
		return m.layoutOptions(gtx, st, len(options), func(i int) layout.Widget {
			cb := material.CheckBox(st.Theme, &m.checks[i], options[i])
			cb.Size = st.IconSize
			cb.TextSize = st.TextSize
			return cb.Layout
		}, func(i int) bool { return m.checks[i].Hovered() })
	})
}

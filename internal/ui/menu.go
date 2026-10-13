package ui

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

// menuItem is a command in a menu, or a separator if label is empty.
type menuItem struct {
	label string
	// shortcut is the keyboard shortcut shown after the label, like "Ctrl+O". May be empty.
	shortcut string
	disabled bool
	action   func()
}

// menuSeparator is a line between groups of items.
var menuSeparator = menuItem{}

// menu is a header in the menu bar that opens a list of commands below it.
// The list closes when a command is chosen, or when clicking anywhere outside of it.
type menu struct {
	title  string
	header widget.Clickable
	open   bool
	// panel is the event tag for the list's background. The menu itself is the tag of the area around it.
	panel int
	// btns are the clickables of the items, in item order.
	btns []widget.Clickable
}

// menuItemGap is the space between the label and the shortcut of an item.
const menuItemGap unit.Dp = 32

// menuBar is the row of menus at the left of the toolbar.
type menuBar struct {
	menus []*menu
}

func newMenuBar(titles ...string) menuBar {
	var b menuBar
	for _, t := range titles {
		b.menus = append(b.menus, &menu{title: t})
	}
	return b
}

// update handles clicks on the headers and items, and outside open lists.
// items returns the items of each menu, in the order of the titles.
func (b *menuBar) update(gtx layout.Context, items [][]menuItem) {
	for i, m := range b.menus {
		if m.header.Clicked(gtx) {
			m.open = !m.open
			for _, o := range b.menus {
				if o != m {
					o.open = false
				}
			}
		}
		for {
			if _, ok := gtx.Event(pointer.Filter{Target: m, Kinds: pointer.Press}); !ok {
				break
			}
			m.open = false
		}
		// The list takes presses, so that they don't reach the widgets below.
		for {
			if _, ok := gtx.Event(pointer.Filter{Target: &m.panel, Kinds: pointer.Press}); !ok {
				break
			}
		}
		for j := range m.btns {
			if !m.btns[j].Clicked(gtx) || j >= len(items[i]) {
				continue
			}
			it := items[i][j]
			if it.label != "" && !it.disabled && it.action != nil {
				m.open = false
				it.action()
			}
		}
	}
}

// layout draws the headers, and the list of the open menu.
func (b *menuBar) layout(gtx layout.Context, st *Style, items [][]menuItem) layout.Dimensions {
	children := make([]layout.FlexChild, len(b.menus))
	for i, m := range b.menus {
		children[i] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return m.layout(gtx, st, items[i])
		})
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

func (m *menu) layout(gtx layout.Context, st *Style, items []menuItem) layout.Dimensions {
	if len(m.btns) != len(items) {
		m.btns = make([]widget.Clickable, len(items))
	}
	dims := m.layoutHeader(gtx, st)
	if !m.open {
		return dims
	}

	macro := op.Record(gtx.Ops)
	// Catch clicks outside the list, including on the header.
	scrim := clip.Rect{Min: image.Pt(-1e6, -1e6), Max: image.Pt(1e6, 1e6)}.Push(gtx.Ops)
	event.Op(gtx.Ops, m)
	scrim.Pop()

	op.Offset(image.Pt(0, dims.Size.Y)).Add(gtx.Ops)
	lgtx := gtx
	lgtx.Constraints = layout.Constraints{Max: image.Pt(gtx.Dp(600), gtx.Dp(800))}
	layoutPanel(lgtx, st, &m.panel, func(gtx layout.Context) layout.Dimensions {
		return m.layoutItems(gtx, st, items)
	})
	op.Defer(gtx.Ops, macro.Stop())
	return dims
}

// layoutHeader draws the title, highlighted while hovered or open, as high as the toolbar buttons.
func (m *menu) layoutHeader(gtx layout.Context, st *Style) layout.Dimensions {
	return m.header.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		h := gtx.Dp(st.ToolIconSize + 2*st.ToolIconPad)
		macro := op.Record(gtx.Ops)
		gtx.Constraints.Min = image.Point{}
		l := st.Label(m.title)
		l.Color = st.Theme.Fg
		ld := l.Layout(gtx)
		label := macro.Stop()

		pad := gtx.Dp(st.Spacing)
		size := image.Pt(ld.Size.X+2*pad, max(h, ld.Size.Y))
		switch {
		case m.open:
			paint.FillShape(gtx.Ops, st.SelectedBg, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(st.CornerRadius)).Op(gtx.Ops))
		case m.header.Hovered():
			paint.FillShape(gtx.Ops, st.DragBg, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(st.CornerRadius)).Op(gtx.Ops))
		}
		off := op.Offset(image.Pt(pad, (size.Y-ld.Size.Y)/2)).Push(gtx.Ops)
		label.Add(gtx.Ops)
		off.Pop()
		pointer.CursorPointer.Add(gtx.Ops)
		return layout.Dimensions{Size: size}
	})
}

// layoutItems draws the items below each other, all as wide as the widest, with the shortcuts aligned.
func (m *menu) layoutItems(gtx layout.Context, st *Style, items []menuItem) layout.Dimensions {
	var labels, shortcuts []string
	for _, it := range items {
		labels = append(labels, it.label)
		shortcuts = append(shortcuts, it.shortcut)
	}
	pad := gtx.Dp(st.Spacing)
	labelW := maxLabelWidth(gtx, st, labels)
	shortcutW := maxLabelWidth(gtx, st, shortcuts)
	width := labelW + 2*pad
	if shortcutW > 0 {
		width += gtx.Dp(menuItemGap) + shortcutW
	}

	children := make([]layout.FlexChild, len(items))
	for i, it := range items {
		children[i] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if it.label == "" {
				return m.layoutSeparator(gtx, st, width)
			}
			return m.layoutItem(gtx, st, &m.btns[i], it, width, labelW)
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (m *menu) layoutItem(gtx layout.Context, st *Style, btn *widget.Clickable, it menuItem, width, labelW int) layout.Dimensions {
	if it.disabled {
		gtx = gtx.Disabled()
	}
	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		pad := gtx.Dp(st.Spacing)
		macro := op.Record(gtx.Ops)
		gtx.Constraints.Min = image.Point{}
		col := st.Theme.Fg
		if it.disabled {
			col = st.HintFg
		}
		l := st.Label(it.label)
		l.Color = col
		ld := l.Layout(gtx)
		if it.shortcut != "" {
			s := st.Label(it.shortcut)
			s.Color = st.HintFg
			off := op.Offset(image.Pt(labelW+gtx.Dp(menuItemGap), 0)).Push(gtx.Ops)
			s.Layout(gtx)
			off.Pop()
		}
		content := macro.Stop()

		size := image.Pt(width, ld.Size.Y+pad)
		if !it.disabled && btn.Hovered() {
			paint.FillShape(gtx.Ops, st.DragBg, clip.Rect{Max: size}.Op())
		}
		off := op.Offset(image.Pt(pad, pad/2)).Push(gtx.Ops)
		content.Add(gtx.Ops)
		off.Pop()
		if !it.disabled {
			pointer.CursorPointer.Add(gtx.Ops)
		}
		return layout.Dimensions{Size: size}
	})
}

func (m *menu) layoutSeparator(gtx layout.Context, st *Style, width int) layout.Dimensions {
	h := gtx.Dp(st.Spacing)
	line := image.Rect(0, h/2, width, h/2+max(1, gtx.Dp(1)))
	paint.FillShape(gtx.Ops, st.DividerColor, clip.Rect(line).Op())
	return layout.Dimensions{Size: image.Pt(width, h)}
}

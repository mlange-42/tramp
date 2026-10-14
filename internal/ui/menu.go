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
	// tip is the tooltip, shown while the item is hovered. May be empty.
	tip string
	// icon is shown before the label, if not nil, or the app logo if logo is set.
	icon     *widget.Icon
	logo     bool
	disabled bool
	// check makes the item a checkbox, shown checked if checked, in place of the icon.
	// Choosing it keeps the menu open, for switching several.
	check, checked bool
	action         func()
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
	// btns are the clickables of the items, in item order, and tips their tooltips.
	btns []widget.Clickable
	tips []tooltip
	// bounds is the area of the header in the menu bar, at the last layout.
	bounds image.Rectangle
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
	// A click on another header while a list is open is caught outside the list, and opens that menu.
	var next *menu
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
			ev, ok := gtx.Event(pointer.Filter{Target: m, Kinds: pointer.Press})
			if !ok {
				break
			}
			m.open = false
			if e, ok := ev.(pointer.Event); ok {
				// The position is relative to the header of m.
				if o := b.headerAt(m.bounds.Min.Add(e.Position.Round())); o != nil && o != m {
					next = o
				}
			}
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
				m.open = it.check
				it.action()
			}
		}
	}
	if next != nil {
		next.open = true
	}
}

// headerAt returns the menu with the header at a position in the menu bar, or nil.
func (b *menuBar) headerAt(p image.Point) *menu {
	for _, m := range b.menus {
		if p.In(m.bounds) {
			return m
		}
	}
	return nil
}

// layout draws the headers, and the list of the open menu.
func (b *menuBar) layout(gtx layout.Context, st *Style, items [][]menuItem) layout.Dimensions {
	children := make([]layout.FlexChild, len(b.menus))
	// The headers are laid out in order, all as high as the bar.
	x := 0
	for i, m := range b.menus {
		children[i] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := m.layout(gtx, st, items[i])
			m.bounds = image.Rectangle{Min: image.Pt(x, 0), Max: image.Pt(x+dims.Size.X, dims.Size.Y)}
			x += dims.Size.X
			return dims
		})
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

func (m *menu) layout(gtx layout.Context, st *Style, items []menuItem) layout.Dimensions {
	if len(m.btns) != len(items) {
		m.btns = make([]widget.Clickable, len(items))
		m.tips = make([]tooltip, len(items))
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
	// If any item has an icon, all labels leave room for one, so that they are aligned.
	iconW := 0
	for _, it := range items {
		if it.icon != nil || it.logo || it.check {
			iconW = menuIconSize(gtx, st) + pad
			break
		}
	}
	labelW += iconW
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
			dims := m.layoutItem(gtx, st, &m.btns[i], it, width, labelW, iconW)
			if it.tip != "" {
				m.tips[i].Layout(gtx, st, m.btns[i].Hovered(), dims.Size, gtx.Constraints.Max.X, it.tip)
			}
			return dims
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// menuIconSize returns the size of the icons of menu items.
func menuIconSize(gtx layout.Context, st *Style) int {
	return gtx.Sp(st.TextSize) * 5 / 4
}

// layoutItem draws an item with its label at iconW, after the icon if it has one, and the shortcut at labelW.
func (m *menu) layoutItem(gtx layout.Context, st *Style, btn *widget.Clickable, it menuItem, width, labelW, iconW int) layout.Dimensions {
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
		off := op.Offset(image.Pt(iconW, 0)).Push(gtx.Ops)
		ld := l.Layout(gtx)
		off.Pop()
		is := menuIconSize(gtx, st)
		switch {
		case it.check:
			ic, ccol := st.Theme.Icon.CheckBoxUnchecked, col
			if it.checked {
				ic = st.Theme.Icon.CheckBoxChecked
				if !it.disabled {
					ccol = st.Theme.ContrastBg
				}
			}
			layoutIcon(gtx, ic, image.Pt(0, (ld.Size.Y-is)/2), is, ccol)
		case it.icon != nil:
			layoutIcon(gtx, it.icon, image.Pt(0, (ld.Size.Y-is)/2), is, col)
		case it.logo:
			layoutLogo(gtx, image.Pt(0, (ld.Size.Y-is)/2), is)
		}
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
		off = op.Offset(image.Pt(pad, pad/2)).Push(gtx.Ops)
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

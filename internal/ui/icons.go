package ui

import (
	"image"
	"image/color"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

// Icons, from the Material Design icons.
var (
	iconNew      = mustIcon(icons.EditorInsertDriveFile)
	iconOpen     = mustIcon(icons.FileFolderOpen)
	iconUndo     = mustIcon(icons.ContentUndo)
	iconRedo     = mustIcon(icons.ContentRedo)
	iconEdit     = mustIcon(icons.EditorModeEdit)
	iconClose    = mustIcon(icons.NavigationClose)
	iconExpand   = mustIcon(icons.NavigationExpandMore)
	iconCollapse = mustIcon(icons.NavigationExpandLess)
	iconDelete   = mustIcon(icons.ActionDelete)
	iconLayers   = mustIcon(icons.MapsLayers)
	iconMap      = mustIcon(icons.MapsMap)
	iconPalette  = mustIcon(icons.ImagePalette)
	iconGradient = mustIcon(icons.ImageGradient)

	toolIcons = [numTools]*widget.Icon{
		mustIcon(icons.ActionTouchApp),
		mustIcon(icons.MapsAddLocation),
		mustIcon(icons.ActionTimeline),
	}
)

func mustIcon(data []byte) *widget.Icon {
	ic, err := widget.NewIcon(data)
	if err != nil {
		panic(err)
	}
	return ic
}

// iconButton is a button showing an icon, with a tooltip.
type iconButton struct {
	btn widget.Clickable
	tip tooltip
}

// Clicked reports whether the button was clicked.
func (b *iconButton) Clicked(gtx layout.Context) bool {
	return b.btn.Clicked(gtx)
}

// Layout draws the button for a toolbar, with a background that is highlighted if active.
// A disabled button is grayed out.
func (b *iconButton) Layout(gtx layout.Context, st *Style, ic *widget.Icon, tip string, enabled, active bool) layout.Dimensions {
	avail := gtx.Constraints.Max.X
	bgtx := gtx
	if !enabled {
		bgtx = gtx.Disabled()
	}
	dims := b.btn.Layout(bgtx, func(gtx layout.Context) layout.Dimensions {
		size, pad := gtx.Dp(st.ToolIconSize), gtx.Dp(st.ToolIconPad)
		box := image.Rectangle{Max: image.Pt(size+2*pad, size+2*pad)}
		bg := st.StatusBg
		switch {
		case active:
			bg = st.SelectedBg
		case enabled && b.btn.Hovered():
			bg = st.DragBg
		}
		paint.FillShape(gtx.Ops, bg, clip.UniformRRect(box, gtx.Dp(st.CornerRadius)).Op(gtx.Ops))
		col := st.Theme.Fg
		if !enabled {
			col = st.HintFg
		}
		layoutIcon(gtx, ic, image.Pt(pad, pad), size, col)
		if enabled && !active {
			pointer.CursorPointer.Add(gtx.Ops)
		}
		return layout.Dimensions{Size: box.Max}
	})
	b.tip.Layout(gtx, st, b.btn.Hovered(), dims.Size, avail, tip)
	return dims
}

// LayoutFlat draws the button without background, for the side panel.
func (b *iconButton) LayoutFlat(gtx layout.Context, st *Style, ic *widget.Icon, col color.NRGBA, tip string) layout.Dimensions {
	avail := gtx.Constraints.Max.X
	dims := b.btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size, pad := gtx.Dp(st.IconSize), gtx.Dp(st.Spacing/2)
		layoutIcon(gtx, ic, image.Pt(pad, pad), size, col)
		pointer.CursorPointer.Add(gtx.Ops)
		return layout.Dimensions{Size: image.Pt(size+2*pad, size+2*pad)}
	})
	b.tip.Layout(gtx, st, b.btn.Hovered(), dims.Size, avail, tip)
	return dims
}

// layoutIcon draws an icon of the given size at an offset.
func layoutIcon(gtx layout.Context, ic *widget.Icon, at image.Point, size int, col color.NRGBA) {
	defer op.Offset(at).Push(gtx.Ops).Pop()
	gtx.Constraints = layout.Exact(image.Pt(size, size))
	ic.Layout(gtx, col)
}

// withTooltip draws w, and the tooltip while hovered is set.
func withTooltip(gtx layout.Context, st *Style, tip *tooltip, hovered bool, text string, w layout.Widget) layout.Dimensions {
	avail := gtx.Constraints.Max.X
	dims := w(gtx)
	tip.Layout(gtx, st, hovered, dims.Size, avail, text)
	return dims
}

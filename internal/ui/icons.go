package ui

import (
	"image"
	"image/color"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

// Icons, from the Material Design icons.
var (
	iconNew        = mustIcon(icons.EditorInsertDriveFile)
	iconOpen       = mustIcon(icons.FileFolderOpen)
	iconUndo       = mustIcon(icons.ContentUndo)
	iconRedo       = mustIcon(icons.ContentRedo)
	iconEdit       = mustIcon(icons.EditorModeEdit)
	iconClose      = mustIcon(icons.NavigationClose)
	iconExpand     = mustIcon(icons.NavigationExpandMore)
	iconCollapse   = mustIcon(icons.NavigationExpandLess)
	iconPrev       = mustIcon(icons.NavigationChevronLeft)
	iconNext       = mustIcon(icons.NavigationChevronRight)
	iconNow        = mustIcon(icons.ActionUpdate)
	iconDelete     = mustIcon(icons.ActionDelete)
	iconNamePoints = mustIcon(icons.EditorFormatListNumbered)
	iconLayers     = mustIcon(icons.MapsLayers)
	iconMap        = mustIcon(icons.MapsMap)
	iconPalette    = mustIcon(icons.ImagePalette)
	iconGradient   = mustIcon(icons.ImageGradient)
	iconSearch     = mustIcon(icons.ActionSearch)
	iconPresets    = mustIcon(icons.ImageCollectionsBookmark)
	iconCheck      = mustIcon(icons.NavigationCheck)
	iconMinimize   = mustIcon(icons.NavigationFullscreenExit)

	// Icons of the main menu items, besides those of the toolbar.
	iconSettings   = mustIcon(icons.ActionSettings)
	iconQuit       = mustIcon(icons.ActionExitToApp)
	iconClearCache = mustIcon(icons.ContentDeleteSweep)
	iconFolder     = mustIcon(icons.FileFolder)
	iconHelp       = mustIcon(icons.ActionHelpOutline)
	iconBug        = mustIcon(icons.ActionBugReport)

	// Icons of the horizontal chart axes and the metrics, in the hover label and the color-by drop-down.
	iconDistance  = mustIcon(icons.ImageStraighten)
	iconTripTime  = mustIcon(icons.ImageTimer)
	iconClockTime = mustIcon(icons.DeviceAccessTime)
	iconElevation = mustIcon(icons.EditorVerticalAlignTop)
	iconMountain  = mustIcon(icons.ImageFilterHDR)
	iconSpeed     = mustIcon(icons.MapsDirectionsRun)
	iconSlope     = mustIcon(icons.DeviceSignalCellular4Bar)
	iconNoColor   = mustIcon(icons.ContentBlock)

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

// Layout draws the button for a toolbar, flat unless hovered or active.
// A disabled button is grayed out.
func (b *iconButton) Layout(gtx layout.Context, st *Style, ic *widget.Icon, tip string, enabled, active bool) layout.Dimensions {
	avail := gtx.Constraints.Max.X
	bgtx := gtx
	if !enabled {
		bgtx = gtx.Disabled()
	}
	dims := b.btn.Layout(bgtx, func(gtx layout.Context) layout.Dimensions {
		size, pad := gtx.Dp(st.ToolIconSize), gtx.Dp(st.ToolIconPad)
		box := image.Pt(size+2*pad, size+2*pad)
		layoutButtonBg(gtx, st, box, enabled && b.btn.Hovered(), active)
		col := st.Theme.Fg
		if !enabled {
			col = st.HintFg
		}
		layoutIcon(gtx, ic, image.Pt(pad, pad), size, col)
		if enabled && !active {
			pointer.CursorPointer.Add(gtx.Ops)
		}
		return layout.Dimensions{Size: box}
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

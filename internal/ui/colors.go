package ui

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"log"
	"strconv"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/widget"
	"github.com/ncruces/zenity"
)

// trackColors are the default colors of opened files, used in turn.
var trackColors = []color.NRGBA{
	{R: 0xe0, G: 0x10, B: 0x10, A: 0xff}, // red
	{R: 0x10, G: 0x50, B: 0xe0, A: 0xff}, // blue
	{R: 0x00, G: 0x99, B: 0x30, A: 0xff}, // green
	{R: 0x90, G: 0x20, B: 0xc0, A: 0xff}, // purple
	{R: 0xf0, G: 0x80, B: 0x00, A: 0xff}, // orange
	{R: 0xe0, G: 0x10, B: 0xa0, A: 0xff}, // magenta
	{R: 0x00, G: 0x99, B: 0xa0, A: 0xff}, // teal
	{R: 0x80, G: 0x50, B: 0x20, A: 0xff}, // brown
}

// formatColor formats an opaque color like "#e01010".
func formatColor(c color.NRGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// parseColor parses a color like "#e01010".
func parseColor(s string) (color.NRGBA, error) {
	if len(s) != 7 || s[0] != '#' {
		return color.NRGBA{}, fmt.Errorf("invalid color %q, expected #rrggbb", s)
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("invalid color %q, expected #rrggbb", s)
	}
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}, nil
}

// formatColorAlpha formats a color like "#e01010", or with opacity like "#e01010c0" if it is not opaque.
func formatColorAlpha(c color.NRGBA) string {
	s := formatColor(c)
	if c.A != 0xff {
		s += fmt.Sprintf("%02x", c.A)
	}
	return s
}

// parseColorAlpha parses a color like "#e01010", or with opacity like "#e01010c0".
func parseColorAlpha(s string) (color.NRGBA, error) {
	c, err := parseColor(s)
	if len(s) == 9 {
		var a uint64
		c, err = parseColor(s[:7])
		if err == nil {
			a, err = strconv.ParseUint(s[7:], 16, 8)
		}
		c.A = uint8(a)
	}
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("invalid color %q, expected #rrggbb or #rrggbbaa", s)
	}
	return c, nil
}

// showColorDialog lets the user choose the color of the given items, without blocking the UI.
func (a *App) showColorDialog(items []*fileItem) {
	if len(items) == 0 {
		return
	}
	pick := a.plat.colorDialog(items[0].color)
	a.dialog(func() func() {
		nc, err := pick()
		if err != nil {
			if !errors.Is(err, zenity.ErrCanceled) {
				log.Printf("color dialog: %v", err)
			}
			return nil
		}
		return func() {
			for _, it := range items {
				it.color = nc
			}
			a.tracksChanged = true
		}
	})
}

// layoutKindSymbol draws a clickable icon for the kind of an item, in its color, for changing the color.
// It is faded while the item is hidden, and highlighted while hovered.
func (a *App) layoutKindSymbol(gtx layout.Context, clk *widget.Clickable, ic *widget.Icon, col color.NRGBA, visible bool) layout.Dimensions {
	st := a.style
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		box := kindSymbolWidth(gtx, st)
		layoutButtonBg(gtx, st, image.Pt(box, box), clk.Hovered(), false)
		size := gtx.Dp(st.IconSize)
		layoutStripedIcon(gtx, ic, image.Pt((box-size)/2, (box-size)/2), size, []color.NRGBA{col}, visible)
		pointer.CursorPointer.Add(gtx.Ops)
		return layout.Dimensions{Size: image.Pt(box, box)}
	})
}

// kindSymbolWidth returns the width of the symbol of an item in the side panel, see [App.layoutKindSymbol].
func kindSymbolWidth(gtx layout.Context, st *Style) int {
	return gtx.Dp(st.IconSize + st.Spacing)
}

// fileSymbolWidth returns the width of the symbol of a file in the side panel, see [App.layoutFileSymbol].
func fileSymbolWidth(gtx layout.Context, st *Style) int {
	return 2*gtx.Dp(st.SmallIconSize) + gtx.Dp(st.Spacing)
}

// layoutFileSymbol draws a clickable symbol of what the file contains, in the colors of its items, for changing them.
// A file with items of one kind shows the icon of the kind. Otherwise, smaller icons show the kinds it has:
// routes above waypoints, and tracks to the right of them. Icons for items with several colors are split into stripes.
// The symbol is faded while the file is hidden, and highlighted while hovered.
func (a *App) layoutFileSymbol(gtx layout.Context, f *openFile) layout.Dimensions {
	st := a.style
	return f.colorBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// As high as the stacked small icons, as the clickable clips its content.
		w, h := fileSymbolWidth(gtx, st), max(gtx.Dp(st.IconSize+st.Spacing), 2*gtx.Dp(st.SmallIconSize))
		layoutButtonBg(gtx, st, image.Pt(w, h), f.colorBtn.Hovered(), false)
		if k, ok := f.singleKind(); ok {
			size := gtx.Dp(st.IconSize)
			layoutStripedIcon(gtx, itemKindIcons[k], image.Pt((w-size)/2, (h-size)/2), size, f.kindColors(k), f.visible.Value)
		} else {
			size := gtx.Dp(st.SmallIconSize)
			x := (w - 2*size) / 2
			at := map[itemKind]image.Point{
				routeItem:    image.Pt(x, h/2-size),
				waypointItem: image.Pt(x, h/2),
				trackItem:    image.Pt(x+size, (h-size)/2),
			}
			for k, p := range at {
				if cols := f.kindColors(k); len(cols) > 0 {
					layoutStripedIcon(gtx, itemKindIcons[k], p, size, cols, f.visible.Value)
				}
			}
		}
		pointer.CursorPointer.Add(gtx.Ops)
		return layout.Dimensions{Size: image.Pt(w, h)}
	})
}

// layoutStripedIcon draws an icon of the given size at an offset, split into vertical stripes of the colors.
// The colors are faded if not visible.
func layoutStripedIcon(gtx layout.Context, ic *widget.Icon, at image.Point, size int, colors []color.NRGBA, visible bool) {
	for i, col := range colors {
		if !visible {
			col.A /= 2
		}
		stripe := image.Rect(at.X+size*i/len(colors), at.Y, at.X+size*(i+1)/len(colors), at.Y+size)
		cl := clip.Rect(stripe).Push(gtx.Ops)
		layoutIcon(gtx, ic, at, size, col)
		cl.Pop()
	}
}

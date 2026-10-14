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
	"gioui.org/op/paint"
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

// layoutSwatch draws a clickable color sample, a small rounded square split into stripes for several colors.
// It has a thin dark border, so that light colors stand out from the panel, and is highlighted while hovered.
func (a *App) layoutSwatch(gtx layout.Context, clk *widget.Clickable, colors []color.NRGBA) layout.Dimensions {
	st := a.style
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		box := gtx.Dp(st.IconSize + st.Spacing)
		layoutButtonBg(gtx, st, image.Pt(box, box), clk.Hovered(), false)
		size := gtx.Dp(st.SwatchSize)
		off := (box - size) / 2
		r := image.Rectangle{Min: image.Pt(off, off), Max: image.Pt(off+size, off+size)}
		rr, border := gtx.Dp(st.CornerRadius), max(1, gtx.Dp(1))
		paint.FillShape(gtx.Ops, st.SwatchBorder, clip.UniformRRect(r, rr).Op(gtx.Ops))
		inner := r.Inset(border)
		cl := clip.UniformRRect(inner, max(0, rr-border)).Push(gtx.Ops)
		for i, c := range colors {
			stripe := inner
			stripe.Min.X = inner.Min.X + inner.Dx()*i/len(colors)
			stripe.Max.X = inner.Min.X + inner.Dx()*(i+1)/len(colors)
			paint.FillShape(gtx.Ops, c, clip.Rect(stripe).Op())
		}
		cl.Pop()
		pointer.CursorPointer.Add(gtx.Ops)
		return layout.Dimensions{Size: image.Pt(box, box)}
	})
}

// layoutKindSymbol draws a clickable icon for the kind of an item, in the item's color, for changing the color.
// It is faded while the item is hidden, and highlighted while hovered.
func (a *App) layoutKindSymbol(gtx layout.Context, clk *widget.Clickable, ic *widget.Icon, col color.NRGBA, visible bool) layout.Dimensions {
	st := a.style
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		box := gtx.Dp(st.IconSize + st.Spacing)
		layoutButtonBg(gtx, st, image.Pt(box, box), clk.Hovered(), false)
		if !visible {
			col.A /= 2
		}
		size := gtx.Dp(st.IconSize)
		layoutIcon(gtx, ic, image.Pt((box-size)/2, (box-size)/2), size, col)
		pointer.CursorPointer.Add(gtx.Ops)
		return layout.Dimensions{Size: image.Pt(box, box)}
	})
}

package ui

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"log"
	"strconv"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"
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

// colorChange is a color chosen in the dialog, for the given items.
type colorChange struct {
	items []*fileItem
	color color.NRGBA
}

// showColorDialog lets the user choose the color of the given items, without blocking the UI.
func (a *App) showColorDialog(items []*fileItem) {
	if len(items) == 0 || !a.dialogOpen.CompareAndSwap(false, true) {
		return
	}
	opts := append([]zenity.Option{
		zenity.Title("Track color"),
		zenity.Color(items[0].color),
	}, a.plat.dialogOptions()...)
	go func() {
		defer a.dialogOpen.Store(false)
		c, err := zenity.SelectColor(opts...)
		if err != nil {
			if !errors.Is(err, zenity.ErrCanceled) {
				log.Printf("color dialog: %v", err)
			}
			return
		}
		nc := color.NRGBAModel.Convert(c).(color.NRGBA)
		nc.A = 0xff
		a.bgMu.Lock()
		a.colorChanges = append(a.colorChanges, colorChange{items: items, color: nc})
		a.bgMu.Unlock()
		a.invalidate()
	}()
}

// applyColors applies colors chosen in the dialog.
func (a *App) applyColors() {
	a.bgMu.Lock()
	changes := a.colorChanges
	a.colorChanges = nil
	a.bgMu.Unlock()

	for _, c := range changes {
		for _, it := range c.items {
			it.color = c.color
		}
		a.tracksChanged = true
	}
}

// layoutSwatch draws a clickable color field, split into stripes for several colors.
func (a *App) layoutSwatch(gtx layout.Context, clk *widget.Clickable, colors []color.NRGBA) layout.Dimensions {
	st := a.style
	return material.Clickable(gtx, clk, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(st.Spacing/2).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			size := gtx.Dp(st.IconSize)
			border := gtx.Dp(1)
			r := image.Rectangle{Max: image.Pt(size, size)}
			paint.FillShape(gtx.Ops, st.PanelBorder, clip.Rect(r).Op())
			inner := r.Inset(border)
			for i, c := range colors {
				stripe := inner
				stripe.Min.X = inner.Min.X + inner.Dx()*i/len(colors)
				stripe.Max.X = inner.Min.X + inner.Dx()*(i+1)/len(colors)
				paint.FillShape(gtx.Ops, c, clip.Rect(stripe).Op())
			}
			return layout.Dimensions{Size: r.Size()}
		})
	})
}

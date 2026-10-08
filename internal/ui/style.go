package ui

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// Style collects the theme and all sizes and colors of the UI, to tune the look in one place.
type Style struct {
	// Theme provides fonts and the base palette for Gio's material widgets.
	Theme *material.Theme

	// TextSize is the size of text in controls and labels.
	TextSize unit.Sp
	// SmallTextSize is the size of secondary text, like the status bar.
	SmallTextSize unit.Sp
	// IconSize is the size of check box and radio button icons.
	IconSize unit.Dp

	// ButtonInset is the padding inside buttons and drop-down headers.
	ButtonInset layout.Inset
	// CornerRadius of buttons and drop-down headers.
	CornerRadius unit.Dp
	// BarInset is the padding of the toolbar and status bar.
	BarInset layout.Inset
	// Spacing between related controls, like a label and its control.
	Spacing unit.Dp
	// GroupSpacing between groups of controls in the toolbar.
	GroupSpacing unit.Dp
	// PanelInset is the padding inside drop-down panels.
	PanelInset unit.Dp

	// PanelBg is the background of drop-down panels.
	PanelBg color.NRGBA
	// PanelBorder is the border color of drop-down panels.
	PanelBorder color.NRGBA
	// StatusBg is the background of the status bar.
	StatusBg color.NRGBA
}

// DefaultStyle returns a compact style.
func DefaultStyle() *Style {
	th := material.NewTheme()
	th.TextSize = 14

	return &Style{
		Theme:         th,
		TextSize:      13,
		SmallTextSize: 12,
		IconSize:      16,

		ButtonInset:  layout.Inset{Top: 3, Bottom: 3, Left: 8, Right: 6},
		CornerRadius: 3,
		BarInset:     layout.UniformInset(3),
		Spacing:      6,
		GroupSpacing: 16,
		PanelInset:   2,

		PanelBg:     color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
		PanelBorder: color.NRGBA{R: 0xa0, G: 0xa0, B: 0xa0, A: 0xff},
		StatusBg:    color.NRGBA{R: 0xf4, G: 0xf4, B: 0xf4, A: 0xff},
	}
}

// Label returns a label with the regular text size.
func (s *Style) Label(txt string) material.LabelStyle {
	return material.Label(s.Theme, s.TextSize, txt)
}

// SmallLabel returns a label with the small text size.
func (s *Style) SmallLabel(txt string) material.LabelStyle {
	return material.Label(s.Theme, s.SmallTextSize, txt)
}

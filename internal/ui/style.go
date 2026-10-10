package ui

import (
	"fmt"
	"image/color"

	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
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
	// SideInset is the padding inside the side panel.
	SideInset layout.Inset
	// DividerWidth is the visible width of the dividers between side panel, map and chart.
	DividerWidth unit.Dp
	// DividerGrip is the width of the area for dragging the divider.
	DividerGrip unit.Dp
	// MinPaneSize is the minimum width or height of the side panel and the map.
	MinPaneSize unit.Dp

	// PanelBg is the background of drop-down panels.
	PanelBg color.NRGBA
	// PanelBorder is the border color of drop-down panels.
	PanelBorder color.NRGBA
	// DividerColor is the color of the dividers between side panel, map and chart.
	DividerColor color.NRGBA
	// StatusBg is the background of the status bar.
	StatusBg color.NRGBA
	// SideBg is the background of the side panel.
	SideBg color.NRGBA
	// HintFg is the color of hint text, like in an empty side panel.
	HintFg color.NRGBA
	// DragBg is the background of a side panel entry while it is dragged.
	DragBg color.NRGBA
	// SelectedBg is the background of the side panel entry shown in the chart.
	SelectedBg color.NRGBA
	// EditActive is the color of the edit button of the file in edit mode.
	EditActive color.NRGBA
	// MutedTrack is the color of the selected track or route on the map,
	// outside of the part visible in the zoomed chart, and of segments without value in tracks colored by value.
	// It can be set in the settings.
	MutedTrack color.NRGBA
	// ChartGrid is the color of the grid lines in the chart.
	ChartGrid color.NRGBA
	// ChartLine is the color of the line on top of the chart's filled area.
	ChartLine color.NRGBA
	// ChartLineWidth is the width of the chart line.
	ChartLineWidth unit.Dp
	// MinChartHeight is the minimum height of the chart. Below 2/3 of it, the chart snaps closed.
	MinChartHeight unit.Dp
	// ChartTickSpacing is the minimum distance between axis ticks in the chart.
	ChartTickSpacing unit.Dp
	// HoverColor is the color of the line and the dot marking the hovered position on the selected item,
	// in the chart and on the map.
	HoverColor color.NRGBA
	// HoverRing is the color of the ring around the hover dot.
	HoverRing color.NRGBA
	// HoverDotSize is the diameter of the hover dot, without the ring.
	HoverDotSize unit.Dp
	// HoverRadius is the distance from the selected item within which the pointer on the map hovers it.
	HoverRadius unit.Dp
	// LegendBg is the background of the legend with scale and color bar on the map.
	LegendBg color.NRGBA
	// LegendWidth is the width of the color bar, and the maximum width of the scale bar, in the legend.
	LegendWidth unit.Dp
}

// DefaultStyle returns a compact style.
func DefaultStyle() *Style {
	th := material.NewTheme()
	th.TextSize = 14
	// A monospaced font keeps changing numbers, like coordinates, from moving around.
	// System fonts remain as a fallback for glyphs that Go Mono lacks.
	faces := monoFaces()
	th.Shaper = text.NewShaper(text.WithCollection(faces))
	th.Face = faces[0].Font.Typeface

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
		SideInset:    layout.UniformInset(6),
		DividerWidth: 4,
		DividerGrip:  7,
		MinPaneSize:  120,

		PanelBg:      color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
		PanelBorder:  color.NRGBA{R: 0xa0, G: 0xa0, B: 0xa0, A: 0xff},
		DividerColor: color.NRGBA{R: 0xd8, G: 0xd8, B: 0xd8, A: 0xff},
		StatusBg:     color.NRGBA{R: 0xf4, G: 0xf4, B: 0xf4, A: 0xff},
		SideBg:       color.NRGBA{R: 0xfa, G: 0xfa, B: 0xfa, A: 0xff},
		HintFg:       color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff},
		DragBg:       color.NRGBA{R: 0xe4, G: 0xe8, B: 0xf4, A: 0xff},
		SelectedBg:   color.NRGBA{R: 0xcc, G: 0xda, B: 0xf4, A: 0xff},
		EditActive:   color.NRGBA{R: 0xd0, G: 0x40, B: 0x20, A: 0xff},
		MutedTrack:   color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xc0},
		ChartGrid:    color.NRGBA{A: 0x20},
		ChartLine:    color.NRGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xd0},

		ChartLineWidth:   1.5,
		ChartTickSpacing: 32,
		MinChartHeight:   50,
		HoverColor:       color.NRGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xff},
		HoverRing:        color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
		HoverDotSize:     8,
		HoverRadius:      12,
		LegendBg:         color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xe0},
		LegendWidth:      180,
	}
}

// monoFaces returns the Go Mono font faces, regular first.
func monoFaces() []font.FontFace {
	var faces []font.FontFace
	for _, ttf := range [][]byte{gomono.TTF, gomonobold.TTF, gomonoitalic.TTF, gomonobolditalic.TTF} {
		f, err := opentype.ParseCollection(ttf)
		if err != nil {
			panic(fmt.Errorf("parsing Go Mono font: %w", err))
		}
		faces = append(faces, f[0])
	}
	return faces
}

// Label returns a label with the regular text size.
func (s *Style) Label(txt string) material.LabelStyle {
	return material.Label(s.Theme, s.TextSize, txt)
}

// SmallLabel returns a label with the small text size.
func (s *Style) SmallLabel(txt string) material.LabelStyle {
	return material.Label(s.Theme, s.SmallTextSize, txt)
}

// Button returns a button with the regular text size.
func (s *Style) Button(clk *widget.Clickable, txt string) material.ButtonStyle {
	b := material.Button(s.Theme, clk, txt)
	b.TextSize = s.TextSize
	b.Inset = s.ButtonInset
	b.CornerRadius = s.CornerRadius
	return b
}

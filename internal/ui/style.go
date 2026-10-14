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
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/gofont/goregular"
)

// brandBlue is the blue of the UI, the middle of the gradient of the logo's tile.
// The tints, neutral colors and the text color are derived from it.
var brandBlue = color.NRGBA{R: 0x27, G: 0x4a, B: 0x6d, A: 0xff}

// brandYellow is the yellow of the boot print in the logo, for markers on the map.
var brandYellow = color.NRGBA{R: 0xf2, G: 0xc2, B: 0x30, A: 0xff}

// Style collects the theme and all sizes and colors of the UI, to tune the look in one place.
type Style struct {
	// Theme provides fonts and the base palette for Gio's material widgets.
	// Its face is the proportional Go font, which has digits of equal width.
	Theme *material.Theme
	// MonoFace is the monospaced typeface, for text aligned by padding with spaces, like the status bar.
	MonoFace font.Typeface

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
	// FieldBorder is the border of text fields, FieldFocus that of the focused one.
	FieldBorder, FieldFocus color.NRGBA
	// ToolbarBg is the background of the toolbar, SeparatorColor that of the lines between its groups.
	ToolbarBg, SeparatorColor color.NRGBA
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
	// EditActive is the color of the edit button of the file in edit mode, and of the selected point.
	EditActive color.NRGBA
	// EditHandleFill and EditHandleRing are the colors of the points of the edited file on the map.
	EditHandleFill, EditHandleRing color.NRGBA
	// EditMidFill is the color of the segment midpoints for inserting points.
	EditMidFill color.NRGBA
	// EditBand is the color of the lines to a point being moved or added.
	EditBand color.NRGBA
	// ToolIconSize is the size of icons in toolbar buttons, ToolIconPad the padding around them.
	ToolIconSize, ToolIconPad unit.Dp
	// EditHandleSize is the diameter of the points of the edited file, EditMidSize that of segment midpoints.
	EditHandleSize, EditMidSize unit.Dp
	// MutedTrack is the color of the selected track or route on the map,
	// outside of the part visible in the zoomed chart, and of segments without value in tracks colored by value.
	// It can be set in the settings. MutedCasing is the border around the muted part, to set it off from the map.
	MutedTrack, MutedCasing color.NRGBA
	// ChartGrid is the color of the grid lines in the chart.
	ChartGrid color.NRGBA
	// ChartLine is the color of the line on top of the chart's filled area.
	ChartLine color.NRGBA
	// ChartTerrain is the color of the line of the terrain elevation below a track, in the chart.
	ChartTerrain color.NRGBA
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
	// TimeBand is the part of the time bar where all overlays have data.
	TimeBand color.NRGBA
	// LegendWidth is the width of the color bar, and the maximum width of the scale bar, in the legend.
	LegendWidth unit.Dp
	// GPSColor is the color of the live GPS position, and of the GPS button while following it.
	// GPSStale is the color of the position while the receiver has no fix.
	// GPSAccuracy is the fill of the circle showing the accuracy of the position.
	GPSColor, GPSStale, GPSAccuracy color.NRGBA
	// GPSDotSize is the diameter of the dot of the live GPS position, without its ring.
	GPSDotSize unit.Dp
	// GPSButtonSize is the diameter of the round GPS button on the map, GPSIconSize that of its icon.
	GPSButtonSize, GPSIconSize unit.Dp
	// SearchWidth is the width of the search field in the toolbar.
	SearchWidth unit.Dp
	// SearchMarker is the color of the marker of a found place, SearchMarkerSize its diameter without the ring.
	SearchMarker     color.NRGBA
	SearchMarkerSize unit.Dp
	// NavLine is the color of a found route, NavCasing that of the border around it.
	NavLine, NavCasing color.NRGBA
	// NavLineWidth is the width of a found route, NavCasingWidth that of the border on each side.
	NavLineWidth, NavCasingWidth unit.Dp
	// NavAlt is the color of alternative routes, NavAltWidth their width.
	NavAlt      color.NRGBA
	NavAltWidth unit.Dp
	// NavStart and NavTarget are the colors of the markers of start and target, NavMarkerSize their diameter without the ring.
	NavStart, NavTarget color.NRGBA
	NavMarkerSize       unit.Dp
	// NavViaSize is the diameter of the markers of via points without the ring.
	NavViaSize unit.Dp
	// NavBand is the color of the lines from a dragged point of a found route to its neighbors.
	NavBand color.NRGBA
	// NavWidth is the width of the route panel.
	NavWidth unit.Dp
}

// DefaultStyle returns a compact style.
func DefaultStyle() *Style {
	th := material.NewTheme()
	th.TextSize = 14
	th.ContrastBg = brandBlue
	th.Fg = mix(color.NRGBA{A: 0xff}, brandBlue, 0.35)
	// The Go fonts are embedded, so the UI looks the same on all systems.
	// System fonts remain as a fallback for glyphs that they lack.
	sans, mono := goFaces(goregular.TTF, gobold.TTF, goitalic.TTF, gobolditalic.TTF),
		goFaces(gomono.TTF, gomonobold.TTF, gomonoitalic.TTF, gomonobolditalic.TTF)
	th.Shaper = text.NewShaper(text.WithCollection(append(sans, mono...)))
	th.Face = sans[0].Font.Typeface

	return &Style{
		Theme:         th,
		MonoFace:      mono[0].Font.Typeface,
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

		PanelBg:        color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
		PanelBorder:    neutral(0xa0),
		DividerColor:   neutral(0xd8),
		FieldBorder:    neutral(0xb8),
		FieldFocus:     brandBlue,
		ToolbarBg:      neutral(0xf4),
		SeparatorColor: neutral(0xc8),
		StatusBg:       neutral(0xf4),
		SideBg:         neutral(0xfa),
		HintFg:         neutral(0x80),
		DragBg:         tint(0.12),
		SelectedBg:     tint(0.25),
		EditActive:     color.NRGBA{R: 0xd0, G: 0x40, B: 0x20, A: 0xff},
		MutedTrack:     color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xc0},
		MutedCasing:    color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
		ChartGrid:      color.NRGBA{A: 0x20},
		ChartLine:      color.NRGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xd0},
		ChartTerrain:   color.NRGBA{R: 0xd0, G: 0x50, B: 0x10, A: 0xe0},

		ChartLineWidth:   1.5,
		ChartTickSpacing: 32,
		MinChartHeight:   50,
		HoverColor:       color.NRGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xff},
		HoverRing:        color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
		HoverDotSize:     8,
		HoverRadius:      12,
		LegendBg:         color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xe0},
		TimeBand:         tint(0.55),
		LegendWidth:      180,
		GPSColor:         color.NRGBA{R: 0x1a, G: 0x73, B: 0xe8, A: 0xff},
		GPSStale:         color.NRGBA{R: 0x90, G: 0x90, B: 0x90, A: 0xff},
		GPSAccuracy:      color.NRGBA{R: 0x1a, G: 0x73, B: 0xe8, A: 0x30},
		GPSDotSize:       14,
		GPSButtonSize:    36,
		GPSIconSize:      22,
		SearchWidth:      280,
		SearchMarker:     brandYellow,
		SearchMarkerSize: 12,
		NavLine:          color.NRGBA{R: 0x1f, G: 0x5f, B: 0xb0, A: 0xff},
		NavCasing:        color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
		NavLineWidth:     5,
		NavAlt:           color.NRGBA{R: 0x8a, G: 0x9b, B: 0xb0, A: 0xff},
		NavAltWidth:      4,
		NavCasingWidth:   1.5,
		NavStart:         color.NRGBA{R: 0x2a, G: 0x9d, B: 0x48, A: 0xff},
		NavTarget:        color.NRGBA{R: 0xc8, G: 0x37, B: 0x2d, A: 0xff},
		NavMarkerSize:    16,
		NavViaSize:       8,
		NavBand:          color.NRGBA{R: 0x1f, G: 0x5f, B: 0xb0, A: 0xa0},
		NavWidth:         290,

		EditHandleFill: color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
		EditHandleRing: color.NRGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xff},
		EditMidFill:    color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xa0},
		EditBand:       color.NRGBA{R: 0xd0, G: 0x40, B: 0x20, A: 0xc0},
		ToolIconSize:   20,
		ToolIconPad:    4,
		EditHandleSize: 9,
		EditMidSize:    7,
	}
}

// goFaces parses the faces of one of the Go fonts, in the given order.
func goFaces(ttfs ...[]byte) []font.FontFace {
	var faces []font.FontFace
	for _, ttf := range ttfs {
		f, err := opentype.ParseCollection(ttf)
		if err != nil {
			panic(fmt.Errorf("parsing Go font: %w", err))
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

// SmallMonoLabel returns a label with the small text size in the monospaced font.
func (s *Style) SmallMonoLabel(txt string) material.LabelStyle {
	l := s.SmallLabel(txt)
	l.Font.Typeface = s.MonoFace
	return l
}

// Button returns a button with the regular text size.
func (s *Style) Button(clk *widget.Clickable, txt string) material.ButtonStyle {
	b := material.Button(s.Theme, clk, txt)
	b.TextSize = s.TextSize
	b.Inset = s.ButtonInset
	b.CornerRadius = s.CornerRadius
	return b
}

// mix blends a with b, by the share t of b.
func mix(a, b color.NRGBA, t float32) color.NRGBA {
	ch := func(x, y uint8) uint8 { return uint8(float32(x)*(1-t) + float32(y)*t + 0.5) }
	return color.NRGBA{R: ch(a.R, b.R), G: ch(a.G, b.G), B: ch(a.B, b.B), A: ch(a.A, b.A)}
}

// tint returns the brand blue mixed with white, with the share t of the blue.
func tint(t float32) color.NRGBA {
	return mix(color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, brandBlue, t)
}

// neutral returns a grey of about the lightness v, with a hint of the brand blue:
// halfway between the plain grey and the tint of the same lightness.
func neutral(v uint8) color.NRGBA {
	luma := func(c color.NRGBA) float32 { return 0.2126*float32(c.R) + 0.7152*float32(c.G) + 0.0722*float32(c.B) }
	t := (255 - float32(v)) / (255 - luma(brandBlue))
	return mix(color.NRGBA{R: v, G: v, B: v, A: 0xff}, tint(t), 0.5)
}

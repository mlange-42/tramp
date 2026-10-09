package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/mapview"
)

var (
	scaleDark  = color.NRGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xff}
	scaleLight = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
)

// scaleLength returns the longest length of 1, 2 or 5 times a power of ten meters
// that fits into maxPx pixels at the given ground resolution in meters per pixel,
// and the number of segments to split it into.
func scaleLength(metersPerPx float64, maxPx int) (meters float64, segments int) {
	maxM := metersPerPx * float64(maxPx)
	if !(maxM >= 1) {
		return 0, 0
	}
	pow := math.Pow(10, math.Floor(math.Log10(maxM)))
	switch {
	case 5*pow <= maxM:
		return 5 * pow, 5
	case 2*pow <= maxM:
		return 2 * pow, 4
	default:
		return pow, 5
	}
}

// formatScale formats a scale length, without decimals, in meters or kilometers.
func formatScale(m float64) string {
	if m < 1000 {
		return fmt.Sprintf("%.0f m", m)
	}
	return fmt.Sprintf("%g km", m/1000)
}

// layoutScaleBar draws a checkerboard scale bar for the center of the view, at most maxWidth wide,
// with the length labelled at its ends.
func layoutScaleBar(gtx layout.Context, st *Style, v *mapview.View, maxWidth int) layout.Dimensions {
	// Web Mercator stretches distances by 1/cos(latitude).
	lat := geo.ToLonLat(v.Center).Lat * math.Pi / 180
	metersPerPx := v.Resolution() * math.Cos(lat)
	meters, n := scaleLength(metersPerPx, maxWidth)
	if n == 0 {
		return layout.Dimensions{}
	}
	width := int(math.Round(meters / metersPerPx))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutCheckerboard(gtx, image.Pt(width, gtx.Dp(8)), n, gtx.Dp(1))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = width
			gtx.Constraints.Max.X = width
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(st.SmallLabel("0").Layout),
				layout.Flexed(1, layout.Spacer{}.Layout),
				layout.Rigid(st.SmallLabel(formatScale(meters)).Layout),
			)
		}),
	)
}

// layoutCheckerboard draws a bar of two rows with n segments of alternating colors,
// where the lower row is the inverse of the upper one, framed by a border.
func layoutCheckerboard(gtx layout.Context, size image.Point, n, border int) layout.Dimensions {
	paint.FillShape(gtx.Ops, scaleDark, clip.Rect{Max: size}.Op())
	mid := size.Y / 2
	for i := range n {
		x0, x1 := size.X*i/n, size.X*(i+1)/n
		if i == 0 {
			x0 += border
		}
		if i == n-1 {
			x1 -= border
		}
		// The light cell is in the upper row for even segments, in the lower row for odd ones.
		r := image.Rect(x0, border, x1, mid)
		if i%2 == 1 {
			r = image.Rect(x0, mid, x1, size.Y-border)
		}
		paint.FillShape(gtx.Ops, scaleLight, clip.Rect(r).Op())
	}
	return layout.Dimensions{Size: size}
}

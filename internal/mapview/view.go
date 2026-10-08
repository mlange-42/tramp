// Package mapview provides an interactive, pannable and zoomable map widget.
package mapview

import (
	"image"
	"math"

	"github.com/mlange-42/tramp/internal/geo"
)

// ZoomStep is the zoom increment. The zoom level is always a multiple of it.
const ZoomStep = 0.5

// SnapZoom rounds a zoom level to the nearest multiple of [ZoomStep].
func SnapZoom(z float64) float64 {
	return math.Round(z/ZoomStep) * ZoomStep
}

// View is the visible part of the map: a center, a zoom level and a size in pixels.
type View struct {
	// Center of the view in Web Mercator coordinates.
	Center geo.Point
	// Zoom is the zoom level, matching tile levels at integer values.
	// It is kept at multiples of [ZoomStep].
	Zoom float64
	// Size of the view in pixels.
	Size image.Point
}

// Resolution returns the ground resolution of the view in meters per pixel.
func (v *View) Resolution() float64 {
	return geo.Resolution(0) / math.Exp2(v.Zoom)
}

// ToScreen converts a map position to pixel coordinates in the view.
func (v *View) ToScreen(p geo.Point) (x, y float64) {
	res := v.Resolution()
	x = (p.X-v.Center.X)/res + float64(v.Size.X)/2
	y = (v.Center.Y-p.Y)/res + float64(v.Size.Y)/2
	return x, y
}

// ToMap converts pixel coordinates in the view to a map position.
func (v *View) ToMap(x, y float64) geo.Point {
	res := v.Resolution()
	return geo.Point{
		X: v.Center.X + (x-float64(v.Size.X)/2)*res,
		Y: v.Center.Y - (y-float64(v.Size.Y)/2)*res,
	}
}

// Bounds returns the visible extent in Web Mercator coordinates.
func (v *View) Bounds() geo.Rect {
	tl := v.ToMap(0, 0)
	br := v.ToMap(float64(v.Size.X), float64(v.Size.Y))
	return geo.Rect{Min: geo.Point{X: tl.X, Y: br.Y}, Max: geo.Point{X: br.X, Y: tl.Y}}
}

// Pan moves the map content by the given number of pixels.
func (v *View) Pan(dx, dy float64) {
	res := v.Resolution()
	v.Center.X -= dx * res
	v.Center.Y += dy * res
	v.clampCenter()
}

// ZoomAt changes the zoom level by dz, keeping the map position under the pixel (x, y) fixed.
// The result is snapped to a multiple of [ZoomStep] and clamped to [minZoom, maxZoom].
func (v *View) ZoomAt(x, y, dz, minZoom, maxZoom float64) {
	anchor := v.ToMap(x, y)
	v.Zoom = math.Max(minZoom, math.Min(maxZoom, SnapZoom(v.Zoom+dz)))
	res := v.Resolution()
	v.Center.X = anchor.X - (x-float64(v.Size.X)/2)*res
	v.Center.Y = anchor.Y + (y-float64(v.Size.Y)/2)*res
	v.clampCenter()
}

// Fit centers the view on r and sets the largest zoom level at which r is fully visible,
// rounded down to a multiple of [ZoomStep].
// The result is clamped to [minZoom, maxZoom]. The view size must be set.
func (v *View) Fit(r geo.Rect, minZoom, maxZoom float64) {
	if v.Size.X <= 0 || v.Size.Y <= 0 {
		return
	}
	res := math.Max((r.Max.X-r.Min.X)/float64(v.Size.X), (r.Max.Y-r.Min.Y)/float64(v.Size.Y))
	v.Zoom = math.Max(minZoom, math.Min(maxZoom, math.Floor(math.Log2(geo.Resolution(0)/res)/ZoomStep)*ZoomStep))
	v.Center = geo.Point{X: (r.Min.X + r.Max.X) / 2, Y: (r.Min.Y + r.Max.Y) / 2}
	v.clampCenter()
}

func (v *View) clampCenter() {
	v.Center.X = math.Max(-geo.OriginShift, math.Min(geo.OriginShift, v.Center.X))
	v.Center.Y = math.Max(-geo.OriginShift, math.Min(geo.OriginShift, v.Center.Y))
}

package mapview

import (
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"github.com/mlange-42/tramp/internal/geo"
)

// minStep is the minimum screen distance in pixels between drawn line vertices.
// Closer points are skipped, which keeps long, dense tracks fast at low zoom.
const minStep = 1.5

// Polyline is a line on the map, in Web Mercator coordinates.
type Polyline struct {
	Points []geo.Point
	// Bounds is the extent of the points, for culling.
	Bounds geo.Rect
}

// NewPolyline creates a polyline and computes its bounds.
func NewPolyline(pts []geo.Point) Polyline {
	b := geo.EmptyRect()
	for _, p := range pts {
		b = b.Extend(p)
	}
	return Polyline{Points: pts, Bounds: b}
}

// LineGroup is a set of polylines and dots drawn in one color.
type LineGroup struct {
	Color color.NRGBA
	Lines []Polyline
	Dots  []geo.Point
}

// Lines draws groups of polylines and dots on top of the map, later groups on top.
//
// The drawing is cached and only rebuilt when the zoom, the view size or the content changes,
// or when the view was panned far. Lines are cut to an area around the view,
// and vertices closer than [minStep] pixels are skipped.
type Lines struct {
	// Width of the lines.
	Width unit.Dp
	// DotRadius is the radius of dots.
	DotRadius unit.Dp

	groups  []LineGroup
	version int

	cache  op.Ops
	call   op.CallOp
	key    linesKey
	origin geo.Point
}

type linesKey struct {
	version int
	zoom    float64
	sizeX   int
	sizeY   int
	width   int
	radius  int
}

// Set replaces the drawn groups.
func (l *Lines) Set(groups []LineGroup) {
	l.groups = groups
	l.version++
}

// Layout draws the lines for the given view, which must have its size set.
func (l *Lines) Layout(gtx layout.Context, v *View) {
	if len(l.groups) == 0 {
		return
	}
	key := linesKey{
		version: l.version,
		zoom:    v.Zoom,
		sizeX:   v.Size.X,
		sizeY:   v.Size.Y,
		width:   gtx.Dp(l.Width),
		radius:  gtx.Dp(l.DotRadius),
	}
	res := v.Resolution()
	dx := (l.origin.X - v.Center.X) / res
	dy := (v.Center.Y - l.origin.Y) / res
	if key != l.key || math.Abs(dx) > float64(v.Size.X)/2 || math.Abs(dy) > float64(v.Size.Y)/2 {
		l.key = key
		l.origin = v.Center
		l.rebuild(v, key)
		dx, dy = 0, 0
	}

	defer clip.Rect{Max: v.Size}.Push(gtx.Ops).Pop()
	defer op.Affine(f32.AffineId().Offset(f32.Pt(float32(dx), float32(dy)))).Push(gtx.Ops).Pop()
	l.call.Add(gtx.Ops)
}

// rebuild records the drawing for a view centered at l.origin.
func (l *Lines) rebuild(v *View, key linesKey) {
	l.cache.Reset()
	macro := op.Record(&l.cache)

	ov := *v
	ov.Center = l.origin
	// Keep everything within one view size around the view, so that panning can reuse the drawing.
	w, h := float64(v.Size.X), float64(v.Size.Y)
	area := screenRect{minX: -w, minY: -h, maxX: 2 * w, maxY: 2 * h}
	b := ov.Bounds()
	bw, bh := b.Max.X-b.Min.X, b.Max.Y-b.Min.Y
	cull := geo.Rect{Min: geo.Point{X: b.Min.X - bw, Y: b.Min.Y - bh}, Max: geo.Point{X: b.Max.X + bw, Y: b.Max.Y + bh}}

	for i := range l.groups {
		l.drawGroup(&l.groups[i], &ov, area, cull, key)
	}
	l.call = macro.Stop()
}

func (l *Lines) drawGroup(g *LineGroup, v *View, area screenRect, cull geo.Rect, key linesKey) {
	var path clip.Path
	path.Begin(&l.cache)
	n := 0
	for i := range g.Lines {
		if g.Lines[i].Bounds.Intersects(cull) {
			n += appendLine(&path, v, area, g.Lines[i].Points)
		}
	}
	spec := path.End()
	if n > 0 {
		paint.FillShape(&l.cache, g.Color, clip.Stroke{Path: spec, Width: float32(key.width)}.Op())
	}

	r := float32(key.radius)
	for _, p := range g.Dots {
		x, y := v.ToScreen(p)
		if !area.contains(x, y) {
			continue
		}
		c := f32.Pt(float32(x), float32(y))
		ell := clip.Ellipse{Min: c.Sub(f32.Pt(r, r)).Round(), Max: c.Add(f32.Pt(r, r)).Round()}
		paint.FillShape(&l.cache, g.Color, ell.Op(&l.cache))
	}
}

// screenRect is a rectangle in screen coordinates.
type screenRect struct {
	minX, minY, maxX, maxY float64
}

func (r screenRect) contains(x, y float64) bool {
	return x >= r.minX && x <= r.maxX && y >= r.minY && y <= r.maxY
}

// outside reports whether the segment from (x0, y0) to (x1, y1) is certainly outside r,
// i.e. both ends are beyond the same edge.
func (r screenRect) outside(x0, y0, x1, y1 float64) bool {
	return (x0 < r.minX && x1 < r.minX) || (x0 > r.maxX && x1 > r.maxX) ||
		(y0 < r.minY && y1 < r.minY) || (y0 > r.maxY && y1 > r.maxY)
}

// appendLine adds the parts of a line that may be visible in area to path, in screen coordinates of v.
// It returns the number of drawn line segments.
func appendLine(path *clip.Path, v *View, area screenRect, pts []geo.Point) int {
	if len(pts) < 2 {
		return 0
	}
	n := 0
	lx, ly := v.ToScreen(pts[0])
	penDown := false
	for i := 1; i < len(pts); i++ {
		x, y := v.ToScreen(pts[i])
		if area.outside(lx, ly, x, y) {
			lx, ly = x, y
			penDown = false
			continue
		}
		ddx, ddy := x-lx, y-ly
		if ddx*ddx+ddy*ddy < minStep*minStep && i < len(pts)-1 {
			continue
		}
		if !penDown {
			path.MoveTo(f32.Pt(float32(lx), float32(ly)))
			penDown = true
		}
		path.LineTo(f32.Pt(float32(x), float32(y)))
		lx, ly = x, y
		n++
	}
	return n
}

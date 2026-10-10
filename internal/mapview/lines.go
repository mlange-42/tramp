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

// casingWidth is how much wider the casing below colored lines is than the lines.
const casingWidth unit.Dp = 3

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

// LineGroup is a set of polylines and dots drawn in one color,
// or with a color gradient by value.
type LineGroup struct {
	Color color.NRGBA
	Lines []Polyline
	Dots  []geo.Point
	// Width of the lines.
	Width unit.Dp
	// DotSize is the diameter of dots.
	DotSize unit.Dp
	// Values are optional values for coloring the lines with the [Coloring] of [Lines].
	// If not nil, there is one entry per line, which is nil or has one value per line segment.
	// Lines and segments without values (nil or NaN) are drawn in the NoValue color of the [Coloring].
	// Without coloring, all lines are drawn in Color.
	Values [][]float64
}

// Coloring maps values to colors.
type Coloring struct {
	// Min and Max are the values for the first and the last color.
	// Values outside are clamped.
	Min, Max float64
	// Colors are the colors for equal-width value bins from Min to Max.
	Colors []color.NRGBA
	// Casing is drawn below colored lines, to set them off from the map.
	Casing color.NRGBA
	// NoValue is the color of lines and segments without values in groups with values.
	// It is distinct from the group colors, which could be mistaken for values.
	NoValue color.NRGBA
}

// Index returns the index of the color for value v.
func (c *Coloring) Index(v float64) int {
	t := (v - c.Min) / (c.Max - c.Min)
	if !(t > 0) { // also NaN, for Min == Max
		return 0
	}
	return min(int(t*float64(len(c.Colors))), len(c.Colors)-1)
}

// Lines draws groups of polylines and dots on top of the map, later groups on top.
//
// The drawing is cached and only rebuilt when the zoom, the view size or the content changes,
// or when the view was panned far. Lines are cut to an area around the view,
// and vertices closer than [minStep] pixels are skipped.
// Lines colored by value are drawn with one path per color.
type Lines struct {
	groups   []LineGroup
	coloring *Coloring
	version  int
	// bounds is the extent of all lines and dots.
	bounds geo.Rect
	// complete is whether the cached drawing contains everything, so that it never needs
	// to be rebuilt for panning.
	complete bool

	// Reused buffers for collecting line segments: solid, casing, without value, and one per color.
	solid   strokes
	casing  strokes
	noValue strokes
	bins    []strokes

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
	pxPerDp float32
}

// Set replaces the drawn groups, and the coloring for groups with values.
// Without coloring, all lines are drawn in their group's color.
func (l *Lines) Set(groups []LineGroup, coloring *Coloring) {
	l.groups = groups
	l.coloring = coloring
	l.version++
	l.bounds = geo.EmptyRect()
	for _, g := range groups {
		for _, line := range g.Lines {
			l.bounds = l.bounds.Union(line.Bounds)
		}
		for _, p := range g.Dots {
			l.bounds = l.bounds.Extend(p)
		}
	}
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
		pxPerDp: gtx.Metric.PxPerDp,
	}
	res := v.Resolution()
	dx := (l.origin.X - v.Center.X) / res
	dy := (v.Center.Y - l.origin.Y) / res
	panned := !l.complete && (math.Abs(dx) > float64(v.Size.X)/2 || math.Abs(dy) > float64(v.Size.Y)/2)
	if key != l.key || panned {
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
	l.complete = cull.Contains(l.bounds)

	for i := range l.groups {
		l.drawGroup(&l.groups[i], &ov, area, cull, key)
	}
	l.call = macro.Stop()
}

func (l *Lines) drawGroup(g *LineGroup, v *View, area screenRect, cull geo.Rect, key linesKey) {
	c := l.coloring
	colored := c != nil && len(c.Colors) > 0 && g.Values != nil
	l.solid.reset()
	l.casing.reset()
	l.noValue.reset()
	if colored {
		for len(l.bins) < len(c.Colors) {
			l.bins = append(l.bins, strokes{})
		}
		for i := range l.bins {
			l.bins[i].reset()
		}
	}

	for i := range g.Lines {
		if !g.Lines[i].Bounds.Intersects(cull) {
			continue
		}
		var vals []float64
		if colored {
			vals = g.Values[i]
		}
		walkLine(v, area, g.Lines[i].Points, func(a, b f32.Point, seg int) {
			if !colored {
				l.solid.add(a, b)
				return
			}
			l.casing.add(a, b)
			if vals == nil || math.IsNaN(vals[seg]) {
				l.noValue.add(a, b)
				return
			}
			l.bins[c.Index(vals[seg])].add(a, b)
		})
	}

	m := unit.Metric{PxPerDp: key.pxPerDp}
	width := float32(m.Dp(g.Width))
	if colored {
		l.stroke(&l.casing, width+float32(m.Dp(casingWidth)), c.Casing)
	}
	l.stroke(&l.solid, width, g.Color)
	if colored {
		l.stroke(&l.noValue, width, c.NoValue)
		for i := range c.Colors {
			l.stroke(&l.bins[i], width, c.Colors[i])
		}
	}

	r := float32(m.Dp(g.DotSize)) / 2
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

// stroke draws the collected line segments with the given width.
//
// Each segment is drawn as a filled rectangle, extended by half the width at both ends
// so that neighbors overlap at joints. This is much faster than [clip.Stroke],
// which computes round joins and caps on the CPU in every frame after a rebuild.
// The rectangles are filled by the non-zero winding rule, so overlaps don't cancel out.
func (l *Lines) stroke(s *strokes, width float32, col color.NRGBA) {
	if len(s.pts) == 0 {
		return
	}
	hw := width / 2
	var path clip.Path
	path.Begin(&l.cache)
	for r, start := range s.starts {
		end := len(s.pts)
		if r+1 < len(s.starts) {
			end = s.starts[r+1]
		}
		for i := start + 1; i < end; i++ {
			a, b := s.pts[i-1], s.pts[i]
			d := b.Sub(a)
			length := float32(math.Hypot(float64(d.X), float64(d.Y)))
			if length == 0 {
				continue
			}
			d = d.Mul(hw / length)
			n := f32.Pt(-d.Y, d.X)
			a, b = a.Sub(d), b.Add(d)
			path.MoveTo(a.Add(n))
			path.LineTo(b.Add(n))
			path.LineTo(b.Sub(n))
			path.LineTo(a.Sub(n))
			path.Close()
		}
	}
	paint.FillShape(&l.cache, col, clip.Outline{Path: path.End()}.Op())
}

// strokes collects line segments as connected runs of points.
type strokes struct {
	pts []f32.Point
	// starts are the indices in pts where runs start.
	starts []int
}

func (s *strokes) reset() {
	s.pts = s.pts[:0]
	s.starts = s.starts[:0]
}

// add adds a segment, continuing the last run if it ends at a.
func (s *strokes) add(a, b f32.Point) {
	if len(s.pts) == 0 || s.pts[len(s.pts)-1] != a {
		s.starts = append(s.starts, len(s.pts))
		s.pts = append(s.pts, a)
	}
	s.pts = append(s.pts, b)
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

// walkLine calls fn for the parts of a line that may be visible in area, in screen coordinates of v.
// Vertices closer than [minStep] to the last drawn vertex are skipped.
// seg is the index of the last original segment that a drawn segment covers.
func walkLine(v *View, area screenRect, pts []geo.Point, fn func(a, b f32.Point, seg int)) {
	if len(pts) < 2 {
		return
	}
	lx, ly := v.ToScreen(pts[0])
	for i := 1; i < len(pts); i++ {
		x, y := v.ToScreen(pts[i])
		if area.outside(lx, ly, x, y) {
			lx, ly = x, y
			continue
		}
		ddx, ddy := x-lx, y-ly
		if ddx*ddx+ddy*ddy < minStep*minStep && i < len(pts)-1 {
			continue
		}
		fn(f32.Pt(float32(lx), float32(ly)), f32.Pt(float32(x), float32(y)), i-1)
		lx, ly = x, y
	}
}

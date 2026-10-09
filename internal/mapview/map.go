package mapview

import (
	"image"
	"image/color"
	"math"
	"slices"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/tiles"
)

const (
	// scrollPerLevel is the scroll distance for one zoom level.
	// A mouse wheel notch scrolls 120 units on most platforms.
	scrollPerLevel = 240
	// maxFallbackLevels is how many levels up the map looks for a coarser tile
	// to show while a tile is loading.
	maxFallbackLevels = 6
)

var background = color.NRGBA{R: 0xe0, G: 0xe0, B: 0xe0, A: 0xff}

// Map is an interactive map widget showing tiles from a [tiles.Manager].
// It is panned by dragging with the secondary (right) mouse button, and zoomed with the scroll wheel.
type Map struct {
	View    View
	MinZoom float64
	MaxZoom float64
	// MaxTileLevel is the highest tile level requested from the server.
	// The map is magnified beyond that.
	MaxTileLevel int

	// Hover is the map position under the pointer, if HoverValid.
	Hover      geo.Point
	HoverValid bool

	// scroll is the accumulated scroll distance not yet applied as a zoom step, in zoom levels.
	scroll float64
	// fit is an area to show once the view size is known.
	fit      *geo.Rect
	dragging bool
	dragID   pointer.ID
	last     f32.Point
	wanted   []geo.TileKey
}

// New creates a map centered on the given position.
func New(center geo.LonLat, zoom float64) *Map {
	return &Map{
		View:         View{Center: geo.ToMercator(center), Zoom: SnapZoom(zoom)},
		MinZoom:      1,
		MaxZoom:      21,
		MaxTileLevel: 19,
	}
}

// SetZoom sets the zoom level, snapped to a multiple of [ZoomStep] and clamped to [Map.MinZoom, Map.MaxZoom].
func (m *Map) SetZoom(z float64) {
	m.View.Zoom = math.Max(m.MinZoom, math.Min(m.MaxZoom, SnapZoom(z)))
}

// Fit shows the given area, with the largest zoom level at which it is fully visible.
// If the map was not laid out yet, this happens on the first layout.
func (m *Map) Fit(r geo.Rect) {
	if m.View.Size.X > 0 && m.View.Size.Y > 0 {
		m.View.Fit(r, m.MinZoom, m.MaxZoom)
		return
	}
	m.fit = &r
}

// Layer is a tile layer to draw on the map.
type Layer struct {
	Tiles *tiles.Manager
	// Opacity of the layer. Zero is treated as fully opaque.
	Opacity float32
	// MaxTileLevel limits [Map.MaxTileLevel] for this layer. Zero means no extra limit.
	MaxTileLevel int
}

// Layout handles input and draws the layers bottom to top, filling the maximum constraints.
func (m *Map) Layout(gtx layout.Context, layers ...Layer) layout.Dimensions {
	size := gtx.Constraints.Max
	m.View.Size = size
	if m.fit != nil && size.X > 0 && size.Y > 0 {
		m.View.Fit(*m.fit, m.MinZoom, m.MaxZoom)
		m.fit = nil
	}
	m.update(gtx)

	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: background}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)

	for _, l := range layers {
		m.drawLayer(gtx, l)
	}

	event.Op(gtx.Ops, m)
	if m.dragging {
		pointer.CursorGrabbing.Add(gtx.Ops)
	}
	return layout.Dimensions{Size: size}
}

func (m *Map) update(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  m,
			Kinds:   pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Move | pointer.Leave | pointer.Scroll,
			ScrollX: pointer.ScrollRange{Min: math.MinInt32, Max: math.MaxInt32},
			ScrollY: pointer.ScrollRange{Min: math.MinInt32, Max: math.MaxInt32},
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Press:
			if !m.dragging && e.Buttons.Contain(pointer.ButtonSecondary) {
				m.dragging = true
				m.dragID = e.PointerID
				m.last = e.Position
				gtx.Execute(pointer.GrabCmd{Tag: m, ID: e.PointerID})
			}
		case pointer.Drag:
			if m.dragging && e.PointerID == m.dragID {
				d := e.Position.Sub(m.last)
				m.View.Pan(float64(d.X), float64(d.Y))
				m.last = e.Position
			}
		case pointer.Release, pointer.Cancel:
			// Releasing another button doesn't end the drag.
			if e.PointerID == m.dragID && (e.Kind == pointer.Cancel || !e.Buttons.Contain(pointer.ButtonSecondary)) {
				m.dragging = false
			}
		case pointer.Scroll:
			// Touchpads scroll in small amounts, so zoom once a full step has accumulated.
			m.scroll -= float64(e.Scroll.Y) / scrollPerLevel
			if dz := math.Trunc(m.scroll/ZoomStep) * ZoomStep; dz != 0 {
				m.scroll -= dz
				m.View.ZoomAt(float64(e.Position.X), float64(e.Position.Y), dz, m.MinZoom, m.MaxZoom)
			}
		case pointer.Leave:
			m.HoverValid = false
			continue
		}
		m.Hover = m.View.ToMap(float64(e.Position.X), float64(e.Position.Y))
		m.HoverValid = true
	}
}

// TileLevel returns the tile level used for the current zoom.
func (m *Map) TileLevel() int {
	return m.tileLevel(0)
}

// tileLevel returns the tile level for the current zoom, additionally limited by maxLevel if positive.
func (m *Map) tileLevel(maxLevel int) int {
	z := int(math.Round(m.View.Zoom))
	return max(0, min(m.maxTileLevel(maxLevel), z))
}

func (m *Map) maxTileLevel(maxLevel int) int {
	if maxLevel > 0 {
		return min(m.MaxTileLevel, maxLevel)
	}
	return m.MaxTileLevel
}

func (m *Map) drawLayer(gtx layout.Context, l Layer) {
	if l.Tiles == nil {
		return
	}
	if l.Opacity > 0 && l.Opacity < 1 {
		defer paint.PushOpacity(gtx.Ops, l.Opacity).Pop()
	}
	m.drawTiles(gtx, l.Tiles, l.MaxTileLevel)
}

func (m *Map) drawTiles(gtx layout.Context, mgr *tiles.Manager, maxLevel int) {
	z := m.tileLevel(maxLevel)
	n := geo.TileCount(z)
	b := m.View.Bounds()
	tl := geo.TileAt(z, geo.Point{X: b.Min.X, Y: b.Max.Y})
	br := geo.TileAt(z, geo.Point{X: b.Max.X, Y: b.Min.Y})
	x0, x1 := max(0, tl.X), min(n-1, br.X)
	y0, y1 := max(0, tl.Y), min(n-1, br.Y)

	m.wanted = m.wanted[:0]
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			key := geo.TileKey{Z: z, X: x, Y: y}
			if tile, ok := mgr.Get(key); ok {
				m.drawTile(gtx, tile, m.screenRect(key))
				continue
			}
			m.wanted = append(m.wanted, key)
			m.drawFallback(gtx, mgr, key, maxLevel)
		}
	}

	// Load tiles closest to the view center first.
	center := geo.TileAt(z, m.View.Center)
	slices.SortFunc(m.wanted, func(a, b geo.TileKey) int {
		return tileDist(a, center) - tileDist(b, center)
	})
	mgr.Request(m.wanted)
}

// drawFallback fills the area of a missing tile with the four finer tiles if all are cached,
// as after zooming out, or otherwise with a coarser cached tile.
// It never draws both, so that semi-transparent layers don't get darker where they overlap.
func (m *Map) drawFallback(gtx layout.Context, mgr *tiles.Manager, key geo.TileKey, maxLevel int) {
	if key.Z < m.maxTileLevel(maxLevel) {
		var children [4]*tiles.Tile
		complete := true
		for i := range children {
			child := geo.TileKey{Z: key.Z + 1, X: key.X*2 + i%2, Y: key.Y*2 + i/2}
			tile, ok := mgr.Get(child)
			if !ok {
				complete = false
				break
			}
			children[i] = tile
		}
		if complete {
			for _, tile := range children {
				m.drawTile(gtx, tile, m.screenRect(tile.Key))
			}
			return
		}
	}

	area := m.screenRect(key)
	for p, i := key, 0; p.Z > 0 && i < maxFallbackLevels; i++ {
		p = p.Parent()
		if tile, ok := mgr.Get(p); ok {
			stack := clip.Rect(area).Push(gtx.Ops)
			m.drawTile(gtx, tile, m.screenRect(p))
			stack.Pop()
			return
		}
	}
}

// drawTile draws a tile image stretched to the given screen rectangle.
func (m *Map) drawTile(gtx layout.Context, tile *tiles.Tile, r image.Rectangle) {
	defer clip.Rect(r).Push(gtx.Ops).Pop()
	sz := tile.Op.Size()
	scale := f32.Pt(float32(r.Dx())/float32(sz.X), float32(r.Dy())/float32(sz.Y))
	tr := f32.AffineId().Scale(f32.Point{}, scale).Offset(f32.Pt(float32(r.Min.X), float32(r.Min.Y)))
	defer op.Affine(tr).Push(gtx.Ops).Pop()
	tile.Op.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
}

// screenRect returns the tile extent in pixels, rounded so that neighboring tiles share edges.
func (m *Map) screenRect(key geo.TileKey) image.Rectangle {
	b := key.Bounds()
	x0, y0 := m.View.ToScreen(geo.Point{X: b.Min.X, Y: b.Max.Y})
	x1, y1 := m.View.ToScreen(geo.Point{X: b.Max.X, Y: b.Min.Y})
	return image.Rect(int(math.Round(x0)), int(math.Round(y0)), int(math.Round(x1)), int(math.Round(y1)))
}

func tileDist(a, b geo.TileKey) int {
	dx, dy := a.X-b.X, a.Y-b.Y
	return dx*dx + dy*dy
}

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

	dragging bool
	dragID   pointer.ID
	last     f32.Point
	wanted   []geo.TileKey
}

// New creates a map centered on the given position.
func New(center geo.LonLat, zoom float64) *Map {
	return &Map{
		View:         View{Center: geo.ToMercator(center), Zoom: zoom},
		MinZoom:      1,
		MaxZoom:      21,
		MaxTileLevel: 19,
	}
}

// Layout handles input and draws the map, filling the maximum constraints.
func (m *Map) Layout(gtx layout.Context, mgr *tiles.Manager) layout.Dimensions {
	size := gtx.Constraints.Max
	m.View.Size = size
	m.update(gtx)

	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: background}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)

	m.drawTiles(gtx, mgr)

	event.Op(gtx.Ops, m)
	if m.dragging {
		pointer.CursorGrabbing.Add(gtx.Ops)
	} else {
		pointer.CursorGrab.Add(gtx.Ops)
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
			if !m.dragging && e.Buttons.Contain(pointer.ButtonPrimary) {
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
			if e.PointerID == m.dragID {
				m.dragging = false
			}
		case pointer.Scroll:
			dz := -float64(e.Scroll.Y) / scrollPerLevel
			m.View.ZoomAt(float64(e.Position.X), float64(e.Position.Y), dz, m.MinZoom, m.MaxZoom)
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
	z := int(math.Round(m.View.Zoom))
	return max(0, min(m.MaxTileLevel, z))
}

func (m *Map) drawTiles(gtx layout.Context, mgr *tiles.Manager) {
	if mgr == nil {
		return
	}
	z := m.TileLevel()
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
			m.drawFallback(gtx, mgr, key)
		}
	}

	// Load tiles closest to the view center first.
	center := geo.TileAt(z, m.View.Center)
	slices.SortFunc(m.wanted, func(a, b geo.TileKey) int {
		return tileDist(a, center) - tileDist(b, center)
	})
	mgr.Request(m.wanted)
}

// drawFallback fills the area of a missing tile with a coarser cached tile,
// or with finer tiles when zooming out.
func (m *Map) drawFallback(gtx layout.Context, mgr *tiles.Manager, key geo.TileKey) {
	area := m.screenRect(key)
	for p, i := key, 0; p.Z > 0 && i < maxFallbackLevels; i++ {
		p = p.Parent()
		if tile, ok := mgr.Get(p); ok {
			stack := clip.Rect(area).Push(gtx.Ops)
			m.drawTile(gtx, tile, m.screenRect(p))
			stack.Pop()
			break
		}
	}
	if key.Z >= m.MaxTileLevel {
		return
	}
	for dy := range 2 {
		for dx := range 2 {
			child := geo.TileKey{Z: key.Z + 1, X: key.X*2 + dx, Y: key.Y*2 + dy}
			if tile, ok := mgr.Get(child); ok {
				m.drawTile(gtx, tile, m.screenRect(child))
			}
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

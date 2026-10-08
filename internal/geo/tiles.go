package geo

import "math"

// TileSize is the edge length of a map tile in pixels.
const TileSize = 256

// TileKey identifies a tile in the Web Mercator tile pyramid.
// Y counts from the top (north), as in the common XYZ scheme.
type TileKey struct {
	Z, X, Y int
}

// Resolution returns the ground resolution in meters per pixel at tile level z.
func Resolution(z int) float64 {
	return 2 * OriginShift / (TileSize * math.Exp2(float64(z)))
}

// TileSpan returns the edge length of a tile at level z in meters.
func TileSpan(z int) float64 {
	return 2 * OriginShift / math.Exp2(float64(z))
}

// TileCount returns the number of tiles along each axis at level z.
func TileCount(z int) int {
	return 1 << z
}

// Bounds returns the extent of the tile in Web Mercator coordinates.
func (k TileKey) Bounds() Rect {
	span := TileSpan(k.Z)
	minX := -OriginShift + float64(k.X)*span
	maxY := OriginShift - float64(k.Y)*span
	return Rect{
		Min: Point{minX, maxY - span},
		Max: Point{minX + span, maxY},
	}
}

// Parent returns the tile at level Z-1 that contains this tile.
func (k TileKey) Parent() TileKey {
	return TileKey{k.Z - 1, k.X >> 1, k.Y >> 1}
}

// Valid reports whether the tile lies inside the tile grid.
func (k TileKey) Valid() bool {
	n := TileCount(k.Z)
	return k.Z >= 0 && k.X >= 0 && k.Y >= 0 && k.X < n && k.Y < n
}

// TileAt returns the tile at level z that contains p.
// The result may lie outside the grid for points outside the Web Mercator extent.
func TileAt(z int, p Point) TileKey {
	span := TileSpan(z)
	x := int(math.Floor((p.X + OriginShift) / span))
	y := int(math.Floor((OriginShift - p.Y) / span))
	return TileKey{z, x, y}
}

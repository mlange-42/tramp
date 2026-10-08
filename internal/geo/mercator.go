// Package geo provides coordinate conversions and the tile grid used by the map.
package geo

import "math"

const (
	// EarthRadius is the WGS84 semi-major axis in meters, as used by Web Mercator.
	EarthRadius = 6378137.0
	// OriginShift is half the extent of the Web Mercator plane in meters.
	OriginShift = math.Pi * EarthRadius
	// MaxLat is the latitude limit of Web Mercator in degrees.
	MaxLat = 85.05112877980659
)

// Point is a position in Web Mercator (EPSG:3857) coordinates, in meters.
type Point struct {
	X, Y float64
}

// LonLat is a geographic position in degrees (WGS84).
type LonLat struct {
	Lon, Lat float64
}

// Rect is an axis-aligned rectangle in Web Mercator coordinates.
type Rect struct {
	Min, Max Point
}

// ToMercator converts geographic coordinates to Web Mercator.
func ToMercator(ll LonLat) Point {
	lat := math.Max(-MaxLat, math.Min(MaxLat, ll.Lat))
	x := ll.Lon * OriginShift / 180
	y := math.Log(math.Tan((90+lat)*math.Pi/360)) * EarthRadius
	return Point{x, y}
}

// ToLonLat converts Web Mercator coordinates to geographic coordinates.
func ToLonLat(p Point) LonLat {
	lon := p.X / OriginShift * 180
	lat := 360/math.Pi*math.Atan(math.Exp(p.Y/EarthRadius)) - 90
	return LonLat{lon, lat}
}

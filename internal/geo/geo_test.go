package geo

import (
	"math"
	"testing"
)

func TestMercatorRoundTrip(t *testing.T) {
	for _, ll := range []LonLat{{0, 0}, {12.37, 51.34}, {-122.4, 37.8}, {179.9, -80}} {
		got := ToLonLat(ToMercator(ll))
		if math.Abs(got.Lon-ll.Lon) > 1e-9 || math.Abs(got.Lat-ll.Lat) > 1e-9 {
			t.Errorf("round trip of %v gave %v", ll, got)
		}
	}
}

func TestMercatorExtent(t *testing.T) {
	p := ToMercator(LonLat{180, MaxLat})
	if math.Abs(p.X-OriginShift) > 1e-6 || math.Abs(p.Y-OriginShift) > 1e-3 {
		t.Errorf("expected corner at %f, got %v", OriginShift, p)
	}
}

func TestTileAtBounds(t *testing.T) {
	p := ToMercator(LonLat{12.37, 51.34})
	for z := 0; z < 20; z++ {
		k := TileAt(z, p)
		if !k.Valid() {
			t.Fatalf("tile %v not valid", k)
		}
		b := k.Bounds()
		if p.X < b.Min.X || p.X >= b.Max.X || p.Y <= b.Min.Y || p.Y > b.Max.Y {
			t.Errorf("point %v not in bounds %v of tile %v", p, b, k)
		}
		if z > 0 && TileAt(z-1, p) != k.Parent() {
			t.Errorf("parent of %v should be %v", k, TileAt(z-1, p))
		}
	}
}

func TestResolution(t *testing.T) {
	if r := Resolution(0); math.Abs(r-156543.03392804097) > 1e-6 {
		t.Errorf("unexpected resolution at level 0: %f", r)
	}
}

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

func TestLonLatString(t *testing.T) {
	for _, c := range []struct {
		ll   LonLat
		want string
	}{
		{LonLat{12.3731, 51.3397}, "51.33970°N   12.37310°E"},
		{LonLat{-122.4, -33.86}, "33.86000°S  122.40000°W"},
		{LonLat{0, 0}, " 0.00000°N    0.00000°E"},
		{LonLat{-0.000001, -0.000001}, " 0.00000°N    0.00000°E"},
		{LonLat{-5.5, 8.25}, " 8.25000°N    5.50000°W"},
	} {
		if got := c.ll.String(); got != c.want {
			t.Errorf("%v: got %q, want %q", c.ll, got, c.want)
		}
	}
}

func TestRect(t *testing.T) {
	r := EmptyRect()
	if !r.Empty() {
		t.Error("expected empty rect")
	}
	r = r.Extend(Point{1, 2}).Extend(Point{-1, 5})
	if r != (Rect{Min: Point{-1, 2}, Max: Point{1, 5}}) || r.Empty() {
		t.Errorf("unexpected rect %v", r)
	}
	if u := r.Union(EmptyRect()); u != r {
		t.Errorf("union with empty rect changed %v to %v", r, u)
	}
	if u := r.Union(Rect{Min: Point{0, 0}, Max: Point{3, 3}}); u != (Rect{Min: Point{-1, 0}, Max: Point{3, 5}}) {
		t.Errorf("unexpected union %v", u)
	}
	if !r.Intersects(Rect{Min: Point{1, 5}, Max: Point{2, 6}}) || r.Intersects(Rect{Min: Point{1.1, 0}, Max: Point{2, 6}}) {
		t.Error("unexpected intersection result")
	}
}

func TestDistance(t *testing.T) {
	// One degree of latitude is about 111.2 km.
	if d := Distance(LonLat{12, 51}, LonLat{12, 52}); math.Abs(d-111_195) > 10 {
		t.Errorf("unexpected distance %f", d)
	}
	if d := Distance(LonLat{12.37, 51.34}, LonLat{12.37, 51.34}); d != 0 {
		t.Errorf("expected zero distance, got %f", d)
	}
}

func TestRectContains(t *testing.T) {
	r := Rect{Min: Point{0, 0}, Max: Point{10, 10}}
	if !r.Contains(Rect{Min: Point{1, 1}, Max: Point{10, 9}}) || !r.Contains(EmptyRect()) {
		t.Error("expected contained")
	}
	if r.Contains(Rect{Min: Point{-1, 1}, Max: Point{5, 5}}) || r.Contains(Rect{Min: Point{5, 5}, Max: Point{11, 6}}) {
		t.Error("expected not contained")
	}
}

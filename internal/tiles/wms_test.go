package tiles

import (
	"image"
	"image/color"
	"testing"
)

func TestShade(t *testing.T) {
	src := image.NewRGBA(image.Rect(10, 10, 13, 11))
	src.Set(10, 10, color.RGBA{255, 255, 255, 255})
	src.Set(11, 10, color.RGBA{0, 0, 0, 255})
	src.Set(12, 10, color.RGBA{128, 128, 128, 255})

	dst := Shade(src)
	if dst.Bounds() != image.Rect(0, 0, 3, 1) {
		t.Fatalf("unexpected bounds %v", dst.Bounds())
	}
	expected := []uint8{0, 255, 127}
	for x, a := range expected {
		c := dst.RGBAAt(x, 0)
		if c.R != 0 || c.G != 0 || c.B != 0 || c.A != a {
			t.Errorf("pixel %d: expected black with alpha %d, got %v", x, a, c)
		}
	}
}

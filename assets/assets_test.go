package assets

import (
	"bytes"
	"image/png"
	"testing"
)

func TestIconPNG(t *testing.T) {
	for _, c := range []struct{ size, want int }{{1, 16}, {16, 16}, {21, 24}, {48, 48}, {100, 256}, {1000, 256}} {
		data, got := IconPNG(c.size)
		if got != c.want {
			t.Errorf("size %d: got %d, want %d", c.size, got, c.want)
			continue
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("size %d: %v", c.size, err)
		}
		if b := img.Bounds(); b.Dx() != got || b.Dy() != got {
			t.Errorf("size %d: image is %v", c.size, b)
		}
	}
}

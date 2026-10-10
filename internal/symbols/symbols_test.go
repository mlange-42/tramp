package symbols

import (
	"image"
	"image/draw"
	"testing"

	"golang.org/x/exp/shiny/iconvg"
)

func TestLookup(t *testing.T) {
	for _, c := range []struct{ name, want string }{
		{"Summit", "Summit"},
		{"summit", "Summit"},
		{"  PEAK ", "Summit"},
		{"Flag, Blue", "Flag, Blue"},
		{"flag-blue", "Flag, Blue"},
		{"City (Large)", "City (Large)"},
		{"trailhead", "Trail Head"},
		{"Start", "Trail Head"},
	} {
		s, ok := Lookup(c.name)
		if !ok || s.Name != c.want {
			t.Errorf("Lookup(%q) = %v, %v, want %q", c.name, s, ok, c.want)
		}
	}
	for _, n := range []string{"", "Nonexistent Thing"} {
		if _, ok := Lookup(n); ok {
			t.Errorf("Lookup(%q) found a symbol", n)
		}
	}
	if s, _ := Lookup("Waypoint"); s.Icon != nil {
		t.Error("Waypoint has an icon")
	}
	if s, _ := Lookup("Flag, Red"); s.Color != red || s.Icon == nil {
		t.Error("Flag, Red has no icon or the wrong color")
	}
}

// TestIcons checks that all icons decode and cover a reasonable part of their area.
func TestIcons(t *testing.T) {
	for name, data := range makiIcons {
		img := image.NewAlpha(image.Rect(0, 0, 30, 30))
		var r iconvg.Rasterizer
		r.SetDstImage(img, img.Bounds(), draw.Src)
		if err := iconvg.Decode(&r, data, nil); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		n := 0
		for _, a := range img.Pix {
			if a > 0x80 {
				n++
			}
		}
		if f := float64(n) / float64(len(img.Pix)); f < 0.1 || f > 0.8 {
			t.Errorf("%s: %.0f%% of the icon covered", name, 100*f)
		}
	}
}

func TestMatches(t *testing.T) {
	s, _ := Lookup("Lodging")
	for _, q := range []string{"", "lodg", "HUT", "alpine-hut"} {
		if !s.Matches(q) {
			t.Errorf("expected %q to match", q)
		}
	}
	if s.Matches("summit") {
		t.Error("unexpected match")
	}
	if s.Category != "Outdoors" {
		t.Errorf("unexpected category %q", s.Category)
	}
}

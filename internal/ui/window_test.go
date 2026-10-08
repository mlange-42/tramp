package ui

import (
	"image"
	"testing"
)

func TestTitleVisible(t *testing.T) {
	work := image.Rect(0, 0, 1920, 1040)
	tests := []struct {
		name string
		r    image.Rectangle
		want bool
	}{
		{"inside", image.Rect(100, 100, 900, 700), true},
		{"invisible border", image.Rect(-8, 0, 800, 600), true},
		{"mostly left of screen", image.Rect(-750, 100, 50, 700), false},
		{"other monitor", image.Rect(2000, 100, 2800, 700), false},
		{"above screen", image.Rect(100, -30, 900, 570), false},
	}
	for _, tt := range tests {
		if got := titleVisible(tt.r, work); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestFitWindow(t *testing.T) {
	work := image.Rect(0, 0, 1920, 1040)
	tests := []struct {
		name string
		r    image.Rectangle
		keep bool
		want image.Rectangle
	}{
		{"fits", image.Rect(100, 100, 900, 700), true, image.Rect(100, 100, 900, 700)},
		{"snapped with invisible borders", image.Rect(-7, 0, 967, 1047), true, image.Rect(-7, 0, 967, 1047)},
		{"too large", image.Rect(100, 100, 2100, 1300), true, image.Rect(0, 0, 1920, 1040)},
		{"too wide", image.Rect(100, 100, 2100, 700), true, image.Rect(0, 100, 1920, 700)},
		{"center", image.Rect(3000, 100, 3800, 700), false, image.Rect(560, 220, 1360, 820)},
		{"center too large", image.Rect(3000, 100, 6000, 700), false, image.Rect(0, 220, 1920, 820)},
	}
	for _, tt := range tests {
		if got := fitWindow(tt.r, work, tt.keep); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

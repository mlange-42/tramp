package mapview

import (
	"image"
	"image/color"
	"math"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/mlange-42/tramp/internal/geo"
)

// benchTrack returns a wiggly track of n points around the view center, with values.
func benchTrack(n int) (Polyline, []float64) {
	pts := make([]geo.Point, n)
	vals := make([]float64, n-1)
	for i := range pts {
		t := float64(i) / float64(n)
		pts[i] = geo.Point{X: 8000 * math.Cos(2*math.Pi*t) * (1 + 0.1*math.Sin(60*math.Pi*t)), Y: 6000 * math.Sin(2*math.Pi*t)}
		if i < n-1 {
			vals[i] = math.Sin(40*math.Pi*t) + 0.3*math.Sin(997*t)
		}
	}
	return NewPolyline(pts), vals
}

func benchLines(b *testing.B, colored bool, bins int, pan, rebuild bool) {
	w, err := headless.NewWindow(1600, 900)
	if err != nil {
		b.Skip("no headless GPU:", err)
	}
	defer w.Release()

	line, vals := benchTrack(11000)
	g := LineGroup{Color: color.NRGBA{R: 255, A: 255}, Lines: []Polyline{line}, Width: 3, DotSize: 8}
	var c *Coloring
	if colored {
		g.Values = [][]float64{vals}
		c = &Coloring{Min: -1, Max: 1, Colors: make([]color.NRGBA, bins), Casing: color.NRGBA{A: 0xcc}}
		for i := range c.Colors {
			c.Colors[i] = color.NRGBA{R: uint8(i * 255 / bins), B: 128, A: 255}
		}
	}
	var l Lines
	l.Set([]LineGroup{g, g}, c)

	v := View{Zoom: 13, Size: image.Pt(1600, 900)}
	var ops op.Ops
	i := 0
	for b.Loop() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(v.Size)}
		if pan {
			v.Center.X = float64(i%200) * v.Resolution() * 2
		}
		if rebuild {
			l.version++
		}
		l.Layout(gtx, &v)
		if err := w.Frame(&ops); err != nil {
			b.Fatal(err)
		}
		i++
	}
}

func BenchmarkLinesSolidPan(b *testing.B)        { benchLines(b, false, 0, true, false) }
func BenchmarkLinesColoredPan(b *testing.B)      { benchLines(b, true, 32, true, false) }
func BenchmarkLinesSolidRebuild(b *testing.B)    { benchLines(b, false, 0, true, true) }
func BenchmarkLinesColoredRebuild(b *testing.B)  { benchLines(b, true, 32, true, true) }
func BenchmarkLinesColored8Rebuild(b *testing.B) { benchLines(b, true, 8, true, true) }

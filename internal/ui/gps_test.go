package ui

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/mlange-42/tramp/internal/settings"
)

func TestGPSButton(t *testing.T) {
	a := newApp(func() {}, Options{
		Maps:  DefaultMaps(),
		State: settings.Settings{View: &settings.View{Map: "None", Zoom: 5}, GPS: settings.GPS{Port: "no-such-port"}},
	})
	defer a.closeTiles()
	defer a.stopGPS()

	var r input.Router
	var ops op.Ops
	size := image.Pt(800, 600)
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: r.Source(), Now: time.Now(), Constraints: layout.Exact(size)}
		gtx.Metric.PxPerDp, gtx.Metric.PxPerSp = 1, 1
		a.updateGPS(gtx)
		a.layoutMap(gtx)
		r.Frame(&ops)
	}
	// The button is at the bottom right of the map.
	pad := float32(a.style.Spacing * 2)
	half := float32(a.style.GPSButtonSize) / 2
	at := f32.Pt(float32(size.X)-pad-half, float32(size.Y)-pad-half)
	click := func() {
		r.Queue(
			pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at},
			pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: at},
		)
		frame()
	}

	frame()
	click()
	if a.gps.rec == nil || !a.gps.following {
		t.Fatalf("GPS not started: following %v", a.gps.following)
	}
	// While following, a click turns the GPS off.
	click()
	if a.gps.rec != nil {
		t.Fatal("GPS not stopped")
	}
	click()
	a.gps.following = false
	// While not following, a click follows again.
	click()
	if a.gps.rec == nil || !a.gps.following {
		t.Fatalf("not following")
	}
}

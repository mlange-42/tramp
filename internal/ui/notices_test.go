package ui

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"github.com/mlange-42/tramp/internal/settings"
)

func TestNotices(t *testing.T) {
	// Without map and elevations, so that the test doesn't download anything.
	a := newApp(func() {}, Options{Maps: DefaultMaps(), Notices: []Notice{{Text: "first", LogLink: true}, {Text: "second"}}, State: settings.Settings{
		View:      &settings.View{Map: "None", Zoom: 5},
		Elevation: settings.Elevation{Source: "none"},
	}})
	defer a.closeTiles()
	a.geocoder = nil
	var r input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: r.Source(), Now: time.Now(), Constraints: layout.Exact(image.Pt(1000, 700))}
		gtx.Metric.PxPerDp, gtx.Metric.PxPerSp = 1, 1
		a.layout(gtx)
		r.Frame(&ops)
	}
	frame()
	if len(a.notices) != 2 {
		t.Fatalf("expected 2 notices, got %d", len(a.notices))
	}
	a.notices[0].close.btn.Click()
	frame()
	if len(a.notices) != 1 || a.notices[0].Text != "second" {
		t.Fatalf("expected the second notice left, got %d", len(a.notices))
	}
	a.notify(Notice{Text: "third"})
	frame()
	if len(a.notices) != 2 || a.notices[1].Text != "third" {
		t.Errorf("expected the new notice added, got %d", len(a.notices))
	}
}

func TestFlowText(t *testing.T) {
	st := DefaultStyle()
	var ops op.Ops
	var link widget.Clickable
	lay := func(w int) layout.Dimensions {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Constraints: layout.Constraints{Max: image.Pt(w, 1000)}}
		gtx.Metric.PxPerDp, gtx.Metric.PxPerSp = 1, 1
		return layoutFlowText(gtx, st, textSpan{text: "Details are in "}, textSpan{text: "tramp.log", link: &link}, textSpan{text: "."})
	}
	wide := lay(1000)
	if wide.Size.X >= 1000 || wide.Size.Y == 0 {
		t.Fatalf("expected one short line, got %v", wide.Size)
	}
	// Slightly too narrow, the last word wraps to a second line.
	narrow := lay(wide.Size.X - 1)
	if narrow.Size.Y != 2*wide.Size.Y || narrow.Size.X >= wide.Size.X {
		t.Errorf("expected 2 lines of height %d, got %v", wide.Size.Y, narrow.Size)
	}
}

package ui

import (
	"image"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/mlange-42/tramp/internal/wms"
)

// newCapsServer serves capabilities with a radar layer with times and a warnings layer without.
func newCapsServer(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("REQUEST") != "GetCapabilities" {
			http.Error(w, "nope", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`<WMS_Capabilities><Capability><Layer>
<Layer><Name>Radar</Name>
<Dimension name="time" default="current">2026-10-13T10:00:00.000Z/2026-10-13T12:55:00.000Z/PT5M</Dimension>
<Dimension name="reference_time" default="2026-10-13T10:55:00.000Z">x</Dimension></Layer>
<Layer><Name>Warnings</Name></Layer>
</Layer></Capability></WMS_Capabilities>`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// waitOverlayTime waits for the capabilities of an overlay, and applies them.
func waitOverlayTime(a *App, i int) {
	for a.overlays[i].times.loading {
		a.runUITasks()
		time.Sleep(time.Millisecond)
	}
}

func TestOverlayTime(t *testing.T) {
	srv := newCapsServer(t)
	a := newApp(func() {}, Options{
		Maps: []Map{{Layer: wms.Layer{Name: "None", Type: wms.TypeNone}}},
		Overlays: []Overlay{
			{Layer: wms.Layer{Name: "Radar", URL: srv.URL, Layers: "dwd:Radar"}, Refresh: 5 * time.Minute},
			{Layer: wms.Layer{Name: "Warnings", URL: srv.URL, Layers: "Warnings"}, Refresh: 5 * time.Minute},
			{Layer: wms.Layer{Name: "Tiles", Type: wms.TypeXYZ, URL: srv.URL + "/{z}/{x}/{y}.png"}, Refresh: 5 * time.Minute},
		},
	})
	defer a.closeTiles()

	// A time within the valid times of the test server, independent of the current time.
	a.timeCtl.selected = time.Date(2026, 10, 13, 11, 0, 0, 0, time.UTC)

	// The overlay is not drawn until its time is known.
	a.setOverlay(0, true)
	o := &a.overlays[0]
	if !o.times.waiting || !o.times.loading {
		t.Fatal("expected the overlay to wait for its time")
	}
	waitOverlayTime(a, 0)
	if o.times.waiting || o.times.time.IsZero() || o.times.reference != "2026-10-13T10:55:00.000Z" {
		t.Fatalf("unexpected times %+v", o.times)
	}
	if o.timeValue != wms.FormatTime(o.times.time) {
		t.Errorf("unexpected time value %q", o.timeValue)
	}

	// The time closest to now, clamped to the valid times.
	at := func(s string) time.Time { v, _ := time.Parse(time.RFC3339, s); return v }
	caps, err := wms.ParseCapabilities(strings.NewReader(`<WMS_Capabilities><Capability><Layer><Name>Radar</Name>
<Dimension name="time">2026-10-13T10:00:00Z/2026-10-13T12:55:00Z/PT5M</Dimension></Layer></Capability></WMS_Capabilities>`))
	if err != nil {
		t.Fatal(err)
	}
	a.updateTime(o, caps, at("2026-10-13T11:21:00Z"))
	if !o.times.time.Equal(at("2026-10-13T11:20:00Z")) || o.timeValue != "2026-10-13T11:20:00.000Z" {
		t.Errorf("unexpected time %v", o.times.time)
	}
	// Failing capabilities keep the time.
	a.updateTime(o, nil, at("2026-10-13T11:31:00Z"))
	if !o.times.time.Equal(at("2026-10-13T11:20:00Z")) || o.timeValue != "2026-10-13T11:20:00.000Z" {
		t.Errorf("expected the time to be kept, got %v", o.times.time)
	}
	// A layer that lost its times is requested for the default time.
	caps.Layers["Radar"].Time = nil
	a.updateTime(o, caps, at("2026-10-13T11:31:00Z"))
	if !o.times.time.IsZero() || o.timeValue != "" {
		t.Errorf("expected the default time, got %v %q", o.times.time, o.timeValue)
	}

	// Layers without a time dimension and XYZ layers have no time.
	a.setOverlay(1, true)
	waitOverlayTime(a, 1)
	if w := a.overlays[1].times; w.waiting || !w.time.IsZero() {
		t.Errorf("unexpected times %+v", w)
	}
	a.setOverlay(2, true)
	if w := a.overlays[2].times; w.waiting || w.loading {
		t.Errorf("XYZ overlay should not read capabilities: %+v", w)
	}

	// Results for an overlay that was disabled meanwhile are dropped.
	a.setOverlay(1, false)
	a.setOverlay(1, true)
	session := a.overlays[1].times.session
	a.setOverlay(1, false)
	a.runUITasks()
	if a.overlays[1].tiles != nil || a.overlays[1].times.session != session {
		t.Error("disabled overlay changed")
	}
}

func TestFormatDataTime(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.Local)
	if got := formatDataTime(now.Add(-30*time.Minute), now); got != "11:30" {
		t.Errorf("got %q", got)
	}
	if got := formatDataTime(now.Add(24*time.Hour), now); got != "Wed 12:00" {
		t.Errorf("got %q", got)
	}
}

func TestTimeControl(t *testing.T) {
	srv := newCapsServer(t)
	a := newApp(func() {}, Options{
		Maps: []Map{{Layer: wms.Layer{Name: "None", Type: wms.TypeNone}}},
		Overlays: []Overlay{
			{Layer: wms.Layer{Name: "Radar", URL: srv.URL, Layers: "Radar"}, Refresh: 5 * time.Minute},
			{Layer: wms.Layer{Name: "Warnings", URL: srv.URL, Layers: "Warnings"}, Refresh: 5 * time.Minute},
		},
	})
	defer a.closeTiles()
	if _, _, ok := a.timeBounds(); ok {
		t.Error("no times without enabled overlays")
	}
	now := time.Date(2026, 10, 13, 11, 22, 0, 0, time.UTC)
	a.timeCtl.selected = now
	for i := range a.overlays {
		a.setOverlay(i, true)
		waitOverlayTime(a, i)
	}
	first, last, ok := a.timeBounds()
	if !ok || first.Hour() != 10 || last.Hour() != 12 || last.Minute() != 55 {
		t.Fatalf("unexpected bounds %v %v %v", first, last, ok)
	}
	radar, warnings := &a.overlays[0], &a.overlays[1]
	if !radar.times.time.Equal(now.Add(-2 * time.Minute)) {
		t.Errorf("unexpected radar time %v", radar.times.time)
	}

	// Steps go to full hours.
	a.selectAllTimes(time.Time{}, now)
	c := &a.timeCtl
	if !c.step(now, 1).Equal(time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)) ||
		!c.step(now, -1).Equal(time.Date(2026, 10, 13, 11, 0, 0, 0, time.UTC)) {
		t.Errorf("unexpected steps %v %v", c.step(now, 1), c.step(now, -1))
	}
	a.selectAllTimes(c.step(now, -1), now)
	if !c.step(now, -1).Equal(time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("unexpected step from a full hour %v", c.step(now, -1))
	}
	if !radar.times.time.Equal(c.selected) || radar.timeValue != "2026-10-13T11:00:00.000Z" {
		t.Errorf("radar did not follow the selected time: %v", radar.times.time)
	}

	// Outside the valid times, the overlay is hidden; overlays without times are not affected.
	a.selectAllTimes(time.Date(2026, 10, 13, 15, 0, 0, 0, time.UTC), now)
	if !radar.times.outside || !radar.times.hidden() || warnings.times.hidden() {
		t.Errorf("unexpected visibility %+v %+v", radar.times, warnings.times)
	}
	a.selectAllTimes(time.Time{}, now)
	if radar.times.outside || !c.selected.IsZero() {
		t.Error("expected the current time again")
	}
}

func TestTimeline(t *testing.T) {
	srv := newCapsServer(t)
	a := newApp(func() {}, Options{
		Maps:     []Map{{Layer: wms.Layer{Name: "None", Type: wms.TypeNone}}},
		Overlays: []Overlay{{Layer: wms.Layer{Name: "Radar", URL: srv.URL, Layers: "Radar"}, Refresh: 5 * time.Minute}},
	})
	defer a.closeTiles()
	var r input.Router
	var ops op.Ops
	size := image.Pt(800, 400)
	now := time.Date(2026, 10, 13, 11, 22, 0, 0, time.UTC)
	height := 0
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: r.Source(), Now: now, Constraints: layout.Exact(size)}
		gtx.Metric.PxPerDp, gtx.Metric.PxPerSp = 1, 1
		a.updateTimeControl(gtx)
		height = a.layoutTimeline(gtx)
		r.Frame(&ops)
	}
	frame()
	if height != 0 {
		t.Error("no time strip without overlays with times")
	}
	a.timeCtl.selected = now
	a.setOverlay(0, true)
	waitOverlayTime(a, 0)
	frame()
	if height == 0 || a.timeCtl.barWidth <= 0 {
		t.Fatalf("expected the time strip, height %d", height)
	}
	if r, ok := a.timeRanges(); !ok || !r.common || !r.first.Equal(r.commonFirst) || !r.last.Equal(r.commonLast) {
		t.Errorf("unexpected ranges %+v", r)
	}

	// A click near the left end of the bar selects the first full hour.
	barLeft := float32(size.X) - float32(a.style.Spacing)*3 - float32(a.timeCtl.barWidth)
	at := f32.Pt(barLeft+2, float32(size.Y-height/2))
	r.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: at},
	)
	frame()
	frame()
	if want := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC); !a.timeCtl.selected.Equal(want) {
		t.Errorf("selected %v, want %v", a.timeCtl.selected, want)
	}
}

func TestTimeAt(t *testing.T) {
	c := &timeControl{barWidth: 100, barFirst: time.Date(2026, 10, 13, 10, 20, 0, 0, time.UTC), barLast: time.Date(2026, 10, 13, 20, 20, 0, 0, time.UTC)}
	for x, want := range map[float32]int{-5: 11, 0: 11, 47: 15, 100: 20, 120: 20} {
		if got := c.timeAt(x); got.Hour() != want || got.Minute() != 0 {
			t.Errorf("at %v: got %v, want %d:00", x, got, want)
		}
	}
}

package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	if l := o.fetcher.CurrentLayer(); l.Time != wms.FormatTime(o.times.time) || !l.Transparent {
		t.Errorf("unexpected layer %+v", l)
	}

	// The time closest to now, clamped to the valid times.
	at := func(s string) time.Time { v, _ := time.Parse(time.RFC3339, s); return v }
	caps, err := wms.ParseCapabilities(strings.NewReader(`<WMS_Capabilities><Capability><Layer><Name>Radar</Name>
<Dimension name="time">2026-10-13T10:00:00Z/2026-10-13T12:55:00Z/PT5M</Dimension></Layer></Capability></WMS_Capabilities>`))
	if err != nil {
		t.Fatal(err)
	}
	o.updateTime(caps, at("2026-10-13T11:21:00Z"))
	if !o.times.time.Equal(at("2026-10-13T11:20:00Z")) || o.fetcher.CurrentLayer().Time != "2026-10-13T11:20:00.000Z" {
		t.Errorf("unexpected time %v", o.times.time)
	}
	// Failing capabilities keep the time.
	o.updateTime(nil, at("2026-10-13T11:31:00Z"))
	if !o.times.time.Equal(at("2026-10-13T11:20:00Z")) || o.fetcher.CurrentLayer().Time != "2026-10-13T11:20:00.000Z" {
		t.Errorf("expected the time to be kept, got %v", o.times.time)
	}
	// A layer that lost its times is requested for the default time.
	caps.Layers["Radar"].Time = nil
	o.updateTime(caps, at("2026-10-13T11:31:00Z"))
	if !o.times.time.IsZero() || o.fetcher.CurrentLayer().Time != "" {
		t.Errorf("expected the default time, got %v %q", o.times.time, o.fetcher.CurrentLayer().Time)
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

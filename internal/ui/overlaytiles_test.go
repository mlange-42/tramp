package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/tiles"
	"github.com/mlange-42/tramp/internal/wms"
)

func TestOverlayTimeTiles(t *testing.T) {
	srv := newCapsServer(t)
	a := newApp(func() {}, Options{
		Maps:     []Map{{Layer: wms.Layer{Name: "None", Type: wms.TypeNone}}},
		Overlays: []Overlay{{Layer: wms.Layer{Name: "Radar", URL: srv.URL, Layers: "Radar"}, Refresh: 5 * time.Minute}},
	})
	defer a.closeTiles()
	at := func(h, m int) time.Time { return time.Date(2026, 10, 13, h, m, 0, 0, time.UTC) }
	value := func(h, m int) string { return wms.FormatTime(at(h, m)) }

	a.timeCtl.selected = at(11, 0)
	a.setOverlay(0, true)
	waitOverlayTime(a, 0)
	o := &a.overlays[0]
	if o.timeValue != value(11, 0) || fmt.Sprint(o.prefetch) != fmt.Sprint([]string{value(10, 0), value(12, 0)}) {
		t.Fatalf("unexpected time %q and prefetch %v", o.timeValue, o.prefetch)
	}

	// The neighboring times are loaded in the background once the shown tiles are loaded.
	a.prefetchTimes()
	if len(o.others) != 2 {
		t.Fatalf("expected two times loading in the background, got %d", len(o.others))
	}
	next := o.others[1].tiles
	shown := o.tiles

	// Stepping shows the tiles loaded in the background, with the previous ones below until loaded.
	a.selectAllTimes(at(12, 0), at(11, 22))
	if o.tiles != next || o.fallback != shown || o.timeValue != value(12, 0) {
		t.Error("expected the tiles loaded in the background")
	}
	// Beyond the last valid time, there is nothing to load for the next step.
	if fmt.Sprint(o.prefetch) != fmt.Sprint([]string{value(11, 0)}) {
		t.Errorf("unexpected prefetch %v", o.prefetch)
	}
	// Going back shows the kept tiles.
	a.selectAllTimes(at(11, 0), at(11, 22))
	if o.tiles != shown {
		t.Error("expected the kept tiles")
	}
	a.prefetchTimes()
	if o.fallback != nil {
		t.Error("previous tiles should not be drawn once the shown ones are loaded")
	}

	// New data drops the tiles of other times.
	caps, err := wms.ParseCapabilities(strings.NewReader(`<WMS_Capabilities><Capability><Layer><Name>Radar</Name>
<Dimension name="time">2026-10-13T10:00:00Z/2026-10-13T12:55:00Z/PT5M</Dimension>
<Dimension name="reference_time" default="2026-10-13T11:00:00.000Z">x</Dimension></Layer></Capability></WMS_Capabilities>`))
	if err != nil {
		t.Fatal(err)
	}
	a.updateTime(o, caps, at(11, 0))
	if len(o.others) != 0 || o.tiles != shown {
		t.Errorf("expected only the shown tiles after new data, got %d others", len(o.others))
	}

	// Disabling closes all tiles.
	a.prefetchTimes()
	a.setOverlay(0, false)
	if o.tiles != nil || o.others != nil || o.fallback != nil || o.timeValue != "" {
		t.Error("expected no tiles after disabling")
	}
}

func TestPruneTimes(t *testing.T) {
	a := newApp(func() {}, Options{Maps: []Map{{Layer: wms.Layer{Name: "None", Type: wms.TypeNone}}}})
	o := &overlayState{prefetch: []string{"p"}}
	start := time.Now()
	for i := range timeKeep + 2 {
		m := tiles.NewManager(&tiles.WMSFetcher{}, tiles.Options{Workers: 1})
		o.others = append(o.others, &timeTiles{value: fmt.Sprint(i), tiles: m, used: start.Add(time.Duration(i) * time.Second)})
	}
	o.others = append(o.others, &timeTiles{value: "p", tiles: tiles.NewManager(&tiles.WMSFetcher{}, tiles.Options{Workers: 1})})
	o.fallback = o.others[0].tiles
	a.pruneTimes(o)
	var values []string
	for _, x := range o.others {
		values = append(values, x.value)
	}
	// The least recently shown are dropped, the ones loaded in the background are kept.
	if fmt.Sprint(values) != "[2 3 4 5 p]" || o.fallback != nil {
		t.Errorf("unexpected kept times %v", values)
	}
	for _, m := range o.allTiles() {
		m.Close()
	}
}

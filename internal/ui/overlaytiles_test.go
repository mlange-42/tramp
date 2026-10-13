package ui

import (
	"context"
	"errors"
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
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
	if o.timeValue != value(11, 0) || fmt.Sprint(o.prefetch) != fmt.Sprint([]string{value(10, 55), value(11, 5)}) {
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
	a.selectAllTimes(at(11, 5), at(11, 22))
	if o.tiles != next || o.fallback != shown || o.timeValue != value(11, 5) {
		t.Error("expected the tiles loaded in the background")
	}
	if fmt.Sprint(o.prefetch) != fmt.Sprint([]string{value(11, 0), value(11, 10)}) {
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

	// Beyond the last valid time, there is nothing to load for the next step.
	a.selectAllTimes(at(12, 55), at(11, 22))
	if fmt.Sprint(o.prefetch) != fmt.Sprint([]string{value(12, 50)}) {
		t.Errorf("unexpected prefetch %v", o.prefetch)
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
	var want []string
	for i := 2; i < timeKeep+2; i++ {
		want = append(want, fmt.Sprint(i))
	}
	want = append(want, "p")
	if fmt.Sprint(values) != fmt.Sprint(want) || o.fallback != nil {
		t.Errorf("unexpected kept times %v", values)
	}
	for _, m := range o.allTiles() {
		m.Close()
	}
}

func TestTimeCapacity(t *testing.T) {
	a := newApp(func() {}, Options{
		Maps:     []Map{{Layer: wms.Layer{Name: "None", Type: wms.TypeNone}}},
		Overlays: []Overlay{{Layer: wms.Layer{Name: "A", URL: "https://example.com"}}},
	})
	defer a.closeTiles()
	a.tileCache = 128 // MB, 512 tiles
	newManager := func() *tiles.Manager { return tiles.NewManager(failFetcher{}, tiles.Options{Workers: 1}) }
	o := &a.overlays[0]
	o.tiles = newManager()
	var keys []geo.TileKey
	for x := range 40 {
		keys = append(keys, geo.TileKey{Z: 6, X: x})
	}
	o.tiles.Request(keys)

	// Kept times get twice the visible tiles each.
	for range 2 {
		o.others = append(o.others, &timeTiles{tiles: newManager()})
	}
	a.updateCapacity()
	if c := o.others[0].tiles.Capacity(); c != 80 {
		t.Errorf("kept time capacity %d, want 80", c)
	}
	if c := o.tiles.Capacity(); c != 512-160 {
		t.Errorf("shown capacity %d, want %d", c, 512-160)
	}
	// Together at most half of the cache.
	for range 6 {
		o.others = append(o.others, &timeTiles{tiles: newManager()})
	}
	a.updateCapacity()
	if c := o.others[0].tiles.Capacity(); c != 32 {
		t.Errorf("kept time capacity %d, want 32", c)
	}
	if c := o.tiles.Capacity(); c != 256 {
		t.Errorf("shown capacity %d, want 256", c)
	}
}

// failFetcher fails to load any tile.
type failFetcher struct{}

func (failFetcher) Fetch(context.Context, geo.TileKey) (image.Image, error) {
	return nil, errors.New("no tiles")
}

func TestOverlayReferences(t *testing.T) {
	o := &overlayState{}
	if refs := o.references("2026-10-13T09:00:00.000Z"); refs != nil {
		t.Errorf("expected no reference times without them, got %v", refs)
	}
	o.times.refs = &wms.TimeDimension{Values: "2026-10-12T18:00:00.000Z,2026-10-13T00:00:00.000Z,2026-10-13T06:00:00.000Z,2026-10-13T12:00:00.000Z"}
	// The latest runs not after the time, so that times before the latest run have data.
	want := "[2026-10-13T06:00:00.000Z 2026-10-13T00:00:00.000Z 2026-10-12T18:00:00.000Z]"
	if refs := o.references("2026-10-13T09:00:00.000Z"); fmt.Sprint(refs) != want {
		t.Errorf("got %v, want %s", refs, want)
	}
	if refs := o.references("2026-10-14T09:00:00.000Z"); len(refs) != referenceTries || refs[0] != "2026-10-13T12:00:00.000Z" {
		t.Errorf("expected the latest run first for later times, got %v", refs)
	}
	if refs := o.references(""); refs != nil {
		t.Errorf("expected no reference times for the default time, got %v", refs)
	}
}

func TestOverlayTimeStep(t *testing.T) {
	at := func(d, h int) time.Time { return time.Date(2026, 10, d, h, 0, 0, 0, time.UTC) }
	o := &overlayState{Overlay: Overlay{TimeStep: 6 * time.Hour}}
	o.times.dim = &wms.TimeDimension{Values: "2026-10-12T12:00:00Z/2026-10-21T00:00:00Z/PT3H"}
	for _, tc := range []struct{ target, want time.Time }{
		{at(13, 15), at(13, 18)},
		{at(13, 14), at(13, 12)},
		{at(12, 12), at(12, 12)},
		{at(21, 0), at(21, 0)},
	} {
		if got, ok := o.validTime(tc.target); !ok || !got.Equal(tc.want) {
			t.Errorf("at %v: got %v, %v, want %v", tc.target, got, ok, tc.want)
		}
	}
	// Steps are kept within the valid times.
	o.times.dim = &wms.TimeDimension{Values: "2026-10-13T15:00:00Z/2026-10-13T21:00:00Z/PT3H"}
	if got, ok := o.validTime(at(13, 16)); !ok || !got.Equal(at(13, 18)) {
		t.Errorf("got %v, %v", got, ok)
	}
}

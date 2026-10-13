package ui

import (
	"context"
	"log"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"github.com/mlange-42/tramp/internal/wms"
)

// capabilitiesTimeout limits reading the capabilities of an overlay.
const capabilitiesTimeout = 30 * time.Second

// overlayTimes is the state of an overlay that changes over time, see [Overlay.Refresh].
// WMS overlays read their valid times from the capabilities of the service at each refresh,
// and request the time closest to now. Overlays without a time dimension are just loaded again.
type overlayTimes struct {
	// refreshed is when the overlay was last refreshed.
	refreshed time.Time
	// session is increased when the overlay is enabled, to drop results for an earlier session.
	session int
	// loading is set while the capabilities are read.
	loading bool
	// waiting is set until the time is known after enabling the overlay. The overlay is not drawn meanwhile.
	waiting bool
	// time is the time of the shown data, zero if not known.
	time time.Time
	// reference is the reference time of the shown data, like the start of a model run, if given.
	reference string
}

// timed reports whether the overlay may have valid times in the capabilities of its service.
func (o *overlayState) timed() bool {
	return o.Refresh > 0 && !o.Layer.IsXYZ() && !o.Layer.IsNone()
}

// enabled starts the refresh cycle of an overlay that was just enabled.
func (o *overlayState) enabled(a *App, i int, now time.Time) {
	t := &o.times
	*t = overlayTimes{refreshed: now, session: t.session + 1}
	if o.timed() {
		t.waiting = true
		a.loadOverlayTime(i)
	}
}

// refreshOverlays refreshes the enabled overlays with a refresh interval when due,
// and schedules a frame for the next one.
func (a *App) refreshOverlays(gtx layout.Context) {
	var next time.Time
	for i := range a.overlays {
		o := &a.overlays[i]
		if o.tiles == nil || o.Refresh <= 0 {
			continue
		}
		interval := max(o.Refresh, MinRefresh)
		due := o.times.refreshed.Add(interval)
		if !gtx.Now.Before(due) {
			o.times.refreshed = gtx.Now
			due = gtx.Now.Add(interval)
			if o.timed() {
				a.loadOverlayTime(i)
			} else {
				o.tiles.Refresh()
			}
		}
		if next.IsZero() || due.Before(next) {
			next = due
		}
	}
	if !next.IsZero() {
		gtx.Execute(op.InvalidateCmd{At: next})
	}
}

// loadOverlayTime reads the capabilities of an overlay in the background, and then updates its time.
func (a *App) loadOverlayTime(i int) {
	o := &a.overlays[i]
	if o.times.loading {
		return
	}
	o.times.loading = true
	session := o.times.session
	layer := o.Layer
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), capabilitiesTimeout)
		defer cancel()
		caps, err := a.client.GetCapabilities(ctx, &layer)
		a.runOnUI(func() {
			o := &a.overlays[i]
			if o.tiles == nil || o.times.session != session {
				return
			}
			o.times.loading = false
			if err != nil {
				log.Printf("overlay %s: %v", o.Layer.Name, err)
			}
			o.updateTime(caps, time.Now())
		})
	}()
}

// updateTime requests the time closest to now from the capabilities, and loads the tiles again
// if the time or the reference time changed, or if it can't tell. Caps is nil if reading them failed.
// Overlays without times are requested for the server's default time, and always loaded again.
func (o *overlayState) updateTime(caps *wms.Capabilities, now time.Time) {
	t := &o.times
	first := t.waiting
	t.waiting = false
	if caps == nil {
		// Keep showing the last known time, so that it is clear how old the data is.
		// The first time, the overlay is requested for the server's default time.
		return
	}

	info, _ := caps.Layer(firstLayer(o.Layer.Layers))
	legendURL := ""
	if info != nil {
		legendURL = info.LegendURL
	}
	o.legend.setURL(legendURL)

	var valid time.Time
	ok := false
	if info != nil && info.Time != nil {
		valid, ok = info.Time.Nearest(now)
	}
	if !ok {
		// Without times, the overlay is requested for the server's default time.
		if !t.time.IsZero() {
			o.setTime("")
		}
		t.time, t.reference = time.Time{}, ""
		if !first {
			o.tiles.Refresh()
		}
		return
	}
	changed := !valid.Equal(t.time) || info.Reference != t.reference || info.Reference == ""
	t.time, t.reference = valid, info.Reference
	if !changed {
		return
	}
	o.setTime(wms.FormatTime(valid))
	if !first {
		o.tiles.Refresh()
	}
}

// setTime makes the fetcher of the overlay request the given time.
func (o *overlayState) setTime(value string) {
	layer := o.Layer
	layer.Transparent = true
	layer.Time = value
	o.fetcher.SetLayer(&layer)
}

// firstLayer returns the first name of a comma-separated list of WMS layers.
func firstLayer(layers string) string {
	name, _, _ := strings.Cut(layers, ",")
	return strings.TrimSpace(name)
}

// formatDataTime formats the time of overlay data in local time, with the weekday if it is not today.
func formatDataTime(t, now time.Time) string {
	t, now = t.Local(), now.Local()
	if y, m, d := t.Date(); y == now.Year() && m == now.Month() && d == now.Day() {
		return t.Format("15:04")
	}
	return t.Format("Mon 15:04")
}

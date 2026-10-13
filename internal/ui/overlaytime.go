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
// and request the time closest to the selected time, see [timeControl].
// Overlays without a time dimension are just loaded again.
type overlayTimes struct {
	// refreshed is when the overlay was last refreshed.
	refreshed time.Time
	// session is increased when the overlay is enabled, to drop results for an earlier session.
	session int
	// loading is set while the capabilities are read.
	loading bool
	// waiting is set until the capabilities were read after enabling the overlay. The overlay is not drawn meanwhile.
	waiting bool
	// dim are the valid times from the capabilities, nil if the layer has none.
	dim *wms.TimeDimension
	// reference is the reference time of the data, like the start of a model run, if given.
	reference string
	// time is the requested time, zero if none.
	time time.Time
	// outside is set if the selected time is outside the valid times. The overlay is not drawn then.
	outside bool
}

// hidden reports whether the overlay is not drawn, as it waits for its times or has no data for the selected time.
func (t *overlayTimes) hidden() bool {
	return t.waiting || t.outside
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
			o.updateTime(caps, a.timeCtl.target(time.Now()))
		})
	}()
}

// updateTime takes the valid times from the capabilities, and requests the time closest to target.
// The tiles are loaded again if the time or the reference time changed, or if it can't tell.
// Caps is nil if reading them failed.
func (o *overlayState) updateTime(caps *wms.Capabilities, target time.Time) {
	t := &o.times
	t.waiting = false
	if caps == nil {
		// Keep showing the last known time, so that it is clear how old the data is.
		// The first time, the overlay is requested for the server's default time.
		return
	}

	info, _ := caps.Layer(firstLayer(o.Layer.Layers))
	t.dim = nil
	reference, legendURL := "", ""
	if info != nil {
		t.dim, reference, legendURL = info.Time, info.Reference, info.LegendURL
	}
	o.legend.setURL(legendURL)
	newData := reference == "" || reference != t.reference
	t.reference = reference
	if !o.selectTime(target) && newData {
		o.tiles.Refresh()
	}
}

// selectTime requests the valid time closest to target, and reports whether the tiles are loaded again
// because the time changed. If target is outside the valid times, the overlay is hidden.
// Overlays without times are requested for the server's default time.
func (o *overlayState) selectTime(target time.Time) bool {
	t := &o.times
	t.outside = false
	valid, ok := time.Time{}, false
	if t.dim != nil {
		if first, last, inside := t.dim.Bounds(); inside && (target.Before(first) || target.After(last)) {
			t.outside = true
			return false
		}
		valid, ok = t.dim.Nearest(target)
	}
	if !ok {
		valid = time.Time{}
	}
	if valid.Equal(t.time) {
		return false
	}
	t.time = valid
	value := ""
	if ok {
		value = wms.FormatTime(valid)
	}
	o.setTime(value)
	o.tiles.Refresh()
	return true
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

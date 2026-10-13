package ui

import (
	"context"
	"log"
	"slices"
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
			a.updateTime(o, caps, a.timeCtl.target(time.Now()))
		})
	}()
}

// updateTime takes the valid times from the capabilities, and shows the time closest to target.
// The tiles are loaded again if the reference time changed, or if it can't tell.
// Caps is nil if reading them failed.
func (a *App) updateTime(o *overlayState, caps *wms.Capabilities, target time.Time) {
	t := &o.times
	first := t.waiting
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
	if newData && !first {
		// The kept tiles of other times are outdated too.
		a.dropOtherTimes(o)
		o.tiles.Refresh()
	}
	a.selectTime(o, target, !first)
}

// selectTime shows the valid time of an overlay closest to target, and notes the times
// closest to one step before and after for loading in the background, see [App.prefetchTimes].
// If target is outside the valid times, the overlay is hidden.
// Overlays without times are requested for the server's default time.
// With keep, the tiles of the previous time are kept for going back.
func (a *App) selectTime(o *overlayState, target time.Time, keep bool) {
	t := &o.times
	t.outside, o.prefetch = false, nil
	valid, ok := o.validTime(target)
	if t.dim != nil && !ok {
		t.outside = true
		return
	}
	t.time = valid
	a.showTime(o, timeValue(valid, ok), keep)
	for _, dir := range []int{-1, 1} {
		if v, ok := o.validTime(a.timeCtl.stepFrom(target, dir)); ok {
			if value := timeValue(v, ok); value != o.timeValue && !slices.Contains(o.prefetch, value) {
				o.prefetch = append(o.prefetch, value)
			}
		}
	}
}

// validTime returns the valid time of an overlay closest to target, or false if the overlay has no times,
// or target is outside of them.
func (o *overlayState) validTime(target time.Time) (time.Time, bool) {
	dim := o.times.dim
	if dim == nil {
		return time.Time{}, false
	}
	if first, last, ok := dim.Bounds(); ok && (target.Before(first) || target.After(last)) {
		return time.Time{}, false
	}
	return dim.Nearest(target)
}

// timeValue returns the TIME parameter for a valid time, or "" for the server's default if there is none.
func timeValue(t time.Time, ok bool) string {
	if !ok {
		return ""
	}
	return wms.FormatTime(t)
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

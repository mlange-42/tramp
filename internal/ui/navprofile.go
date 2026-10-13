package ui

import (
	"math"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/routing"
	"github.com/mlange-42/tramp/internal/track"
)

// newNavItem returns the item of a found route, for its profile in the chart.
// It is not part of a file and not drawn with the files, see [App.layoutNavMap].
func (a *App) newNavItem(r *routing.Route) *fileItem {
	pts := make([]track.Point, len(r.Points))
	merc := make([]geo.Point, len(r.Points))
	for i, p := range r.Points {
		pts[i] = track.Point{Pos: p, Ele: math.NaN()}
		merc[i] = geo.ToMercator(p)
	}
	line := mapview.NewPolyline(merc)
	it := &fileItem{
		kind:   navItem,
		name:   "Found route",
		lines:  []mapview.Polyline{line},
		points: [][]track.Point{pts},
		bounds: line.Bounds,
		color:  a.style.NavLine,
	}
	it.visible.Value = true
	return it
}

// setNavItem replaces the item of the found route after the route changed, and shows its profile.
// Without route, the profile is hidden.
func (a *App) setNavItem() {
	n := &a.nav
	old := n.item
	n.item = nil
	if n.route == nil {
		if old != nil && a.selected == old {
			a.hideNavProfile(old)
		}
		return
	}
	n.item = a.newNavItem(n.route)
	if old != nil && a.selected == old {
		// The previous route's profile was shown, so the item before it stays the one to go back to.
		a.selected = n.item
		a.tracksChanged = true
		return
	}
	a.showNavProfile()
}

// showNavProfile selects the found route for the chart, and keeps the selected item for going back to it.
func (a *App) showNavProfile() {
	n := &a.nav
	if n.item == nil || a.selected == n.item {
		return
	}
	n.prevSelected = a.selected
	a.selected = n.item
	a.tracksChanged = true
}

// hideNavProfile selects the item that was selected before the profile of the found route was shown,
// if the route's item it is selected and the item still belongs to an open file.
func (a *App) hideNavProfile(item *fileItem) {
	n := &a.nav
	if item == nil || a.selected != item {
		return
	}
	a.selected = nil
	if prev := n.prevSelected; prev != nil && a.inFiles(prev) {
		a.selected = prev
	}
	n.prevSelected = nil
	a.tracksChanged = true
}

// navClimb returns the ascent and descent of the found route for the panel, or an empty string while not known.
func (n *navigator) navClimb() string {
	if n.item == nil || n.item.profile != profileDone {
		return ""
	}
	return formatClimb(n.item.ascent, n.item.descent)
}

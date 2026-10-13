package ui

import (
	"context"
	"log"
	"math"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/track"
)

// pointsWithoutEle returns the positions of the waypoints and route points without elevation.
func pointsWithoutEle(routes []track.Route, waypoints []track.Waypoint) map[geo.LonLat]bool {
	pos := map[geo.LonLat]bool{}
	add := func(w *track.Waypoint) {
		if math.IsNaN(w.Ele) {
			pos[w.Pos] = true
		}
	}
	for i := range waypoints {
		add(&waypoints[i])
	}
	for _, r := range routes {
		for i := range r.Points {
			add(&r.Points[i])
		}
	}
	return pos
}

// noteNewPoints notes the waypoints and route points without elevation that were not there before a change,
// which are new or moved, for looking up their terrain elevation, see [App.updatePointElevations].
// Points without elevation from before are left as they are.
func (f *openFile) noteNewPoints(before fileSnapshot) {
	old := pointsWithoutEle(before.routes, before.waypoints)
	for p := range pointsWithoutEle(f.data.Routes, f.data.Waypoints) {
		if !old[p] {
			f.edit.needEle = append(f.edit.needEle, p)
		}
	}
}

// updatePointElevations looks up the terrain elevation of the points noted in the edited file
// in the background, see [openFile.noteNewPoints].
func (a *App) updatePointElevations() {
	f := a.editing
	if f == nil || len(f.edit.needEle) == 0 {
		return
	}
	pos := f.edit.needEle
	f.edit.needEle = nil
	svc := a.elev.service
	if svc == nil {
		return
	}
	zoom := svc.Source().ZoomFor(profileResolution, pos[0].Lat)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), profileTimeout)
		defer cancel()
		eles, err := svc.Elevations(ctx, pos, zoom)
		a.runOnUI(func() {
			if err != nil {
				log.Printf("elevation of points: %v", err)
				return
			}
			a.applyPointElevations(f, pos, eles)
		})
	}()
}

// applyPointElevations sets the elevation of the waypoints and route points of the file at the given positions,
// if they still have none. It is not a change that can be undone, as it completes the change that created
// or moved the points; the states saved for undo and redo after it include it.
// A file saved before the elevations arrived is marked as changed, so that they are saved as well.
func (a *App) applyPointElevations(f *openFile, pos []geo.LonLat, eles []float64) {
	byPos := map[geo.LonLat]float64{}
	for i, p := range pos {
		if !math.IsNaN(eles[i]) {
			byPos[p] = eles[i]
		}
	}
	set := false
	fill := func(w *track.Waypoint) {
		if e, ok := byPos[w.Pos]; ok && math.IsNaN(w.Ele) {
			w.Ele, set = e, true
		}
	}
	d := f.data
	for i := range d.Waypoints {
		fill(&d.Waypoints[i])
	}
	for r := range d.Routes {
		for i := range d.Routes[r].Points {
			fill(&d.Routes[r].Points[i])
		}
	}
	if !set {
		return
	}
	if !f.dirty() {
		f.edit.nextRev++
		f.edit.rev = f.edit.nextRev
	}
	f.rebuild()
	a.changed()
}

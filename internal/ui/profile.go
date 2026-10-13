package ui

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/track"
)

// profileState is the state of the elevation profile of a route.
type profileState int

const (
	// profileNone means the profile was not requested yet.
	profileNone profileState = iota
	profileLoading
	profileDone
	profileFailed
)

const (
	// profileSpacing is the largest distance between the points of a route's profile, in meters.
	profileSpacing = 20
	// maxProfilePoints limits the points of a profile, for very long routes.
	maxProfilePoints = 20000
	// profileResolution is the ground resolution of the elevation tiles for profiles, in meters.
	profileResolution = 10
	// climbThreshold is the smallest change of elevation counted for ascent and descent, in meters.
	// Elevation models are smooth, so it only needs to suppress the interpolation noise.
	climbThreshold = 3
	// profileTimeout limits the download of the elevations of a profile.
	profileTimeout = 2 * time.Minute
)

// updateProfiles requests elevation profiles for the routes of the open files that have none yet.
// A route is only drawn between its points, and these rarely have elevations, so the profile samples
// the lines between them from the elevation service. The route's item then shows the samples
// on the map and in the chart, along the same lines.
func (a *App) updateProfiles() {
	if a.elev.service == nil {
		return
	}
	for _, f := range a.files {
		for _, it := range f.items {
			if it.kind == routeItem && it.profile == profileNone {
				a.startProfile(it)
			}
		}
	}
	if it := a.nav.item; it != nil && it.profile == profileNone {
		a.startProfile(it)
	}
	// Tracks have their own points along the way, so their terrain is looked up there:
	// for the track shown in the chart, and for shown tracks without elevation while colored by elevation or slope.
	if it := a.selected; it != nil && it.kind == trackItem && it.profile == profileNone {
		a.startTrackProfile(it)
	}
	if m := a.colorMetric(); m == track.ElevationMetric || m == track.SlopeMetric {
		for _, it := range a.shownItems() {
			if it.kind == trackItem && it.profile == profileNone && it.lineValues(track.ElevationMetric) == nil {
				a.startTrackProfile(it)
			}
		}
	}
}

// startTrackProfile requests the terrain elevation at the points of a track item in the background.
func (a *App) startTrackProfile(it *fileItem) {
	var pos []geo.LonLat
	for _, pts := range it.points {
		for _, p := range pts {
			pos = append(pos, p.Pos)
		}
	}
	if len(pos) < 2 {
		it.profile = profileDone
		return
	}
	it.profile = profileLoading
	src := it.points
	center := geo.ToLonLat(geo.Point{X: (it.bounds.Min.X + it.bounds.Max.X) / 2, Y: (it.bounds.Min.Y + it.bounds.Max.Y) / 2})
	svc := a.elev.service
	zoom := svc.Source().ZoomFor(profileResolution, center.Lat)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), profileTimeout)
		defer cancel()
		eles, err := svc.Elevations(ctx, pos, zoom)
		a.runOnUI(func() { a.applyTrackProfile(it, src, eles, err) })
	}()
}

// applyTrackProfile keeps the terrain elevations eles at the points src of a track item,
// if the item still has these points.
func (a *App) applyTrackProfile(it *fileItem, src [][]track.Point, eles []float64, err error) {
	if len(it.points) != len(src) || len(src) == 0 || &it.points[0] != &src[0] {
		return
	}
	if err != nil {
		log.Printf("terrain elevation of %s: %v", it.name, err)
		it.profile = profileFailed
		return
	}
	it.terrainPts = make([][]track.Point, len(src))
	k := 0
	for i, pts := range src {
		tp := make([]track.Point, len(pts))
		for j, p := range pts {
			tp[j] = track.Point{Pos: p.Pos, Ele: eles[k]}
			k++
		}
		it.terrainPts[i] = tp
	}
	it.terrain = it.segmentValues(it.terrainPts, track.ElevationMetric)
	it.terrainOnly = it.segmentValues(it.points, track.ElevationMetric) == nil
	// The values of elevation and slope change for tracks without recorded elevation.
	it.values = nil
	it.ascent, it.descent = track.Climb(eles, climbThreshold)
	it.profile = profileDone
	a.tracksChanged = true
}

// startProfile requests the elevation profile of a route item in the background.
func (a *App) startProfile(it *fileItem) {
	if len(it.points) != 1 || len(it.points[0]) < 2 {
		it.profile = profileDone
		return
	}
	it.profile = profileLoading
	src := it.points[0]
	pos := make([]geo.LonLat, len(src))
	for i, p := range src {
		pos[i] = p.Pos
	}
	length := track.CumulativeDistance(src)[len(src)-1]
	dense := track.Densify(pos, max(profileSpacing, length/maxProfilePoints))
	center := geo.ToLonLat(geo.Point{X: (it.bounds.Min.X + it.bounds.Max.X) / 2, Y: (it.bounds.Min.Y + it.bounds.Max.Y) / 2})
	svc := a.elev.service
	zoom := svc.Source().ZoomFor(profileResolution, center.Lat)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), profileTimeout)
		defer cancel()
		eles, err := svc.Elevations(ctx, dense, zoom)
		a.runOnUI(func() { a.applyProfile(it, src, dense, eles, err) })
	}()
}

// applyProfile shows the elevation profile of a route item, sampled at the positions dense,
// if the route still has the points src it was requested for.
func (a *App) applyProfile(it *fileItem, src []track.Point, dense []geo.LonLat, eles []float64, err error) {
	if len(it.points) != 1 || len(it.points[0]) == 0 || &it.points[0][0] != &src[0] {
		// The route changed meanwhile, and gets a new profile.
		return
	}
	if err != nil {
		log.Printf("elevation profile of %s: %v", it.name, err)
		it.profile = profileFailed
		a.tracksChanged = true
		return
	}
	pts := make([]track.Point, len(dense))
	merc := make([]geo.Point, len(dense))
	for i, p := range dense {
		pts[i] = track.Point{Pos: p, Ele: eles[i]}
		merc[i] = geo.ToMercator(p)
	}
	it.points = [][]track.Point{pts}
	it.lines = []mapview.Polyline{mapview.NewPolyline(merc)}
	it.clearCaches()
	it.profile = profileDone
	it.ascent, it.descent = track.Climb(eles, climbThreshold)
	it.summary += " · " + formatClimb(it.ascent, it.descent)
	a.tracksChanged = true
}

// formatClimb returns ascent and descent like "↑420 m ↓380 m".
func formatClimb(up, down float64) string {
	return fmt.Sprintf("↑%.0f m ↓%.0f m", up, down)
}

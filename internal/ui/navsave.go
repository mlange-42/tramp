package ui

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/mlange-42/tramp/internal/elevation"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/routing"
	"github.com/mlange-42/tramp/internal/track"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

var (
	iconSaveRoute = mustIcon(icons.EditorLinearScale)
	iconSaveTrack = mustIcon(icons.ContentGesture)
)

// maxDevicePoints is the largest number of points of a route that many handheld devices take.
const maxDevicePoints = 250

// navSavable reports whether the route can be saved: it is found, and not being updated.
func (n *navigator) navSavable() bool {
	return n.route != nil && !n.busy && n.err == nil
}

// navStopNames returns the names of the stops of the route, in route order, empty for stops
// that were not searched. Stops without position are not part of the route.
func (n *navigator) navStopNames() []string {
	var names []string
	for _, s := range n.stops {
		if s.set {
			names = append(names, s.name)
		}
	}
	return names
}

// navRouteName returns the name of the saved route, like "Leipzig → Halle", from the names of start and target.
func navRouteName(stops []string) string {
	from, to := "", ""
	if len(stops) > 0 {
		from, to = strings.TrimSpace(stops[0]), strings.TrimSpace(stops[len(stops)-1])
	}
	if from == "" && to == "" {
		return "Route"
	}
	if from == "" {
		from = "Start"
	}
	if to == "" {
		to = "Target"
	}
	return from + " → " + to
}

// navPointOptions returns the options for the points of saved routes, from the settings.
func (a *App) navPointOptions() routing.PointOptions {
	return routing.PointOptions{Tolerance: a.routingCfg.Tolerance, SlightAngle: a.routingCfg.SlightAngle}
}

// updateNavSaveCount counts the points the route would be saved with, for the save button.
func (a *App) updateNavSaveCount() {
	n := &a.nav
	n.saveCount = 0
	if n.route != nil {
		n.saveCount = len(routing.RoutePoints(n.route, n.navStopNames(), a.navPointOptions(), nil))
	}
}

// navTrackRoute returns the route as a GPX route, with names not used in d yet, which may be nil.
func (a *App) navTrackRoute(d *track.File) track.Route {
	n := &a.nav
	used := map[string]bool{}
	routeNames := map[string]bool{}
	if d != nil {
		for _, w := range d.Waypoints {
			used[w.Name] = true
		}
		for _, r := range d.Routes {
			routeNames[r.Name] = true
			for _, w := range r.Points {
				used[w.Name] = true
			}
		}
	}
	stops := n.navStopNames()
	name := navRouteName(stops)
	for i, base := 2, name; routeNames[name]; i++ {
		name = fmt.Sprintf("%s (%d)", base, i)
	}
	return track.Route{
		Name:   name,
		Points: routing.RoutePoints(n.route, stops, a.navPointOptions(), used),
	}
}

// saveNavRoute saves the route as a GPX route: into the edited file, where it can be undone,
// or otherwise into a new file, which is opened for editing.
func (a *App) saveNavRoute() {
	n := &a.nav
	if !n.navSavable() {
		return
	}
	if f := a.editing; f != nil {
		r := a.navTrackRoute(f.data)
		f.change(func(d *track.File) { d.Routes = append(d.Routes, r) })
		a.changed()
		f.visible.Value = true
		a.selectRoute(f, len(f.data.Routes)-1)
		return
	}
	r := a.navTrackRoute(nil)
	data := &track.File{Format: track.FormatGPX, Creator: track.Creator, Routes: []track.Route{r}}
	svc := a.elev.service
	// In the edited file, the points get their elevation like other new points, see [openFile.noteNewPoints].
	a.createFileFrom("Save route", fileName(r.Name)+".gpx", func() (*track.File, error) {
		addRouteElevations(svc, &data.Routes[0])
		return data, nil
	}, func(f *openFile) {
		a.selectRoute(f, 0)
	})
}

// addRouteElevations sets the terrain elevation of the points of a route, if svc is not nil.
// A failed download is logged, and leaves the route without elevations.
func addRouteElevations(svc *elevation.Service, r *track.Route) {
	pos := make([]geo.LonLat, len(r.Points))
	for i, p := range r.Points {
		pos[i] = p.Pos
	}
	for i, e := range lookupElevations(svc, pos) {
		r.Points[i].Ele = e
	}
}

// addTrackElevations sets the terrain elevation of the points of a track, like [addRouteElevations].
func addTrackElevations(svc *elevation.Service, t *track.Track) {
	var pos []geo.LonLat
	for _, s := range t.Segments {
		for _, p := range s.Points {
			pos = append(pos, p.Pos)
		}
	}
	eles := lookupElevations(svc, pos)
	if eles == nil {
		return
	}
	k := 0
	for i := range t.Segments {
		for j := range t.Segments[i].Points {
			t.Segments[i].Points[j].Ele = eles[k]
			k++
		}
	}
}

// lookupElevations returns the terrain elevations at positions, or nil if svc is nil or the download failed,
// which is logged. It waits for the download, so it must not run on the UI goroutine.
func lookupElevations(svc *elevation.Service, pos []geo.LonLat) []float64 {
	if svc == nil || len(pos) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), profileTimeout)
	defer cancel()
	eles, err := svc.Elevations(ctx, pos, svc.Source().ZoomFor(profileResolution, pos[0].Lat))
	if err != nil {
		log.Printf("terrain elevation: %v", err)
		return nil
	}
	return eles
}

// navTrackFile returns the route as a GPX file with a track of all points of the route.
func (a *App) navTrackFile() *track.File {
	n := &a.nav
	seg := track.Segment{Points: make([]track.Point, len(n.route.Points))}
	for i, p := range n.route.Points {
		seg.Points[i] = track.Point{Pos: p, Ele: math.NaN()}
	}
	return &track.File{
		Format:  track.FormatGPX,
		Creator: track.Creator,
		Tracks:  []track.Track{{Name: navRouteName(n.navStopNames()), Segments: []track.Segment{seg}}},
	}
}

// saveNavTrack saves the route as a GPX track with all its points, in a new file, which is opened.
// Files with tracks are not edited, so it never goes into the edited file.
func (a *App) saveNavTrack() {
	if !a.nav.navSavable() {
		return
	}
	data := a.navTrackFile()
	svc := a.elev.service
	a.createFileFrom("Save track", fileName(data.Tracks[0].Name)+".gpx", func() (*track.File, error) {
		addTrackElevations(svc, &data.Tracks[0])
		return data, nil
	}, func(f *openFile) {
		for _, it := range f.items {
			if it.kind == trackItem {
				a.selected = it
				a.tracksChanged = true
				break
			}
		}
	})
}

// fileName returns name without the characters that are not allowed in file names on some systems,
// and with the arrow of route names as a dash.
func fileName(name string) string {
	name = strings.ReplaceAll(name, " → ", " - ")
	name = strings.Map(func(r rune) rune {
		if r < ' ' || strings.ContainsRune(`<>:"/\|?*`, r) {
			return -1
		}
		return r
	}, name)
	if name = strings.Trim(name, " ."); name == "" {
		return "Route"
	}
	return name
}

// selectRoute selects route r of file f, for the chart.
func (a *App) selectRoute(f *openFile, r int) {
	if it := f.routeItem(r); it != nil {
		a.selected = it
		a.tracksChanged = true
	}
}

// navSaveHelp returns the tooltip of the save button.
func (a *App) navSaveHelp() string {
	n := &a.nav
	if !n.navSavable() {
		return "Save as GPX route, once the route is found"
	}
	where := "a new file"
	if a.editing != nil {
		where = a.editing.name
	}
	return fmt.Sprintf("Save as GPX route in %s, with %s", where, count(n.saveCount, "point"))
}

// navSaveTrackHelp returns the tooltip of the button for saving the route as track.
func (a *App) navSaveTrackHelp() string {
	n := &a.nav
	if !n.navSavable() {
		return "Save as GPX track, once the route is found"
	}
	return fmt.Sprintf("Save as GPX track in a new file, with all %s", count(len(n.route.Points), "point"))
}

// navSaveWarning returns a warning about the number of points of the saved route, or an empty string.
func (a *App) navSaveWarning() string {
	n := &a.nav
	if !n.navSavable() || n.saveCount <= maxDevicePoints {
		return ""
	}
	return fmt.Sprintf("Saved as route, this has %d points. Many devices take at most %d.", n.saveCount, maxDevicePoints)
}

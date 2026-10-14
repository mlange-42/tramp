package ui

import (
	"fmt"
	"regexp"

	"github.com/mlange-42/tramp/internal/routing"
	"github.com/mlange-42/tramp/internal/track"
)

// nameRouteHelp is the tooltip of the button for naming route points.
const nameRouteHelp = "Name the route points by number and turn, like \"01 Start\", \"02 SL\" (slight left), \"03\" and \"04 End\". " +
	"Points linked to waypoints and points with other names keep their name. " +
	"The names are updated when the route is changed"

var (
	// autoNameRe matches generated route point names, like "01 Start", "03", "04 SL" or "05 End",
	// with the number added for unique names, like "03 (2)".
	autoNameRe = regexp.MustCompile(`^\d{2,}(?: (?:Start|End|SL|SR|L|R|HL|HR|U))?(?: \(\d+\))?$`)
	// autoEndRe matches the generated names of the first and last point, which mark a route as named automatically.
	autoEndRe = regexp.MustCompile(`^\d{2,} (?:Start|End)(?: \(\d+\))?$`)
)

// autoNamed reports whether the points of a route are named automatically:
// its start or end has a generated name, see [autoRouteNames].
func autoNamed(r *track.Route) bool {
	for _, p := range r.Points {
		if autoEndRe.MatchString(p.Name) {
			return true
		}
	}
	return false
}

// autoRouteNames returns the generated names of the points of route r, like "01 Start", "02 SL", "03" and "04 End":
// the number of the point, and the turn at it, see [routing.PointTurnCode].
// Points linked to a waypoint and points with other names keep their name.
// The names are unique among all points of the file, as devices merge points with the same name.
// changed reports whether any name differs from the current one.
func autoRouteNames(d *track.File, r int, slightAngle float64) (names []string, changed bool) {
	pts := d.Routes[r].Points
	links := linkedRoutePoints(d)
	keep := func(i int) bool {
		return links[vertex{route: r, point: i}] || (pts[i].Name != "" && !autoNameRe.MatchString(pts[i].Name))
	}
	used := map[string]bool{}
	for _, w := range d.Waypoints {
		used[w.Name] = true
	}
	for ri, rt := range d.Routes {
		for i, p := range rt.Points {
			if ri != r || keep(i) {
				used[p.Name] = true
			}
		}
	}
	width := max(2, len(fmt.Sprint(len(pts))))
	names = make([]string, len(pts))
	for i, p := range pts {
		if keep(i) {
			names[i] = p.Name
			continue
		}
		name := fmt.Sprintf("%0*d", width, i+1)
		code := ""
		switch i {
		case 0:
			code = "Start"
		case len(pts) - 1:
			code = "End"
		default:
			code = routing.PointTurnCode(pts[i-1].Pos, p.Pos, pts[i+1].Pos, slightAngle)
		}
		if code != "" {
			name += " " + code
		}
		unique := name
		for n := 2; used[unique]; n++ {
			unique = fmt.Sprintf("%s (%d)", name, n)
		}
		used[unique] = true
		names[i] = unique
		changed = changed || unique != p.Name
	}
	return names, changed
}

// nameRoutePoints sets the generated names of the points of route r, see [autoRouteNames].
func nameRoutePoints(d *track.File, r int, slightAngle float64) {
	names, _ := autoRouteNames(d, r, slightAngle)
	for i, n := range names {
		d.Routes[r].Points[i].Name = n
	}
}

// updateAutoNames renames the points of the automatically named routes after a change,
// so that numbers and turns stay up to date, and new points get names.
func updateAutoNames(d *track.File, slightAngle float64) {
	for r := range d.Routes {
		if autoNamed(&d.Routes[r]) {
			nameRoutePoints(d, r, slightAngle)
		}
	}
}

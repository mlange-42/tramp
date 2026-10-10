package ui

import (
	"math"

	"gioui.org/f32"
	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/track"
)

// linkTol is the largest difference of coordinates, in degrees, for linked points.
const linkTol = 1e-6

// linked reports whether a route point is linked to a waypoint: both have the same name and position.
// Devices that build routes from stored waypoints, like many handhelds, write their routes like this.
// Linked points are moved together.
func linked(w, rp *track.Waypoint) bool {
	return w.Name != "" && w.Name == rp.Name &&
		math.Abs(w.Pos.Lat-rp.Pos.Lat) <= linkTol && math.Abs(w.Pos.Lon-rp.Pos.Lon) <= linkTol
}

// waypointAt returns the vertex, if it is a waypoint, or otherwise the waypoint the route point is linked to.
// It returns -1 if there is none.
func waypointAt(d *track.File, v vertex) int {
	if v.route < 0 {
		return v.point
	}
	rp := &d.Routes[v.route].Points[v.point]
	for i := range d.Waypoints {
		if linked(&d.Waypoints[i], rp) {
			return i
		}
	}
	return -1
}

// linkedGroup returns the vertex together with the vertices linked to it:
// a waypoint and all route points linked to it.
func linkedGroup(d *track.File, v vertex) []vertex {
	wi := waypointAt(d, v)
	if wi < 0 {
		return []vertex{v}
	}
	w := &d.Waypoints[wi]
	group := []vertex{{route: -1, point: wi}}
	for r := range d.Routes {
		for i := range d.Routes[r].Points {
			if linked(w, &d.Routes[r].Points[i]) {
				group = append(group, vertex{route: r, point: i})
			}
		}
	}
	return group
}

// linkedRoutePoints returns the route points that are linked to a waypoint.
func linkedRoutePoints(d *track.File) map[vertex]bool {
	byName := map[string][]int{}
	for i, w := range d.Waypoints {
		if w.Name != "" {
			byName[w.Name] = append(byName[w.Name], i)
		}
	}
	res := map[vertex]bool{}
	if len(byName) == 0 {
		return res
	}
	for r := range d.Routes {
		for i := range d.Routes[r].Points {
			rp := &d.Routes[r].Points[i]
			for _, wi := range byName[rp.Name] {
				if linked(&d.Waypoints[wi], rp) {
					res[vertex{route: r, point: i}] = true
					break
				}
			}
		}
	}
	return res
}

// nearestVertex returns the vertex nearest to the screen position p, within radius pixels.
// prio ranks vertices at the same place, lower first; vertices with negative priority are skipped.
func nearestVertex(d *track.File, view *mapview.View, p f32.Point, radius float32, prio func(v vertex) int) (vertex, bool) {
	// Vertices closer than this in pixels count as being at the same place.
	const same = 1
	bestDist, bestPrio, found := radius, 0, false
	var hit vertex
	check := func(v vertex, pt *track.Waypoint) {
		pr := prio(v)
		if pr < 0 {
			return
		}
		dist := float32(math.Sqrt(float64(dist2(screenPos(view, pt.Pos), p))))
		if dist > radius {
			return
		}
		if !found || dist < bestDist-same || dist <= bestDist+same && pr < bestPrio {
			bestDist, bestPrio, hit, found = dist, pr, v, true
		}
	}
	for i := range d.Waypoints {
		check(vertex{route: -1, point: i}, &d.Waypoints[i])
	}
	for r := range d.Routes {
		for i := range d.Routes[r].Points {
			check(vertex{route: r, point: i}, &d.Routes[r].Points[i])
		}
	}
	return hit, found
}

// hitVertex returns the waypoint or route point nearest to the screen position p, within radius pixels.
// Of points at the same place, like linked points, a point of route prefer comes first,
// then a waypoint, then points of other routes.
func hitVertex(d *track.File, view *mapview.View, p f32.Point, radius float32, prefer int) (vertex, bool) {
	return nearestVertex(d, view, p, radius, func(v vertex) int {
		switch {
		case v.route >= 0 && v.route == prefer:
			return 0
		case v.route < 0:
			return 1
		}
		return 2
	})
}

// hitRouteEnd returns the first or last point of a route nearest to the screen position p, within radius pixels.
// Of points at the same place, an end of route prefer comes first.
func hitRouteEnd(d *track.File, view *mapview.View, p f32.Point, radius float32, prefer int) (vertex, bool) {
	return nearestVertex(d, view, p, radius, func(v vertex) int {
		if v.route < 0 || v.point != 0 && v.point != len(d.Routes[v.route].Points)-1 {
			return -1
		}
		if v.route == prefer {
			return 0
		}
		return 1
	})
}

package routing

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/track"
)

// mergeDistance is the distance along the route, in meters, within which turns become one route point,
// like crossing a road with a small offset.
const mergeDistance = 30.0

// routePoint is a point of a route to save, while selecting them.
type routePoint struct {
	// index is the index in [Route.Points].
	index int
	// stop is the number of the start, via point or destination, or -1 for other points.
	stop int
	// kinds are the turns at the point, more than one for merged turns, and none for points
	// added to keep the route near its path.
	kinds  []TurnKind
	street string
	exit   int
}

// PointOptions are the options of [RoutePoints].
type PointOptions struct {
	// Tolerance is the largest distance in meters between the route and the straight lines between its points.
	// Zero or less adds no points for it.
	Tolerance float64
	// SlightAngle is the smallest change of direction in degrees for which a slight turn gets a point.
	// Zero or less leaves out all slight turns.
	SlightAngle float64
}

// keepTurn reports whether a turn needs a route point. Following the road, also under a new name, doesn't,
// and neither do slight turns that change the direction less than slightAngle degrees.
// Slight turns at forks are [TurnKeepLeft] or [TurnKeepRight], and always kept.
func keepTurn(t Turn, slightAngle float64) bool {
	switch t.Kind {
	case TurnContinue:
		return false
	case TurnSlightLeft, TurnSlightRight:
		return slightAngle > 0 && math.Abs(t.Angle) >= slightAngle
	}
	return true
}

// isStop reports whether a turn is the start, a via point, or the destination.
func isStop(k TurnKind) bool {
	return k == TurnStart || k == TurnVia || k == TurnEnd
}

// RoutePoints returns the points of a GPX route that follows r with as few points as possible,
// for navigating with a handheld device:
//
//   - the start, the via points and the destination,
//   - turns, forks and roundabouts, but no points where the route follows the road,
//     and only slight turns that change the direction by at least opts.SlightAngle degrees,
//   - one point for turns closer than 30 m, at the first of them,
//   - and points where the straight lines between the other points would be farther than opts.Tolerance meters
//     from the route.
//
// stops are the names of the start, the via points and the destination, in route order.
// Missing or empty names become "Start", "Via 1", ... and "End". The other points are named with a number,
// a short code of the turn and the street, like "03 L Waldweg", and describe the turn in full.
// All names are unique, as devices merge points with the same name. used are the names taken already,
// like those of other routes in the same file. Names are added to it. It may be nil.
func RoutePoints(r *Route, stops []string, opts PointOptions, used map[string]bool) []track.Waypoint {
	if len(r.Points) < 2 {
		return nil
	}
	dist := make([]float64, len(r.Points))
	for i := 1; i < len(r.Points); i++ {
		dist[i] = dist[i-1] + geo.Distance(r.Points[i-1], r.Points[i])
	}

	var points []routePoint
	nStops := 0
	// The turns are fixed on a copy, for routes not made by a router.
	for _, t := range fixTurns(slices.Clone(r.Turns), len(r.Points)) {
		if !keepTurn(t, opts.SlightAngle) {
			continue
		}
		p := routePoint{index: t.Index, stop: -1, kinds: []TurnKind{t.Kind}, street: t.Street, exit: t.Exit}
		if isStop(t.Kind) {
			p.stop = nStops
			p.kinds = nil
			nStops++
		}
		if len(points) == 0 {
			points = append(points, p)
			continue
		}
		last := &points[len(points)-1]
		switch {
		case dist[p.index]-dist[last.index] >= mergeDistance || (last.stop >= 0 && p.stop >= 0):
			points = append(points, p)
		case last.stop >= 0:
			// A turn right after a stop is left to the stop.
		case p.stop >= 0:
			// A turn right before a stop is replaced by the stop.
			*last = p
		default:
			last.kinds = append(last.kinds, p.kinds...)
			last.street, last.exit = p.street, max(last.exit, p.exit)
		}
	}

	if opts.Tolerance > 0 {
		var all []routePoint
		for i, p := range points {
			if i > 0 {
				all = append(all, refine(r, points[i-1].index, p.index, opts.Tolerance)...)
			}
			all = append(all, p)
		}
		points = all
	}
	return namePoints(r, points, stops, nStops, used)
}

// refine returns points between the points at indices from and to, so that the straight lines between them
// are within tolerance of the route. It adds the point farthest from the line, and repeats on both sides,
// like the Douglas-Peucker algorithm.
func refine(r *Route, from, to int, tolerance float64) []routePoint {
	if to-from < 2 {
		return nil
	}
	a, b := r.Points[from], r.Points[to]
	far, farDist := -1, tolerance
	for i := from + 1; i < to; i++ {
		if d := segmentDistance(r.Points[i], a, b); d > farDist {
			far, farDist = i, d
		}
	}
	if far < 0 {
		return nil
	}
	p := routePoint{index: far, stop: -1, street: streetAt(r, far)}
	return append(append(refine(r, from, far, tolerance), p), refine(r, far, to, tolerance)...)
}

// segmentDistance returns the distance of p from the line from a to b, in meters.
// It projects the positions to a plane at a, which is good enough for the short lines of routes.
func segmentDistance(p, a, b geo.LonLat) float64 {
	const meanRadius = 6371008.8
	scale := meanRadius * math.Pi / 180
	cos := math.Cos(a.Lat * math.Pi / 180)
	px, py := (p.Lon-a.Lon)*cos*scale, (p.Lat-a.Lat)*scale
	bx, by := (b.Lon-a.Lon)*cos*scale, (b.Lat-a.Lat)*scale
	t := 0.0
	if l2 := bx*bx + by*by; l2 > 0 {
		t = min(max((px*bx+py*by)/l2, 0), 1)
	}
	return math.Hypot(px-t*bx, py-t*by)
}

// streetAt returns the street of the route at point i: the street of the last turn at or before it.
func streetAt(r *Route, i int) string {
	street := ""
	for _, t := range r.Turns {
		if t.Index > i {
			break
		}
		if t.Kind != TurnVia && t.Kind != TurnEnd {
			street = t.Street
		}
	}
	return street
}

// namePoints converts the selected points to waypoints with unique names and descriptions.
func namePoints(r *Route, points []routePoint, stops []string, nStops int, used map[string]bool) []track.Waypoint {
	width := len(fmt.Sprint(len(points)))
	if width < 2 {
		width = 2
	}
	if used == nil {
		used = map[string]bool{}
	}
	wps := make([]track.Waypoint, len(points))
	for i, p := range points {
		w := track.Waypoint{Pos: r.Points[p.index], Ele: math.NaN()}
		if p.stop >= 0 {
			w.Name = stopName(stops, p.stop, nStops)
		} else {
			parts := []string{fmt.Sprintf("%0*d", width, i+1)}
			for _, s := range []string{turnCode(p), p.street} {
				if s != "" {
					parts = append(parts, s)
				}
			}
			w.Name = strings.Join(parts, " ")
			w.Desc = turnDesc(p)
		}
		w.Name = uniqueName(w.Name, used)
		wps[i] = w
	}
	return wps
}

// stopName returns the name of stop i of n, or its default name.
func stopName(stops []string, i, n int) string {
	if i < len(stops) {
		if s := strings.TrimSpace(stops[i]); s != "" {
			return s
		}
	}
	switch i {
	case 0:
		return "Start"
	case n - 1:
		return "End"
	}
	return fmt.Sprintf("Via %d", i)
}

// uniqueName returns name, or name with a number if it is used already, and marks it as used.
func uniqueName(name string, used map[string]bool) string {
	unique := name
	for n := 2; used[unique]; n++ {
		unique = fmt.Sprintf("%s (%d)", name, n)
	}
	used[unique] = true
	return unique
}

// turnCodes are the short codes of turns in route point names.
var turnCodes = map[TurnKind]string{
	TurnSlightLeft:  "SL",
	TurnLeft:        "L",
	TurnSharpLeft:   "HL",
	TurnSlightRight: "SR",
	TurnRight:       "R",
	TurnSharpRight:  "HR",
	TurnUTurn:       "U",
	TurnKeepLeft:    "KL",
	TurnKeepRight:   "KR",
	TurnRoundabout:  "RB",
	TurnFerry:       "FERRY",
}

// turnCode returns the short code of the turns at a point, like "L", "L-R" for merged turns,
// or "RB2" for the second exit of a roundabout.
func turnCode(p routePoint) string {
	codes := make([]string, len(p.kinds))
	for i, k := range p.kinds {
		codes[i] = turnCodes[k]
		if k == TurnRoundabout && p.exit > 0 {
			codes[i] += fmt.Sprint(p.exit)
		}
	}
	return strings.Join(codes, "-")
}

// turnPhrases are the descriptions of turns.
var turnPhrases = map[TurnKind]string{
	TurnSlightLeft:  "turn slightly left",
	TurnSlightRight: "turn slightly right",
	TurnLeft:        "turn left",
	TurnSharpLeft:   "turn sharp left",
	TurnRight:       "turn right",
	TurnSharpRight:  "turn sharp right",
	TurnUTurn:       "make a U-turn",
	TurnKeepLeft:    "keep left",
	TurnKeepRight:   "keep right",
	TurnRoundabout:  "enter the roundabout",
	TurnFerry:       "board the ferry",
}

// turnDesc returns the description of the turns at a point, like "Turn left, then right onto Waldweg".
func turnDesc(p routePoint) string {
	if len(p.kinds) == 0 {
		if p.street == "" {
			return ""
		}
		return "Follow " + p.street
	}
	phrases := make([]string, len(p.kinds))
	for i, k := range p.kinds {
		phrases[i] = turnPhrases[k]
		if k == TurnRoundabout && p.exit > 0 {
			phrases[i] = fmt.Sprintf("take exit %d at the roundabout", p.exit)
		}
		// "then turn right" reads better as "then right".
		if i > 0 {
			phrases[i] = strings.TrimPrefix(phrases[i], "turn ")
		}
	}
	desc := strings.Join(phrases, ", then ")
	if p.street != "" {
		desc += " onto " + p.street
	}
	return strings.ToUpper(desc[:1]) + desc[1:]
}

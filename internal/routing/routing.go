// Package routing finds routes between positions with online routing services.
package routing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/httpx"
)

// Routing services.
const (
	// ServiceValhalla is the Valhalla routing engine, the default.
	ServiceValhalla = "valhalla"
	// ServiceOSRM is the Open Source Routing Machine.
	ServiceOSRM = "osrm"
)

// Profiles are the means of travel a route is planned for.
const (
	ProfileFoot = "foot"
	ProfileBike = "bike"
	ProfileCar  = "car"
)

// Profiles are all profiles, the default first.
var Profiles = []string{ProfileFoot, ProfileBike, ProfileCar}

// dataAttribution is the attribution of the OpenStreetMap data, as required by its license.
const dataAttribution = "Data © OpenStreetMap contributors (ODbL)"

// FixTheMapURL is where users report errors in the OpenStreetMap data.
// The terms of the FOSSGIS servers require a link to it with the routes.
const FixTheMapURL = "https://www.openstreetmap.org/fixthemap"

// interval is the shortest time between two requests.
// The terms of the FOSSGIS servers allow at most one request per second.
const interval = time.Second

// Router finds routes.
type Router interface {
	// Route returns the route through the points of the request, in order.
	Route(ctx context.Context, r Request) (*Route, error)
	// Attribution is shown with routes, as required by the data license.
	Attribution() string
}

// Request is a route to find.
type Request struct {
	// Points are the start, any intermediate points, and the destination. At least two.
	Points []geo.LonLat
	// Profile is one of [Profiles]. Empty means [ProfileFoot].
	Profile string
	// Alternatives is the largest number of alternative routes wanted, besides the best one.
	// The services find alternatives only for routes without intermediate points, and may find fewer or none.
	Alternatives int
}

// alternatives returns the number of alternatives to ask the service for: none with intermediate points.
func (r Request) alternatives() int {
	if len(r.Points) != 2 {
		return 0
	}
	return max(0, r.Alternatives)
}

// Route is a found route.
type Route struct {
	// Points are the positions along the route.
	Points []geo.LonLat
	// Distance is the length of the route, in meters.
	Distance float64
	// Duration is the estimated travel time.
	Duration time.Duration
	// Turns are the points where the route starts, changes direction or road, passes a via point, and ends,
	// in route order. The first is a [TurnStart] at index 0, and the last a [TurnEnd] at the last point.
	Turns []Turn
	// Alternatives are other routes between the same points, if asked for and found.
	// They are set only for the best route.
	Alternatives []*Route
}

// Turn is a point of a route where the traveler has to decide where to go, or could have to.
type Turn struct {
	// Index is the index of the point in [Route.Points].
	Index int
	Kind  TurnKind
	// Street is the name of the road or path the route follows after the turn, if it has one.
	Street string
	// Exit is the number of the exit to take, for a [TurnRoundabout]. Zero if not known.
	Exit int
}

// TurnKind is the kind of a [Turn].
type TurnKind int

// Kinds of turns.
const (
	// TurnContinue follows the road or path, maybe under a new name.
	TurnContinue TurnKind = iota
	TurnSlightLeft
	TurnLeft
	TurnSharpLeft
	TurnSlightRight
	TurnRight
	TurnSharpRight
	TurnUTurn
	// TurnKeepLeft and TurnKeepRight take the left or right branch of a fork.
	TurnKeepLeft
	TurnKeepRight
	// TurnRoundabout enters a roundabout, see [Turn.Exit].
	TurnRoundabout
	// TurnFerry boards a ferry.
	TurnFerry
	// TurnStart and TurnEnd are the start and the destination, and TurnVia an intermediate point of the request.
	TurnStart
	TurnVia
	TurnEnd
)

var turnNames = [...]string{"continue", "slight left", "left", "sharp left", "slight right", "right", "sharp right",
	"u-turn", "keep left", "keep right", "roundabout", "ferry", "start", "via", "end"}

func (k TurnKind) String() string {
	if k < 0 || int(k) >= len(turnNames) {
		return "unknown"
	}
	return turnNames[k]
}

// fixTurns sorts the turns by index, and makes sure that they start with a [TurnStart] at the first point
// and end with a [TurnEnd] at the last point, as services may leave them out.
func (r *Route) fixTurns() {
	r.Turns = fixTurns(r.Turns, len(r.Points))
}

// fixTurns does the work of [Route.fixTurns], for a route with n points. It changes turns in place.
func fixTurns(turns []Turn, n int) []Turn {
	last := n - 1
	for i := range turns {
		turns[i].Index = min(max(turns[i].Index, 0), last)
	}
	slices.SortStableFunc(turns, func(a, b Turn) int { return a.Index - b.Index })
	if len(turns) == 0 || turns[0].Kind != TurnStart {
		turns = slices.Insert(turns, 0, Turn{Kind: TurnStart})
	}
	turns[0].Index = 0
	if turns[len(turns)-1].Kind != TurnEnd {
		turns = append(turns, Turn{Kind: TurnEnd})
	}
	turns[len(turns)-1].Index = last
	return turns
}

// New returns the router for a service, [ServiceValhalla] if empty, with requests throttled
// to one per second, and one at a time.
// url is the endpoint of the service, or empty for the public FOSSGIS server of the service.
// userAgent identifies the application, as the services require.
func New(service, url string, client *http.Client, userAgent string) (Router, error) {
	var r Router
	switch strings.ToLower(strings.TrimSpace(service)) {
	case "", ServiceValhalla:
		r = &Valhalla{URL: url, Client: httpx.Client{HTTP: client, UserAgent: userAgent}}
	case ServiceOSRM:
		r = &OSRM{URL: url, Client: httpx.Client{HTTP: client, UserAgent: userAgent}}
	default:
		return nil, fmt.Errorf("unknown routing service %q, expected %s or %s", service, ServiceValhalla, ServiceOSRM)
	}
	return NewThrottle(r, interval), nil
}

// check returns an error if the request has too few points.
func (r Request) check() error {
	if len(r.Points) < 2 {
		return errors.New("routing needs at least two points")
	}
	return nil
}

// profile returns the profile, or [ProfileFoot] if empty.
func (r Request) profile() string {
	if r.Profile == "" {
		return ProfileFoot
	}
	return r.Profile
}

// Throttle passes requests to a router one at a time, with a minimum time between their starts.
// It is safe for concurrent use.
type Throttle struct {
	Router
	interval time.Duration

	// sem holds a token while a request waits or runs.
	sem  chan struct{}
	last time.Time
}

// NewThrottle returns a throttle for r, with the given minimum time between requests.
func NewThrottle(r Router, interval time.Duration) *Throttle {
	return &Throttle{Router: r, interval: interval, sem: make(chan struct{}, 1)}
}

// Route waits until no other request runs and the interval has passed, and then routes with the router.
func (t *Throttle) Route(ctx context.Context, r Request) (*Route, error) {
	select {
	case t.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-t.sem }()

	if wait := time.Until(t.last.Add(t.interval)); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	t.last = time.Now()
	return t.Router.Route(ctx, r)
}

// decodePolyline decodes a line in the Encoded Polyline Algorithm Format, with the given precision
// as a power of ten: 5 for Google and OSRM, 6 for Valhalla.
// See https://developers.google.com/maps/documentation/utilities/polylinealgorithm.
func decodePolyline(s string, precision int) ([]geo.LonLat, error) {
	factor := 1.0
	for range precision {
		factor *= 10
	}
	var points []geo.LonLat
	var lat, lon int64
	for i := 0; i < len(s); {
		var deltas [2]int64
		for j := range deltas {
			var result int64
			var shift uint
			for {
				if i >= len(s) {
					return nil, errors.New("truncated polyline")
				}
				b := int64(s[i]) - 63
				i++
				if b < 0 || shift > 60 {
					return nil, errors.New("invalid polyline")
				}
				result |= (b & 0x1f) << shift
				shift += 5
				if b < 0x20 {
					break
				}
			}
			if result&1 != 0 {
				deltas[j] = ^(result >> 1)
			} else {
				deltas[j] = result >> 1
			}
		}
		lat += deltas[0]
		lon += deltas[1]
		points = append(points, geo.LonLat{Lon: float64(lon) / factor, Lat: float64(lat) / factor})
	}
	return points, nil
}

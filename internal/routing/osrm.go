package routing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/httpx"
)

// OSRMURL is the address of the public OSRM servers of FOSSGIS e.V., with a placeholder for the profile.
// Its terms allow at most one request per second, and require an identifying user agent,
// see https://routing.openstreetmap.de/about.html.
const OSRMURL = "https://routing.openstreetmap.de/routed-" + ProfilePlaceholder

// ProfilePlaceholder in an OSRM address is replaced by the profile: foot, bike or car.
// OSRM serves one profile per server, so the public servers have one address per profile.
const ProfilePlaceholder = "{profile}"

// OSRMAttribution is shown with routes, as required by the data license.
const OSRMAttribution = "Routing by OSRM · " + dataAttribution

// OSRM finds routes with OSRM servers.
type OSRM struct {
	// URL is the address of the server, without the /route/v1 path.
	// [ProfilePlaceholder] in it is replaced by the profile. Empty means [OSRMURL].
	URL string
	httpx.Client
}

type osrmResponse struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Routes  []osrmRoute `json:"routes"`
}

type osrmRoute struct {
	Geometry struct {
		// Coordinates are longitude and latitude.
		Coordinates [][]float64 `json:"coordinates"`
	} `json:"geometry"`
	Legs []struct {
		Steps []osrmStep `json:"steps"`
	} `json:"legs"`
	// Distance is in meters, and Duration in seconds.
	Distance float64 `json:"distance"`
	Duration float64 `json:"duration"`
}

type osrmStep struct {
	// Name is the name of the road or path of the step.
	Name     string `json:"name"`
	Maneuver struct {
		Type     string `json:"type"`
		Modifier string `json:"modifier"`
		// Location is longitude and latitude.
		Location []float64 `json:"location"`
		// Exit is the number of the exit to take from a roundabout.
		Exit int `json:"exit"`
		// BearingBefore and BearingAfter are the directions before and after the maneuver,
		// in degrees clockwise from north. Zero for depart and arrive.
		BearingBefore float64 `json:"bearing_before"`
		BearingAfter  float64 `json:"bearing_after"`
	} `json:"maneuver"`
}

// osrmModifiers are the turn kinds of the direction modifiers of OSRM.
// "straight" and unknown modifiers mean [TurnContinue].
var osrmModifiers = map[string]TurnKind{
	"uturn":        TurnUTurn,
	"sharp right":  TurnSharpRight,
	"right":        TurnRight,
	"slight right": TurnSlightRight,
	"slight left":  TurnSlightLeft,
	"left":         TurnLeft,
	"sharp left":   TurnSharpLeft,
}

// kind returns the turn kind of the step, other than for its start and end.
// See https://project-osrm.org/docs/v5.24.0/api/#stepmaneuver-object.
func (s *osrmStep) kind() TurnKind {
	switch s.Maneuver.Type {
	case "new name", "merge", "notification", "use lane", "exit roundabout", "exit rotary":
		return TurnContinue
	case "roundabout", "rotary":
		return TurnRoundabout
	case "fork", "on ramp", "off ramp":
		switch {
		case strings.Contains(s.Maneuver.Modifier, "left"):
			return TurnKeepLeft
		case strings.Contains(s.Maneuver.Modifier, "right"):
			return TurnKeepRight
		}
		return TurnContinue
	}
	// "turn", "end of road", "continue" and "roundabout turn" go where the modifier says.
	return osrmModifiers[s.Maneuver.Modifier]
}

// Attribution returns [OSRMAttribution].
func (o *OSRM) Attribution() string { return OSRMAttribution }

// Route returns the route through the points of the request.
func (o *OSRM) Route(ctx context.Context, r Request) (*Route, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	profile := r.profile()
	if !slices.Contains(Profiles, profile) {
		return nil, fmt.Errorf("unknown routing profile %q", r.Profile)
	}
	base := o.URL
	if base == "" {
		base = OSRMURL
	}
	base = strings.TrimSuffix(strings.ReplaceAll(base, ProfilePlaceholder, profile), "/")

	coords := make([]string, len(r.Points))
	for i, p := range r.Points {
		coords[i] = strconv.FormatFloat(p.Lon, 'f', 6, 64) + "," + strconv.FormatFloat(p.Lat, 'f', 6, 64)
	}
	// The profile in the path is ignored by OSRM, which serves the profile it was set up with.
	endpoint := base + "/route/v1/driving/" + strings.Join(coords, ";") + "?overview=full&geometries=geojson&steps=true"
	if n := r.alternatives(); n > 0 {
		endpoint += "&alternatives=" + strconv.Itoa(n)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	var res osrmResponse
	if err := o.DoJSON(req, &res, osrmError); err != nil {
		return nil, fmt.Errorf("routing failed: %w", err)
	}
	if res.Code != "Ok" {
		return nil, fmt.Errorf("routing failed: %s", osrmMessage(res.Code, res.Message))
	}
	if len(res.Routes) == 0 {
		return nil, errors.New("routing failed: no route found")
	}
	route, err := res.Routes[0].route()
	if err != nil {
		return nil, err
	}
	for _, alt := range res.Routes[1:] {
		// A broken alternative doesn't spoil the route.
		if a, err := alt.route(); err == nil {
			route.Alternatives = append(route.Alternatives, a)
		}
	}
	return route, nil
}

// route converts a route of the response.
func (or *osrmRoute) route() (*Route, error) {
	route := &Route{
		Distance: or.Distance,
		Duration: time.Duration(or.Duration * float64(time.Second)),
	}
	for _, c := range or.Geometry.Coordinates {
		if len(c) >= 2 {
			route.Points = append(route.Points, geo.LonLat{Lon: c[0], Lat: c[1]})
		}
	}
	if len(route.Points) < 2 {
		return nil, errors.New("routing failed: empty route")
	}
	// The steps don't refer to the points of the route, so their points are searched along it.
	from := 0
	for i, leg := range or.Legs {
		for _, s := range leg.Steps {
			if len(s.Maneuver.Location) < 2 {
				continue
			}
			turn := Turn{Kind: s.kind(), Street: s.Name}
			switch s.Maneuver.Type {
			case "depart":
				if i > 0 {
					// The via point was added with the arrival of the previous leg.
					continue
				}
				turn.Kind = TurnStart
			case "arrive":
				turn.Kind = TurnVia
				if i == len(or.Legs)-1 {
					turn.Kind = TurnEnd
				}
				// The street at the arrival is the one of the step before.
				turn.Street = ""
			}
			if turn.Kind == TurnRoundabout {
				turn.Exit = s.Maneuver.Exit
			}
			if !isStop(turn.Kind) {
				turn.Angle = turnAngle(s.Maneuver.BearingBefore, s.Maneuver.BearingAfter)
			}
			loc := geo.LonLat{Lon: s.Maneuver.Location[0], Lat: s.Maneuver.Location[1]}
			turn.Index = pointIndex(route.Points, loc, from)
			from = turn.Index
			route.Turns = append(route.Turns, turn)
		}
	}
	route.fixTurns()
	return route, nil
}

// pointIndex returns the index of the first point at or after from that equals p,
// or of the nearest point at or after from if none does.
func pointIndex(points []geo.LonLat, p geo.LonLat, from int) int {
	best, bestDist := from, math.Inf(1)
	for i := from; i < len(points); i++ {
		if points[i] == p {
			return i
		}
		// Degrees are good enough to find the nearest point.
		dx, dy := points[i].Lon-p.Lon, points[i].Lat-p.Lat
		if d := dx*dx + dy*dy; d < bestDist {
			best, bestDist = i, d
		}
	}
	return best
}

// osrmError returns the message of an error response, like "Query string malformed close to position 49".
func osrmError(body []byte) string {
	var res osrmResponse
	_ = json.Unmarshal(body, &res)
	if res.Code == "" && res.Message == "" {
		return ""
	}
	return osrmMessage(res.Code, res.Message)
}

// osrmMessage returns the message, or the code if there is none.
func osrmMessage(code, message string) string {
	if message != "" {
		return message
	}
	return code
}

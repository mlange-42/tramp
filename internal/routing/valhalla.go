package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/mlange-42/tramp/internal/httpx"
)

// ValhallaURL is the route endpoint of the public Valhalla server of FOSSGIS e.V.
// Its terms allow at most one request per second, and require an identifying user agent,
// see https://www.fossgis.de/arbeitsgruppen/osm-server/nutzungsbedingungen/.
// It limits the length of routes, to 100 km on foot and 150 km by bike.
const ValhallaURL = "https://valhalla1.openstreetmap.de/route"

// ValhallaAttribution is shown with routes, as required by the data license.
const ValhallaAttribution = "Routing by Valhalla · " + dataAttribution

// valhallaCosting are the costing models of Valhalla by profile.
var valhallaCosting = map[string]string{
	ProfileFoot: "pedestrian",
	ProfileBike: "bicycle",
	ProfileCar:  "auto",
}

// Valhalla finds routes with a Valhalla server.
type Valhalla struct {
	// URL is the route endpoint. Empty means [ValhallaURL].
	URL string
	httpx.Client
}

type valhallaLocation struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type valhallaRequest struct {
	Locations []valhallaLocation `json:"locations"`
	Costing   string             `json:"costing"`
	Units     string             `json:"units"`
	// DirectionsType "maneuvers" asks for the turns without the instruction texts.
	DirectionsType string `json:"directions_type"`
	// Alternates is the number of alternative routes wanted.
	Alternates int `json:"alternates,omitempty"`
}

type valhallaResponse struct {
	Trip       valhallaTrip `json:"trip"`
	Alternates []struct {
		Trip valhallaTrip `json:"trip"`
	} `json:"alternates"`
}

type valhallaTrip struct {
	Legs []struct {
		// Shape is the line of the leg as an encoded polyline with precision 6.
		Shape     string             `json:"shape"`
		Maneuvers []valhallaManeuver `json:"maneuvers"`
	} `json:"legs"`
	Summary struct {
		// Length is in kilometers, and Time in seconds.
		Length float64 `json:"length"`
		Time   float64 `json:"time"`
	} `json:"summary"`
}

type valhallaManeuver struct {
	Type        int      `json:"type"`
	StreetNames []string `json:"street_names"`
	// BeginShapeIndex is the index of the maneuver's point in the shape of the leg.
	BeginShapeIndex     int `json:"begin_shape_index"`
	RoundaboutExitCount int `json:"roundabout_exit_count"`
	// BearingBefore and BearingAfter are the directions before and after the maneuver,
	// in degrees clockwise from north. Pointers, as the start has none before, and the end none after.
	BearingBefore *float64 `json:"bearing_before"`
	BearingAfter  *float64 `json:"bearing_after"`
}

// valhallaStart and valhallaDestination are the maneuver types at the start and the end of a leg:
// straight ahead, or with the target to the right or left.
var (
	valhallaStart       = []int{1, 2, 3}
	valhallaDestination = []int{4, 5, 6}
)

// valhallaTurns are the turn kinds of the other maneuver types of Valhalla.
// Types not listed, like "becomes", merging, leaving a roundabout or a ferry, and transit, mean [TurnContinue].
// See https://valhalla.github.io/valhalla/api/turn-by-turn/api-reference/#trip-legs-and-maneuvers.
var valhallaTurns = map[int]TurnKind{
	9:  TurnSlightRight,
	10: TurnRight,
	11: TurnSharpRight,
	12: TurnUTurn, // right
	13: TurnUTurn, // left
	14: TurnSharpLeft,
	15: TurnLeft,
	16: TurnSlightLeft,
	18: TurnKeepRight, // ramp
	19: TurnKeepLeft,  // ramp
	20: TurnKeepRight, // exit
	21: TurnKeepLeft,  // exit
	23: TurnKeepRight, // stay right
	24: TurnKeepLeft,  // stay left
	26: TurnRoundabout,
	28: TurnFerry,
}

// Attribution returns [ValhallaAttribution].
func (v *Valhalla) Attribution() string { return ValhallaAttribution }

// Route returns the route through the points of the request.
func (v *Valhalla) Route(ctx context.Context, r Request) (*Route, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	costing, ok := valhallaCosting[r.profile()]
	if !ok {
		return nil, fmt.Errorf("unknown routing profile %q", r.Profile)
	}
	body := valhallaRequest{Costing: costing, Units: "kilometers", DirectionsType: "maneuvers", Alternates: r.alternatives()}
	for _, p := range r.Points {
		body.Locations = append(body.Locations, valhallaLocation{Lat: p.Lat, Lon: p.Lon})
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	endpoint := v.URL
	if endpoint == "" {
		endpoint = ValhallaURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	var res valhallaResponse
	if err := v.DoJSON(req, &res, valhallaError); err != nil {
		return nil, fmt.Errorf("routing failed: %w", err)
	}

	route, err := res.Trip.route()
	if err != nil {
		return nil, err
	}
	for _, alt := range res.Alternates {
		// A broken alternative doesn't spoil the route.
		if a, err := alt.Trip.route(); err == nil {
			route.Alternatives = append(route.Alternatives, a)
		}
	}
	return route, nil
}

// route converts a trip, joining the lines and the maneuvers of its legs.
func (t *valhallaTrip) route() (*Route, error) {
	route := &Route{
		Distance: t.Summary.Length * 1000,
		Duration: time.Duration(t.Summary.Time * float64(time.Second)),
	}
	for i, leg := range t.Legs {
		points, err := decodePolyline(leg.Shape, 6)
		if err != nil {
			return nil, fmt.Errorf("routing failed: %w", err)
		}
		// offset is the index of the first point of the leg in the route.
		offset := len(route.Points)
		// Each leg starts where the previous one ends.
		if i > 0 && len(points) > 0 {
			points = points[1:]
			offset--
		}
		route.Points = append(route.Points, points...)
		for _, m := range leg.Maneuvers {
			turn := Turn{Index: offset + m.BeginShapeIndex, Kind: valhallaTurns[m.Type]}
			switch {
			case slices.Contains(valhallaStart, m.Type):
				if i > 0 {
					// The via point was added with the end of the previous leg.
					continue
				}
				turn.Kind = TurnStart
			case slices.Contains(valhallaDestination, m.Type):
				turn.Kind = TurnVia
				if i == len(t.Legs)-1 {
					turn.Kind = TurnEnd
				}
			case turn.Kind == TurnRoundabout:
				turn.Exit = m.RoundaboutExitCount
			}
			if len(m.StreetNames) > 0 {
				turn.Street = m.StreetNames[0]
			}
			if m.BearingBefore != nil && m.BearingAfter != nil && !isStop(turn.Kind) {
				turn.Angle = turnAngle(*m.BearingBefore, *m.BearingAfter)
			}
			route.Turns = append(route.Turns, turn)
		}
	}
	if len(route.Points) < 2 {
		return nil, errors.New("routing failed: empty route")
	}
	route.fixTurns()
	return route, nil
}

// valhallaError returns the message of an error response, like
// "Path distance exceeds the max distance limit: 100000 meters".
func valhallaError(body []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &e)
	return e.Error
}

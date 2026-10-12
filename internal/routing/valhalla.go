package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
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
	// HTTP is the client for requests. Nil means [http.DefaultClient].
	HTTP *http.Client
	// UserAgent identifies the application.
	UserAgent string
}

type valhallaLocation struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type valhallaRequest struct {
	Locations []valhallaLocation `json:"locations"`
	Costing   string             `json:"costing"`
	Units     string             `json:"units"`
	// DirectionsType "none" leaves out the turn-by-turn instructions.
	DirectionsType string `json:"directions_type"`
}

type valhallaResponse struct {
	Trip struct {
		Legs []struct {
			// Shape is the line of the leg as an encoded polyline with precision 6.
			Shape string `json:"shape"`
		} `json:"legs"`
		Summary struct {
			// Length is in kilometers, and Time in seconds.
			Length float64 `json:"length"`
			Time   float64 `json:"time"`
		} `json:"summary"`
	} `json:"trip"`
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
	body := valhallaRequest{Costing: costing, Units: "kilometers", DirectionsType: "none"}
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
	if err := doJSON(v.HTTP, req, v.UserAgent, &res, valhallaError); err != nil {
		return nil, err
	}

	route := &Route{
		Distance: res.Trip.Summary.Length * 1000,
		Duration: time.Duration(res.Trip.Summary.Time * float64(time.Second)),
	}
	for i, leg := range res.Trip.Legs {
		points, err := decodePolyline(leg.Shape, 6)
		if err != nil {
			return nil, fmt.Errorf("routing failed: %w", err)
		}
		// Each leg starts where the previous one ends.
		if i > 0 && len(points) > 0 {
			points = points[1:]
		}
		route.Points = append(route.Points, points...)
	}
	if len(route.Points) < 2 {
		return nil, errors.New("routing failed: empty route")
	}
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

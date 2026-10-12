package routing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
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
	// HTTP is the client for requests. Nil means [http.DefaultClient].
	HTTP *http.Client
	// UserAgent identifies the application.
	UserAgent string
}

type osrmResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Routes  []struct {
		Geometry struct {
			// Coordinates are longitude and latitude.
			Coordinates [][]float64 `json:"coordinates"`
		} `json:"geometry"`
		// Distance is in meters, and Duration in seconds.
		Distance float64 `json:"distance"`
		Duration float64 `json:"duration"`
	} `json:"routes"`
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
	endpoint := base + "/route/v1/driving/" + strings.Join(coords, ";") + "?overview=full&geometries=geojson&steps=false"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	var res osrmResponse
	if err := doJSON(o.HTTP, req, o.UserAgent, &res, osrmError); err != nil {
		return nil, err
	}
	if res.Code != "Ok" {
		return nil, fmt.Errorf("routing failed: %s", osrmMessage(res.Code, res.Message))
	}
	if len(res.Routes) == 0 {
		return nil, errors.New("routing failed: no route found")
	}
	best := res.Routes[0]
	route := &Route{
		Distance: best.Distance,
		Duration: time.Duration(best.Duration * float64(time.Second)),
	}
	for _, c := range best.Geometry.Coordinates {
		if len(c) >= 2 {
			route.Points = append(route.Points, geo.LonLat{Lon: c[0], Lat: c[1]})
		}
	}
	if len(route.Points) < 2 {
		return nil, errors.New("routing failed: empty route")
	}
	return route, nil
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

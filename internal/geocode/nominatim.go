package geocode

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/httpx"
)

// NominatimURL is the search endpoint of the public Nominatim service of OpenStreetMap.
// Its usage policy allows at most one request per second, and requires an identifying user agent,
// see https://operations.osmfoundation.org/policies/nominatim/.
const NominatimURL = "https://nominatim.openstreetmap.org/search"

// NominatimAttribution is shown with search results, as required by the data license.
const NominatimAttribution = "Search by Nominatim · © OpenStreetMap contributors"

// nominatimInterval is the shortest time between two requests.
const nominatimInterval = time.Second

// Nominatim searches places with a Nominatim service.
// It is safe for concurrent use, and waits between requests as required by the usage policy.
// The policy also requires caching results, see [Cache], and switching the server without
// a software update, which the configurable URL allows.
type Nominatim struct {
	// URL is the search endpoint. Empty means [NominatimURL].
	URL string
	httpx.Client

	mu   sync.Mutex
	last time.Time
}

// nominatimPlace is a result in the JSON format of Nominatim.
type nominatimPlace struct {
	DisplayName string `json:"display_name"`
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
	Type        string `json:"type"`
	// BoundingBox is min latitude, max latitude, min longitude, max longitude.
	BoundingBox []string `json:"boundingbox"`
}

// Attribution returns [NominatimAttribution].
func (n *Nominatim) Attribution() string { return NominatimAttribution }

// Interactive returns false: the usage policy forbids searching while the user types.
func (n *Nominatim) Interactive() bool { return false }

// Search returns up to q.Limit places matching the query, best matches first.
// q.Near is not used.
func (n *Nominatim) Search(ctx context.Context, query Query) ([]Place, error) {
	if err := n.wait(ctx); err != nil {
		return nil, err
	}
	endpoint := n.URL
	if endpoint == "" {
		endpoint = NominatimURL
	}
	q := url.Values{"q": {query.Text}, "format": {"jsonv2"}, "limit": {strconv.Itoa(query.Limit)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var raw []nominatimPlace
	if err := n.DoJSON(req, &raw, nil); err != nil {
		return nil, err
	}
	places := make([]Place, 0, len(raw))
	for _, r := range raw {
		if p, ok := r.place(); ok {
			places = append(places, p)
		}
	}
	return places, nil
}

// wait blocks until a request is allowed, and reserves it.
func (n *Nominatim) wait(ctx context.Context) error {
	n.mu.Lock()
	next := n.last.Add(nominatimInterval)
	now := time.Now()
	if next.Before(now) {
		next = now
	}
	n.last = next
	n.mu.Unlock()

	t := time.NewTimer(time.Until(next))
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// place converts a result, and returns false if its position is invalid.
func (r nominatimPlace) place() (Place, bool) {
	lat, err1 := strconv.ParseFloat(r.Lat, 64)
	lon, err2 := strconv.ParseFloat(r.Lon, 64)
	if err1 != nil || err2 != nil {
		return Place{}, false
	}
	p := Place{Name: r.DisplayName, Kind: r.Type, Pos: geo.LonLat{Lon: lon, Lat: lat}}
	if len(r.BoundingBox) == 4 {
		var b [4]float64
		ok := true
		for i, s := range r.BoundingBox {
			v, err := strconv.ParseFloat(s, 64)
			b[i], ok = v, ok && err == nil
		}
		if ok && b[0] <= b[1] && b[2] <= b[3] {
			p.Min, p.Max, p.HasBounds = geo.LonLat{Lon: b[2], Lat: b[0]}, geo.LonLat{Lon: b[3], Lat: b[1]}, true
		}
	}
	return p, true
}

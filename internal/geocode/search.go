package geocode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/mlange-42/tramp/internal/geo"
)

// Search services.
const (
	// ServiceNominatim is the search service of OpenStreetMap, the default.
	ServiceNominatim = "nominatim"
	// ServicePhoton is the Photon search service by komoot.
	ServicePhoton = "photon"
)

// Searcher finds places by name.
type Searcher interface {
	// Search returns up to q.Limit places matching the query, best matches first.
	Search(ctx context.Context, q Query) ([]Place, error)
	// Attribution is shown with the results, as required by the data license.
	Attribution() string
	// Interactive reports whether the service allows searching while the user types.
	Interactive() bool
}

// Query is a search for places.
type Query struct {
	// Text is the name to search for.
	Text string
	// Limit is the maximum number of places.
	Limit int
	// Near prefers places near this position, if HasNear, within about the area of a map
	// at zoom level Zoom around it. Zero means the default of the service. Not all services support it.
	Near    geo.LonLat
	HasNear bool
	Zoom    int
}

// key returns the cache key of the query. Case and spaces of the text don't change the results,
// and positions closer than about a kilometer neither.
func (q Query) key() string {
	k := strconv.Itoa(q.Limit) + "|" + strings.ToLower(strings.Join(strings.Fields(q.Text), " "))
	if q.HasNear {
		k += fmt.Sprintf("|%.2f,%.2f,%d", q.Near.Lon, q.Near.Lat, q.Zoom)
	}
	return k
}

// Place is a search result.
type Place struct {
	// Name is the full name of the place, like "Leipzig, Sachsen, Deutschland".
	Name string
	// Kind is the type of the place, like "city" or "peak".
	Kind string
	// Pos is the position of the place.
	Pos geo.LonLat
	// Min and Max are the south-west and north-east corners of the extent of the place, if HasBounds.
	Min, Max  geo.LonLat
	HasBounds bool
}

// New returns the searcher for a service, [ServiceNominatim] if empty, with results cached.
// url is the search endpoint, or empty for the public server of the service.
// userAgent identifies the application, as the services require.
func New(service, url string, client *http.Client, userAgent string) (Searcher, error) {
	var s Searcher
	switch strings.ToLower(strings.TrimSpace(service)) {
	case "", ServiceNominatim:
		s = &Nominatim{URL: url, HTTP: client, UserAgent: userAgent}
	case ServicePhoton:
		s = &Photon{URL: url, HTTP: client, UserAgent: userAgent}
	default:
		return nil, fmt.Errorf("unknown search service %q, expected %s or %s", service, ServiceNominatim, ServicePhoton)
	}
	return NewCache(s, defaultCacheSize), nil
}

// defaultCacheSize is the number of searches kept by the searchers from [New].
const defaultCacheSize = 100

// Cache keeps the results of recent searches, so that repeated searches don't reach the service again.
// Nominatim requires caching. It is safe for concurrent use.
type Cache struct {
	Searcher
	size int

	mu sync.Mutex
	// results are the cached places by query, and keys the queries, oldest first.
	results map[string][]Place
	keys    []string
}

// NewCache returns a cache for the results of s, keeping the given number of searches.
func NewCache(s Searcher, size int) *Cache {
	return &Cache{Searcher: s, size: size, results: map[string][]Place{}}
}

// Search returns cached results for the query, or searches with the cached searcher.
// Failed searches are not cached.
func (c *Cache) Search(ctx context.Context, q Query) ([]Place, error) {
	key := q.key()
	c.mu.Lock()
	places, ok := c.results[key]
	c.mu.Unlock()
	if ok {
		return slices.Clone(places), nil
	}

	places, err := c.Searcher.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.results[key]; !ok {
		if len(c.keys) >= c.size {
			delete(c.results, c.keys[0])
			c.keys = c.keys[1:]
		}
		c.keys = append(c.keys, key)
	}
	c.results[key] = slices.Clone(places)
	return places, nil
}

// getJSON sends a request with the user agent, and decodes the JSON response into v.
func getJSON(client *http.Client, req *http.Request, userAgent string, v any) error {
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("search failed: %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("search failed: %w", err)
	}
	return nil
}

package geocode

import (
	"context"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/httpx"
)

// PhotonURL is the search endpoint of the public Photon service of komoot.
// Its terms ask for fair use; extensive use is throttled, see https://photon.komoot.io.
const PhotonURL = "https://photon.komoot.io/api"

// PhotonAttribution is shown with search results, as required by the license of the OpenStreetMap data.
const PhotonAttribution = "Search by Photon · © OpenStreetMap contributors"

// photonBiasScale is how much the importance of places still counts against their distance
// when searching near a position, from 0 to 1. Photon's default of 0.4 lists only streets
// named like Berlin when searching "Berlin" in Leipzig; 0.6 lists the city first,
// and still nearby places for common names like "Hauptbahnhof".
const photonBiasScale = "0.6"

// Photon searches places with a Photon service.
type Photon struct {
	// URL is the search endpoint. Empty means [PhotonURL].
	URL string
	httpx.Client
}

// photonResult is the GeoJSON response of Photon.
type photonResult struct {
	Features []struct {
		Geometry struct {
			// Coordinates are longitude and latitude.
			Coordinates []float64 `json:"coordinates"`
		} `json:"geometry"`
		Properties photonProperties `json:"properties"`
	} `json:"features"`
}

type photonProperties struct {
	Name        string `json:"name"`
	HouseNumber string `json:"housenumber"`
	Street      string `json:"street"`
	Postcode    string `json:"postcode"`
	City        string `json:"city"`
	District    string `json:"district"`
	County      string `json:"county"`
	State       string `json:"state"`
	Country     string `json:"country"`
	OSMValue    string `json:"osm_value"`
	// Extent is the extent of the place as two corners, longitude and latitude each.
	Extent []float64 `json:"extent"`
}

// Attribution returns [PhotonAttribution].
func (p *Photon) Attribution() string { return PhotonAttribution }

// Interactive returns true: Photon is made for searching while the user types.
func (p *Photon) Interactive() bool { return true }

// Search returns up to q.Limit places matching the query, best matches first,
// preferring places near q.Near.
func (p *Photon) Search(ctx context.Context, query Query) ([]Place, error) {
	endpoint := p.URL
	if endpoint == "" {
		endpoint = PhotonURL
	}
	q := url.Values{"q": {query.Text}, "limit": {strconv.Itoa(query.Limit)}}
	if query.HasNear {
		q.Set("lat", strconv.FormatFloat(query.Near.Lat, 'f', 5, 64))
		q.Set("lon", strconv.FormatFloat(query.Near.Lon, 'f', 5, 64))
		if query.Zoom > 0 {
			q.Set("zoom", strconv.Itoa(query.Zoom))
		}
		q.Set("location_bias_scale", photonBiasScale)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var res photonResult
	if err := p.DoJSON(req, &res, nil); err != nil {
		return nil, err
	}
	places := make([]Place, 0, len(res.Features))
	for _, f := range res.Features {
		c := f.Geometry.Coordinates
		if len(c) < 2 {
			continue
		}
		pr := f.Properties
		pl := Place{Name: pr.displayName(), Kind: pr.OSMValue, Pos: geo.LonLat{Lon: c[0], Lat: c[1]}}
		if e := pr.Extent; len(e) == 4 {
			// The order of the corners is not documented, so take the minimum and maximum.
			pl.Min = geo.LonLat{Lon: math.Min(e[0], e[2]), Lat: math.Min(e[1], e[3])}
			pl.Max = geo.LonLat{Lon: math.Max(e[0], e[2]), Lat: math.Max(e[1], e[3])}
			pl.HasBounds = true
		}
		places = append(places, pl)
	}
	return places, nil
}

// displayName joins the name and the address parts of a place, like
// "Thomaskirche, Thomaskirchhof 18, 04109 Leipzig, Sachsen, Deutschland".
// Repeated parts, like a city named like its state, are left out.
func (pr photonProperties) displayName() string {
	street := strings.TrimSpace(pr.Street + " " + pr.HouseNumber)
	city := strings.TrimSpace(pr.Postcode + " " + pr.City)
	var parts []string
	for _, s := range []string{pr.Name, street, pr.District, city, pr.County, pr.State, pr.Country} {
		if s == "" {
			continue
		}
		dup := false
		for _, p := range parts {
			// A city part like "04109 Leipzig" repeats a name "Leipzig".
			if p == s || strings.HasSuffix(s, " "+p) {
				dup = true
				break
			}
		}
		if !dup {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}

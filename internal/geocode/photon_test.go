package geocode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mlange-42/tramp/internal/geo"
)

// photonSample is a shortened response of the public Photon server.
const photonSample = `{"type":"FeatureCollection","features":[
	{"type":"Feature","properties":{"osm_key":"historic","osm_value":"church","type":"house","housenumber":"18",
	 "name":"Thomaskirche","street":"Thomaskirchhof","district":"Zentrum","city":"Leipzig","state":"Sachsen",
	 "country":"Deutschland","postcode":"04109","extent":[12.3720494,51.3395019,12.373234,51.3391598]},
	 "geometry":{"type":"Point","coordinates":[12.3725965,51.3393315]}},
	{"type":"Feature","properties":{"osm_key":"place","osm_value":"city","type":"city","name":"Leipzig",
	 "state":"Sachsen","country":"Deutschland","extent":[12.2366519,51.4481145,12.5424918,51.2381704]},
	 "geometry":{"type":"Point","coordinates":[12.3747329,51.3406321]}},
	{"type":"Feature","properties":{"name":"Broken"},"geometry":{"type":"Point","coordinates":[]}}
]}`

func TestPhotonSearch(t *testing.T) {
	var gotQuery, gotLimit, gotAgent, gotLat, gotLon, gotZoom string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := r.URL.Query()
		gotQuery, gotLimit, gotAgent = v.Get("q"), v.Get("limit"), r.UserAgent()
		gotLat, gotLon, gotZoom = v.Get("lat"), v.Get("lon"), v.Get("zoom")
		_, _ = w.Write([]byte(photonSample))
	}))
	defer srv.Close()

	p := &Photon{URL: srv.URL, UserAgent: "test-agent"}
	q := Query{Text: "Thomaskirche", Limit: 5, Near: geo.LonLat{Lon: 12.37, Lat: 51.34}, HasNear: true, Zoom: 14}
	places, err := p.Search(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "Thomaskirche" || gotLimit != "5" || gotAgent != "test-agent" || gotLat != "51.34000" || gotLon != "12.37000" || gotZoom != "14" {
		t.Errorf("request: query %q, limit %q, agent %q, near %s %s zoom %s", gotQuery, gotLimit, gotAgent, gotLat, gotLon, gotZoom)
	}
	if len(places) != 2 {
		t.Fatalf("expected 2 places, got %d", len(places))
	}
	church := places[0]
	if church.Name != "Thomaskirche, Thomaskirchhof 18, Zentrum, 04109 Leipzig, Sachsen, Deutschland" || church.Kind != "church" {
		t.Errorf("unexpected place %+v", church)
	}
	if church.Pos.Lon != 12.3725965 || church.Pos.Lat != 51.3393315 {
		t.Errorf("unexpected position %v", church.Pos)
	}
	city := places[1]
	if city.Name != "Leipzig, Sachsen, Deutschland" {
		t.Errorf("unexpected name %q", city.Name)
	}
	if !city.HasBounds || city.Min.Lon != 12.2366519 || city.Min.Lat != 51.2381704 ||
		city.Max.Lon != 12.5424918 || city.Max.Lat != 51.4481145 {
		t.Errorf("unexpected bounds %+v", city)
	}
}

func TestPhotonDisplayName(t *testing.T) {
	// The city part repeats the name, and is left out.
	pr := photonProperties{Name: "Leipzig", Postcode: "04109", City: "Leipzig", Country: "Deutschland"}
	if got := pr.displayName(); got != "Leipzig, Deutschland" {
		t.Errorf("got %q", got)
	}
}

package routing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/httpx"
)

// osrmSample is a response of the public OSRM foot server for a short walk in Leipzig,
// with the geometry shortened.
const osrmSample = `{"code":"Ok","routes":[{"legs":[{"steps":[],"weight":22.06,"summary":"","duration":208.2,"distance":260.4}],
	"weight_name":"routability","geometry":{"coordinates":[[12.373117,51.339701],[12.373115,51.339716],
	[12.375729,51.339902],[12.375852,51.340513]],"type":"LineString"},"weight":22.06,"duration":208.2,"distance":260.4}],
	"waypoints":[{"hint":"","distance":4.4,"name":"","location":[12.373117,51.339701]},
	{"hint":"","distance":17.5,"name":"","location":[12.375852,51.340513]}]}`

func TestOSRMRoute(t *testing.T) {
	var gotPath, gotQuery, gotAgent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotAgent = r.URL.Path, r.URL.RawQuery, r.UserAgent()
		_, _ = w.Write([]byte(osrmSample))
	}))
	defer srv.Close()

	o := &OSRM{URL: srv.URL + "/routed-" + ProfilePlaceholder + "/", Client: httpx.Client{UserAgent: "test-agent"}}
	req := Request{Points: []geo.LonLat{{Lon: 12.3731, Lat: 51.3397}, {Lon: 12.376, Lat: 51.3405}}}
	route, err := o.Route(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/routed-foot/route/v1/driving/12.373100,51.339700;12.376000,51.340500" ||
		gotQuery != "overview=full&geometries=geojson&steps=true" || gotAgent != "test-agent" {
		t.Errorf("unexpected request %s?%s, agent %q", gotPath, gotQuery, gotAgent)
	}
	if route.Distance != 260.4 || route.Duration != 208200*time.Millisecond {
		t.Errorf("unexpected distance %v or duration %v", route.Distance, route.Duration)
	}
	want := []geo.LonLat{{Lon: 12.373117, Lat: 51.339701}, {Lon: 12.373115, Lat: 51.339716},
		{Lon: 12.375729, Lat: 51.339902}, {Lon: 12.375852, Lat: 51.340513}}
	if len(route.Points) != len(want) {
		t.Fatalf("expected %v, got %v", want, route.Points)
	}
	for i, p := range route.Points {
		if p != want[i] {
			t.Errorf("point %d: expected %v, got %v", i, want[i], p)
		}
	}
}

func TestOSRMErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusBadRequest, `{"message":"Query string malformed close to position 49","code":"InvalidQuery"}`, "Query string malformed"},
		{http.StatusBadRequest, `{"code":"NoRoute"}`, "NoRoute"},
		{http.StatusBadGateway, `<html>Bad Gateway</html>`, "502"},
		{http.StatusOK, `{"code":"Ok","routes":[]}`, "no route"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		o := &OSRM{URL: srv.URL}
		_, err := o.Route(context.Background(), Request{Points: make([]geo.LonLat, 2), Profile: ProfileCar})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("response %s: expected error with %q, got %v", tc.body, tc.want, err)
		}
		srv.Close()
	}

	o := &OSRM{URL: "http://localhost:1"}
	if _, err := o.Route(context.Background(), Request{Points: make([]geo.LonLat, 2), Profile: "horse"}); err == nil {
		t.Error("expected error for unknown profile")
	}
}

func TestOSRMAlternatives(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"code":"Ok","routes":[
			{"geometry":{"coordinates":[[0,0],[0,1]]},"distance":100,"duration":60},
			{"geometry":{"coordinates":[[0,0],[1,1],[0,1]]},"distance":150,"duration":80},
			{"geometry":{"coordinates":[]},"distance":1,"duration":1}]}`))
	}))
	defer srv.Close()

	o := &OSRM{URL: srv.URL}
	route, err := o.Route(context.Background(), Request{Points: make([]geo.LonLat, 2), Alternatives: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(gotQuery, "&alternatives=2") {
		t.Errorf("expected alternatives asked for, got %q", gotQuery)
	}
	if route.Distance != 100 || len(route.Alternatives) != 1 || route.Alternatives[0].Distance != 150 ||
		len(route.Alternatives[0].Points) != 3 {
		t.Errorf("expected one alternative, got %+v", route.Alternatives)
	}

	if _, err := o.Route(context.Background(), Request{Points: make([]geo.LonLat, 3), Alternatives: 2}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(gotQuery, "alternatives") {
		t.Errorf("expected no alternatives asked for with intermediate points, got %q", gotQuery)
	}
}

func TestOSRMTurns(t *testing.T) {
	// Two legs from (0, 0) over (1, 1) to (2, 2).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"Ok","routes":[{"geometry":{"coordinates":[[0,0],[0,1],[1,1],[2,1],[2,2]]},
			"legs":[{"steps":[
				{"name":"A","maneuver":{"type":"depart","location":[0,0]}},
				{"name":"B","maneuver":{"type":"turn","modifier":"left","location":[0,1]}},
				{"name":"B","maneuver":{"type":"arrive","modifier":"left","location":[1,1]}}]},
			{"steps":[
				{"name":"B","maneuver":{"type":"depart","location":[1,1]}},
				{"name":"C","maneuver":{"type":"fork","modifier":"slight right","location":[2,1],"bearing_before":350,"bearing_after":20}},
				{"name":"E","maneuver":{"type":"roundabout","modifier":"right","exit":2,"location":[2,1]}},
				{"name":"D","maneuver":{"type":"new name","modifier":"left","location":[2.1,1.6]}},
				{"name":"D","maneuver":{"type":"arrive","location":[2,2]}}]}],
			"distance":1,"duration":1}]}`))
	}))
	defer srv.Close()

	o := &OSRM{URL: srv.URL}
	route, err := o.Route(context.Background(), Request{Points: make([]geo.LonLat, 3)})
	if err != nil {
		t.Fatal(err)
	}
	want := []Turn{
		{Index: 0, Kind: TurnStart, Street: "A"},
		{Index: 1, Kind: TurnLeft, Street: "B"},
		{Index: 2, Kind: TurnVia},
		{Index: 3, Kind: TurnKeepRight, Street: "C", Angle: 30},
		{Index: 3, Kind: TurnRoundabout, Street: "E", Exit: 2},
		{Index: 4, Kind: TurnContinue, Street: "D"},
		{Index: 4, Kind: TurnEnd},
	}
	checkTurns(t, route.Turns, want)
}

func TestPointIndex(t *testing.T) {
	points := []geo.LonLat{{Lon: 0, Lat: 0}, {Lon: 1, Lat: 0}, {Lon: 0, Lat: 0}, {Lon: 2, Lat: 0}}
	for _, tc := range []struct {
		p          geo.LonLat
		from, want int
	}{
		{geo.LonLat{Lon: 0, Lat: 0}, 0, 0},
		{geo.LonLat{Lon: 0, Lat: 0}, 1, 2},
		{geo.LonLat{Lon: 1.9, Lat: 0.1}, 0, 3},
		{geo.LonLat{Lon: 0.4, Lat: 0}, 2, 2},
	} {
		if got := pointIndex(points, tc.p, tc.from); got != tc.want {
			t.Errorf("point %v from %d: expected %d, got %d", tc.p, tc.from, tc.want, got)
		}
	}
}

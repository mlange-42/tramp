package routing

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/httpx"
)

// valhallaSample is a response of the public Valhalla server for a short walk in Leipzig,
// with the summaries shortened.
const valhallaSample = `{"trip":{"locations":[{"type":"break","lat":51.3397,"lon":12.3731,"original_index":0},
	{"type":"break","lat":51.3405,"lon":12.376,"side_of_street":"right","original_index":1}],
	"legs":[{"summary":{"time":181.171,"length":0.26},
	"shape":"}|o|` + "`" + `BmferVmCuFs@wAK}CA]Ag@SgISiGQkGMmFIsAE_DEkAIqCEaBCu@QcGEaE?iJ@uI@aLsa@gABwEUuAZ}PD{AXsP{BS"}],
	"summary":{"time":181.171,"length":0.26},
	"status_message":"Found route between points","status":0,"units":"kilometers","language":"en-US"}}`

func TestValhallaRoute(t *testing.T) {
	var got valhallaRequest
	var gotAgent, gotType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAgent, gotType = r.UserAgent(), r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(valhallaSample))
	}))
	defer srv.Close()

	v := &Valhalla{URL: srv.URL, Client: httpx.Client{UserAgent: "test-agent"}}
	req := Request{Points: []geo.LonLat{{Lon: 12.3731, Lat: 51.3397}, {Lon: 12.376, Lat: 51.3405}}, Profile: ProfileBike}
	route, err := v.Route(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if gotAgent != "test-agent" || gotType != "application/json" || got.Costing != "bicycle" || got.Units != "kilometers" ||
		len(got.Locations) != 2 || got.Locations[1] != (valhallaLocation{Lat: 51.3405, Lon: 12.376}) {
		t.Errorf("unexpected request %+v, agent %q, type %q", got, gotAgent, gotType)
	}
	if route.Distance != 260 || route.Duration != 181171*time.Millisecond {
		t.Errorf("unexpected distance %v or duration %v", route.Distance, route.Duration)
	}
	if len(route.Points) != 28 {
		t.Fatalf("expected 28 points, got %d", len(route.Points))
	}
	first, last := route.Points[0], route.Points[len(route.Points)-1]
	if !near(first, geo.LonLat{Lon: 12.373111, Lat: 51.339743}) || !near(last, geo.LonLat{Lon: 12.375851, Lat: 51.34051}) {
		t.Errorf("unexpected ends %v, %v", first, last)
	}
}

func TestValhallaLegs(t *testing.T) {
	// Two legs of a line from (0, 0) over (0, 1) to (1, 1), each starting where the previous one ends.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"trip":{"legs":[{"shape":"??_c` + "`" + `|@?"},{"shape":"_c` + "`" + `|@??_c` + "`" + `|@"}],"summary":{"length":222,"time":3600}}}`))
	}))
	defer srv.Close()

	v := &Valhalla{URL: srv.URL}
	route, err := v.Route(context.Background(), Request{Points: make([]geo.LonLat, 3)})
	if err != nil {
		t.Fatal(err)
	}
	want := []geo.LonLat{{Lon: 0, Lat: 0}, {Lon: 0, Lat: 1}, {Lon: 1, Lat: 1}}
	if len(route.Points) != len(want) {
		t.Fatalf("expected %v, got %v", want, route.Points)
	}
	for i, p := range route.Points {
		if !near(p, want[i]) {
			t.Errorf("point %d: expected %v, got %v", i, want[i], p)
		}
	}
}

func TestValhallaErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error_code":154,"error":"Path distance exceeds the max distance limit: 100000 meters","status_code":400,"status":"Bad Request"}`))
	}))
	defer srv.Close()

	v := &Valhalla{URL: srv.URL}
	points := []geo.LonLat{{Lon: 12, Lat: 51}, {Lon: 5, Lat: 54}}
	_, err := v.Route(context.Background(), Request{Points: points})
	if err == nil || !strings.Contains(err.Error(), "max distance limit") {
		t.Errorf("expected message of the service, got %v", err)
	}
	if _, err := v.Route(context.Background(), Request{Points: points[:1]}); err == nil {
		t.Error("expected error for a single point")
	}
	if _, err := v.Route(context.Background(), Request{Points: points, Profile: "horse"}); err == nil {
		t.Error("expected error for unknown profile")
	}
}

// near reports whether two positions are equal up to rounding.
func near(a, b geo.LonLat) bool {
	return math.Abs(a.Lon-b.Lon) < 1e-7 && math.Abs(a.Lat-b.Lat) < 1e-7
}

func TestValhallaAlternatives(t *testing.T) {
	var got valhallaRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = valhallaRequest{}
		_ = json.NewDecoder(r.Body).Decode(&got)
		// The main route from (0, 0) to (0, 1), and alternatives over (1, 0) and a broken one.
		_, _ = w.Write([]byte(`{"trip":{"legs":[{"shape":"??_c` + "`" + `|@?"}],"summary":{"length":111,"time":3600}},
			"alternates":[{"trip":{"legs":[{"shape":"??_c` + "`" + `|@?"}],"summary":{"length":150,"time":4000}}},
			{"trip":{"legs":[{"shape":"_"}],"summary":{"length":1,"time":1}}}]}`))
	}))
	defer srv.Close()

	v := &Valhalla{URL: srv.URL}
	route, err := v.Route(context.Background(), Request{Points: make([]geo.LonLat, 2), Alternatives: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got.Alternates != 2 {
		t.Errorf("expected 2 alternates asked for, got %d", got.Alternates)
	}
	if len(route.Alternatives) != 1 || route.Alternatives[0].Distance != 150000 || len(route.Alternatives[0].Points) != 2 {
		t.Errorf("expected one alternative, got %+v", route.Alternatives)
	}

	// With intermediate points, no alternatives are asked for.
	if _, err := v.Route(context.Background(), Request{Points: make([]geo.LonLat, 3), Alternatives: 2}); err != nil {
		t.Fatal(err)
	}
	if got.Alternates != 0 {
		t.Errorf("expected no alternates asked for, got %d", got.Alternates)
	}
}

func TestValhallaTurns(t *testing.T) {
	// Two legs from (0, 0) over (0, 1) to (2, 2), with ' for backticks in the shapes.
	body := strings.ReplaceAll(`{"trip":{"legs":[
		{"shape":"??_c'|@?","maneuvers":[
			{"type":1,"street_names":["A"],"begin_shape_index":0},
			{"type":5,"begin_shape_index":1}]},
		{"shape":"_c'|@??_c'|@?_c'|@_c'|@?","maneuvers":[
			{"type":3,"street_names":["B"],"begin_shape_index":0},
			{"type":15,"street_names":["C","C2"],"begin_shape_index":1,"bearing_before":10,"bearing_after":280},
			{"type":26,"street_names":["D"],"begin_shape_index":2,"roundabout_exit_count":3},
			{"type":8,"street_names":["E"],"begin_shape_index":2},
			{"type":6,"begin_shape_index":3,"bearing_before":90}]}],
		"summary":{"length":1,"time":1}}}`, "'", "`")
	var got valhallaRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	v := &Valhalla{URL: srv.URL}
	route, err := v.Route(context.Background(), Request{Points: make([]geo.LonLat, 3)})
	if err != nil {
		t.Fatal(err)
	}
	if got.DirectionsType != "maneuvers" {
		t.Errorf("expected maneuvers asked for, got %q", got.DirectionsType)
	}
	if len(route.Points) != 5 {
		t.Fatalf("expected 5 points, got %v", route.Points)
	}
	want := []Turn{
		{Index: 0, Kind: TurnStart, Street: "A"},
		{Index: 1, Kind: TurnVia},
		{Index: 2, Kind: TurnLeft, Street: "C", Angle: -90},
		{Index: 3, Kind: TurnRoundabout, Street: "D", Exit: 3},
		{Index: 3, Kind: TurnContinue, Street: "E"},
		{Index: 4, Kind: TurnEnd},
	}
	checkTurns(t, route.Turns, want)
}

// checkTurns reports differences between turns.
func checkTurns(t *testing.T, got, want []Turn) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("turn %d: expected %+v, got %+v", i, want[i], got[i])
		}
	}
}

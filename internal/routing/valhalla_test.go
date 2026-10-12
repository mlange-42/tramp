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

	v := &Valhalla{URL: srv.URL, UserAgent: "test-agent"}
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

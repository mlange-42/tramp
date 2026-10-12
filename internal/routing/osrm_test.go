package routing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
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

	o := &OSRM{URL: srv.URL + "/routed-" + ProfilePlaceholder + "/", UserAgent: "test-agent"}
	req := Request{Points: []geo.LonLat{{Lon: 12.3731, Lat: 51.3397}, {Lon: 12.376, Lat: 51.3405}}}
	route, err := o.Route(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/routed-foot/route/v1/driving/12.373100,51.339700;12.376000,51.340500" ||
		gotQuery != "overview=full&geometries=geojson&steps=false" || gotAgent != "test-agent" {
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

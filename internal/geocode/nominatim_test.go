package geocode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/httpx"
)

func TestNominatimSearch(t *testing.T) {
	var gotQuery, gotAgent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery, gotAgent = r.URL.Query().Get("q"), r.UserAgent()
		_, _ = w.Write([]byte(`[
			{"display_name": "Leipzig, Sachsen, Deutschland", "lat": "51.3406", "lon": "12.3747", "type": "city",
			 "boundingbox": ["51.238", "51.448", "12.236", "12.542"]},
			{"display_name": "Broken", "lat": "x", "lon": "12"},
			{"display_name": "Leipzig, Ohio", "lat": "41.10", "lon": "-83.99", "type": "village"}
		]`))
	}))
	defer srv.Close()

	n := &Nominatim{URL: srv.URL, Client: httpx.Client{UserAgent: "test-agent"}}
	places, err := n.Search(context.Background(), Query{Text: "Leipzig", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "Leipzig" || gotAgent != "test-agent" {
		t.Errorf("request: query %q, agent %q", gotQuery, gotAgent)
	}
	if len(places) != 2 {
		t.Fatalf("expected 2 places, got %d", len(places))
	}
	p := places[0]
	if p.Name != "Leipzig, Sachsen, Deutschland" || p.Kind != "city" || p.Pos.Lat != 51.3406 || p.Pos.Lon != 12.3747 {
		t.Errorf("unexpected place %+v", p)
	}
	if !p.HasBounds || p.Min.Lat != 51.238 || p.Max.Lat != 51.448 || p.Min.Lon != 12.236 || p.Max.Lon != 12.542 {
		t.Errorf("unexpected bounds %+v", p)
	}
	if places[1].HasBounds {
		t.Errorf("expected no bounds for %+v", places[1])
	}
}

func TestNominatimError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "busy", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	n := &Nominatim{URL: srv.URL}
	if _, err := n.Search(context.Background(), Query{Text: "x", Limit: 5}); err == nil {
		t.Error("expected an error")
	}
}

func TestNominatimInterval(t *testing.T) {
	n := &Nominatim{}
	ctx := context.Background()
	start := time.Now()
	if err := n.wait(ctx); err != nil {
		t.Fatal(err)
	}
	if err := n.wait(ctx); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < nominatimInterval {
		t.Errorf("second request after %v, want at least %v", d, nominatimInterval)
	}
	// A canceled context stops waiting.
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := n.wait(ctx); err == nil {
		t.Error("expected an error for a canceled context")
	}
}

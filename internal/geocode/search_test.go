package geocode

import (
	"context"
	"errors"
	"testing"

	"github.com/mlange-42/tramp/internal/geo"
)

// countingSearcher counts the searches, and fails if err is set.
type countingSearcher struct {
	n   int
	err error
}

func (c *countingSearcher) Search(_ context.Context, q Query) ([]Place, error) {
	c.n++
	return []Place{{Name: q.Text}}, c.err
}

func (c *countingSearcher) Attribution() string { return "test" }

func (c *countingSearcher) Interactive() bool { return true }

func TestCache(t *testing.T) {
	s := &countingSearcher{}
	c := NewCache(s, 2)
	ctx := context.Background()
	search := func(q string) string {
		t.Helper()
		p, err := c.Search(ctx, Query{Text: q, Limit: 5})
		if err != nil || len(p) != 1 {
			t.Fatalf("%q: %v %v", q, p, err)
		}
		return p[0].Name
	}
	search("Leipzig")
	// Case and spaces don't matter.
	if got := search("  leipzig "); got != "Leipzig" || s.n != 1 {
		t.Errorf("expected cached result, got %q after %d searches", got, s.n)
	}
	search("Halle")
	search("Dresden")
	// The oldest search was dropped.
	search("Leipzig")
	if s.n != 4 {
		t.Errorf("expected 4 searches, got %d", s.n)
	}
	if c.Attribution() != "test" {
		t.Errorf("attribution not passed on")
	}

	// Failed searches are not cached.
	s.err = errors.New("offline")
	if _, err := c.Search(ctx, Query{Text: "Berlin", Limit: 5}); err == nil {
		t.Fatal("expected an error")
	}
	s.err = nil
	search("Berlin")
	if s.n != 6 {
		t.Errorf("expected the failed search repeated, got %d searches", s.n)
	}

	// Nearby positions share results, distant ones don't.
	near := func(lon, lat float64) {
		t.Helper()
		q := Query{Text: "Hauptbahnhof", Limit: 5, Near: geo.LonLat{Lon: lon, Lat: lat}, HasNear: true}
		if _, err := c.Search(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	near(12.371, 51.341)
	near(12.372, 51.342)
	if s.n != 7 {
		t.Errorf("expected nearby search cached, got %d searches", s.n)
	}
	near(13.4, 52.5)
	if s.n != 8 {
		t.Errorf("expected distant search not cached, got %d searches", s.n)
	}
}

func TestNew(t *testing.T) {
	for service, interactive := range map[string]bool{"": true, "Nominatim": false, "photon": true} {
		s, err := New(service, "", nil, "test")
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := s.(*Cache); !ok || s.Interactive() != interactive {
			t.Errorf("%q: unexpected searcher %T, interactive %v", service, s, s.Interactive())
		}
	}
	if _, err := New("google", "", nil, "test"); err == nil {
		t.Error("expected an error for an unknown service")
	}
}

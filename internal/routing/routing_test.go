package routing

import (
	"context"
	"errors"
	"math"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
)

func TestNew(t *testing.T) {
	for _, tc := range []struct {
		service string
		attr    string
	}{
		{"", ValhallaAttribution},
		{"valhalla", ValhallaAttribution},
		{" OSRM ", OSRMAttribution},
	} {
		r, err := New(tc.service, "", http.DefaultClient, "test-agent")
		if err != nil {
			t.Fatalf("service %q: %v", tc.service, err)
		}
		if _, ok := r.(*Throttle); !ok {
			t.Errorf("service %q: expected a throttled router, got %T", tc.service, r)
		}
		if r.Attribution() != tc.attr {
			t.Errorf("service %q: attribution %q", tc.service, r.Attribution())
		}
	}
	if _, err := New("google", "", nil, ""); err == nil {
		t.Error("expected error for unknown service")
	}
}

func TestDecodePolyline(t *testing.T) {
	// The example of the polyline algorithm documentation.
	points, err := decodePolyline("_p~iF~ps|U_ulLnnqC_mqNvxq`@", 5)
	if err != nil {
		t.Fatal(err)
	}
	want := []geo.LonLat{{Lon: -120.2, Lat: 38.5}, {Lon: -120.95, Lat: 40.7}, {Lon: -126.453, Lat: 43.252}}
	if len(points) != len(want) {
		t.Fatalf("expected %d points, got %d", len(want), len(points))
	}
	for i, p := range points {
		if math.Abs(p.Lon-want[i].Lon) > 1e-9 || math.Abs(p.Lat-want[i].Lat) > 1e-9 {
			t.Errorf("point %d: expected %v, got %v", i, want[i], p)
		}
	}

	for _, s := range []string{"_p~iF~ps|U_", "_p~iF", " "} {
		if _, err := decodePolyline(s, 5); err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
}

// slowRouter records the start times of its requests and takes some time for each.
type slowRouter struct {
	mu      sync.Mutex
	starts  []time.Time
	running int
	overlap bool
}

func (s *slowRouter) Route(ctx context.Context, r Request) (*Route, error) {
	s.mu.Lock()
	s.starts = append(s.starts, time.Now())
	s.running++
	s.overlap = s.overlap || s.running > 1
	s.mu.Unlock()
	time.Sleep(5 * time.Millisecond)
	s.mu.Lock()
	s.running--
	s.mu.Unlock()
	return &Route{}, nil
}

func (s *slowRouter) Attribution() string { return "" }

func TestThrottle(t *testing.T) {
	s := &slowRouter{}
	const gap = 30 * time.Millisecond
	th := NewThrottle(s, gap)
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			if _, err := th.Route(context.Background(), Request{}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if len(s.starts) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(s.starts))
	}
	if s.overlap {
		t.Error("requests ran at the same time")
	}
	for i := 1; i < len(s.starts); i++ {
		if d := s.starts[i].Sub(s.starts[i-1]); d < gap-time.Millisecond {
			t.Errorf("request %d started %v after the previous one, expected at least %v", i, d, gap)
		}
	}
}

func TestThrottleCanceled(t *testing.T) {
	th := NewThrottle(&slowRouter{}, time.Hour)
	if _, err := th.Route(context.Background(), Request{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := th.Route(ctx, Request{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected deadline error, got %v", err)
	}
}

func TestTurnAngle(t *testing.T) {
	for _, tc := range []struct{ before, after, want float64 }{
		{0, 90, 90},
		{90, 0, -90},
		{350, 20, 30},
		{20, 350, -30},
		{0, 180, 180},
		{180, 0, 180},
		{222, 164, -58},
	} {
		if got := turnAngle(tc.before, tc.after); got != tc.want {
			t.Errorf("%v to %v: expected %v, got %v", tc.before, tc.after, tc.want, got)
		}
	}
}

func TestFixTurns(t *testing.T) {
	// Without turns, the start and the end are added.
	r := Route{Points: make([]geo.LonLat, 3)}
	r.fixTurns()
	checkTurns(t, r.Turns, []Turn{{Index: 0, Kind: TurnStart}, {Index: 2, Kind: TurnEnd}})

	// Turns are clamped and sorted, and the start is moved to the first point.
	r.Turns = []Turn{{Index: 1, Kind: TurnStart}, {Index: 5, Kind: TurnLeft}, {Index: 1, Kind: TurnRight}}
	r.fixTurns()
	checkTurns(t, r.Turns, []Turn{
		{Index: 0, Kind: TurnStart},
		{Index: 1, Kind: TurnRight},
		{Index: 2, Kind: TurnLeft},
		{Index: 2, Kind: TurnEnd},
	})
}

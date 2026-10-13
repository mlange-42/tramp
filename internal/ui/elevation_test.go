package ui

import (
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/settings"
)

// elevationServer serves terrarium tiles of 4x4 pixels with an elevation of 123 m,
// or errors if failing is set. It counts the requests.
func elevationServer(t *testing.T) (*httptest.Server, *atomic.Bool, *atomic.Int32) {
	var failing atomic.Bool
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if failing.Load() {
			http.Error(w, "broken", http.StatusInternalServerError)
			return
		}
		img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
		// 123 m is 32891 = 128*256 + 123 in the terrarium encoding.
		for i := range 16 {
			img.SetNRGBA(i%4, i/4, color.NRGBA{R: 128, G: 123, A: 255})
		}
		_ = png.Encode(w, img)
	}))
	t.Cleanup(srv.Close)
	return srv, &failing, &requests
}

// waitElevation waits for the background download of the elevation under the pointer, and applies its result.
func waitElevation(t *testing.T, a *App) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for a.elev.busy {
		if time.Now().After(deadline) {
			t.Fatal("timeout")
		}
		a.runUITasks()
		time.Sleep(time.Millisecond)
	}
}

func TestHoverElevation(t *testing.T) {
	srv, failing, requests := elevationServer(t)
	cfg := settings.Elevation{URL: srv.URL + "/{z}/{x}/{y}.png", TileSize: 4}
	a := &App{mapView: mapview.New(geo.LonLat{Lon: 12.38, Lat: 51.34}, 12), invalidate: func() {}}
	a.elev = elevationView{cfg: cfg, service: newElevationService(cfg, 0)}

	// Nothing without the pointer on the map.
	if _, st := a.hoverElevation(); st != elevOff || a.elev.busy {
		t.Fatal("expected no elevation without the pointer on the map")
	}
	a.mapView.Hover, a.mapView.HoverValid = geo.ToMercator(geo.LonLat{Lon: 12.38, Lat: 51.34}), true

	// The tile is downloaded in the background, and shown as loading meanwhile, also on later frames.
	if _, st := a.hoverElevation(); st != elevLoading || !a.elev.busy {
		t.Fatal("expected a download")
	}
	if _, st := a.hoverElevation(); st != elevLoading {
		t.Errorf("expected loading while downloading, got %v", st)
	}
	waitElevation(t, a)
	if e, st := a.hoverElevation(); st != elevKnown || e != 123 {
		t.Errorf("expected 123 m, got %v, %v", e, st)
	}
	if cs := a.mapCredits(); len(cs) != 0 {
		t.Errorf("expected no credit for another service, got %+v", cs)
	}

	// A failed download is not tried again right away.
	failing.Store(true)
	a.mapView.SetZoom(14)
	a.hoverElevation()
	waitElevation(t, a)
	n := requests.Load()
	if _, st := a.hoverElevation(); st != elevMissing || a.elev.busy || a.elev.failed.IsZero() {
		t.Errorf("expected no new download after a failure, got %v", st)
	}
	if requests.Load() != n {
		t.Errorf("expected no new requests, got %d after %d", requests.Load(), n)
	}
}

func TestElevationNone(t *testing.T) {
	if s := newElevationService(settings.Elevation{Source: " None "}, 0); s != nil {
		t.Error("expected no service for source none")
	}
	if s := newElevationService(settings.Elevation{Source: "srtm"}, 0); s == nil || s.Source().Name != "mapterhorn" {
		t.Error("expected the default source for an unknown one")
	}
	a := &App{mapView: mapview.New(geo.LonLat{}, 3)}
	a.mapView.HoverValid = true
	if _, st := a.hoverElevation(); st != elevOff || len(a.mapCredits()) != 0 {
		t.Error("expected no elevation and attribution when turned off")
	}
}

func TestFormatHoverElevation(t *testing.T) {
	for _, tc := range []struct {
		e     float64
		state elevState
		want  string
	}{
		{117.4, elevKnown, "   117 m"},
		{-12.6, elevKnown, "   -13 m"},
		{2962, elevKnown, "  2962 m"},
		{0, elevLoading, "   ... m"},
		{0, elevMissing, "     - m"},
		{0, elevOff, "        "},
	} {
		if got := formatHoverElevation(tc.e, tc.state); got != tc.want || len(got) != elevationWidth {
			t.Errorf("%v, %v: expected %q, got %q", tc.e, tc.state, tc.want, got)
		}
	}
}

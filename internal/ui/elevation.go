package ui

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/mlange-42/tramp/internal/elevation"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/httpx"
	"github.com/mlange-42/tramp/internal/settings"
)

// elevationNone is the elevation source setting that turns elevations off.
const elevationNone = "none"

// elevationRetry is the time after a failed download before the elevation under the pointer is requested again.
const elevationRetry = 10 * time.Second

// elevationView looks up the elevation under the pointer on the map, for the status bar.
type elevationView struct {
	// service is nil if elevations are turned off.
	service *elevation.Service
	// busy is set while a tile is downloaded, and failed is the time of the last failed download.
	busy   bool
	failed time.Time
	// cfg is the configured source, and cacheSize the memory in MB for the tiles, kept for saving the settings.
	cfg       settings.Elevation
	cacheSize int
}

// newElevationService returns the service for the configured elevation source, or nil if turned off.
// An invalid configuration is logged, and the default source is used.
// cacheSize is the memory in MB for the tiles, the default if 0 or less.
func newElevationService(cfg settings.Elevation, cacheSize int) *elevation.Service {
	if strings.EqualFold(strings.TrimSpace(cfg.Source), elevationNone) {
		return nil
	}
	src, err := elevation.Configure(cfg.Source, cfg.URL, cfg.Encoding, cfg.TileSize, cfg.MaxZoom)
	if err != nil {
		log.Printf("elevation: %v", err)
		src = elevation.Sources[0]
	}
	client := httpx.Client{HTTP: &http.Client{Timeout: 30 * time.Second}, UserAgent: UserAgent}
	return elevation.NewService(src, client, cacheSize<<20)
}

// elevState is the state of the elevation under the pointer.
type elevState int

const (
	// elevOff is shown while the pointer is not on the map, or elevations are turned off.
	elevOff elevState = iota
	// elevLoading is shown while the tile is downloaded.
	elevLoading
	// elevMissing is shown where the source has no data, or the download failed.
	elevMissing
	// elevKnown is shown with the elevation.
	elevKnown
)

// hoverElevation returns the elevation under the pointer on the map and its state.
// A missing tile is downloaded in the background, one at a time, and the window redrawn when it arrives.
func (a *App) hoverElevation() (float64, elevState) {
	ev := &a.elev
	if ev.service == nil || !a.mapView.HoverValid {
		return math.NaN(), elevOff
	}
	p := geo.ToLonLat(a.mapView.Hover)
	// The tiles of the map's level have about the resolution of the screen.
	zoom := a.mapView.TileLevel()
	if e, ok := ev.service.Cached(p, zoom); ok {
		if math.IsNaN(e) {
			return e, elevMissing
		}
		return e, elevKnown
	}
	if ev.busy {
		return math.NaN(), elevLoading
	}
	if time.Since(ev.failed) <= elevationRetry {
		return math.NaN(), elevMissing
	}
	ev.busy = true
	svc := ev.service
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := svc.Elevation(ctx, p, zoom)
		a.runOnUI(func() {
			ev.busy = false
			if err != nil {
				// Logged once per streak of failures.
				if ev.failed.IsZero() {
					log.Printf("elevation: %v", err)
				}
				ev.failed = time.Now()
				return
			}
			ev.failed = time.Time{}
		})
	}()
	return math.NaN(), elevLoading
}

// elevationWidth is the width of the elevation in the status bar, like "  1234 m".
const elevationWidth = 8

// formatHoverElevation returns the elevation for the status bar, padded to a fixed width:
// blank while off, "..." while loading, and "-" without data.
func formatHoverElevation(e float64, state elevState) string {
	switch state {
	case elevLoading:
		return fmt.Sprintf("%*s m", elevationWidth-2, "...")
	case elevMissing:
		return fmt.Sprintf("%*s m", elevationWidth-2, "-")
	case elevKnown:
		return fmt.Sprintf("%*.0f m", elevationWidth-2, e)
	}
	return strings.Repeat(" ", elevationWidth)
}

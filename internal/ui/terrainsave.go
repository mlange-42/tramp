package ui

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/track"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

var iconTerrain = mustIcon(icons.MapsTerrain)

// terrainSavable reports whether the file can be saved with the terrain elevation: it has tracks,
// and elevations are turned on. Such files are never edited, see [openFile.editable].
func (a *App) terrainSavable(f *openFile) bool {
	return a.elev.service != nil && f.data != nil && len(f.data.Tracks) > 0
}

// saveWithTerrain asks for a file name, and saves a copy of the file there, with the elevation of all track points
// replaced by the terrain elevation, see [withTerrain]. The copy is opened.
func (a *App) saveWithTerrain(f *openFile) {
	if !a.terrainSavable(f) {
		return
	}
	svc := a.elev.service
	data := f.data
	pos := trackPositions(data)
	center := geo.ToLonLat(geo.Point{X: (f.bounds.Min.X + f.bounds.Max.X) / 2, Y: (f.bounds.Min.Y + f.bounds.Max.Y) / 2})
	zoom := svc.Source().ZoomFor(profileResolution, center.Lat)
	name := strings.TrimSuffix(f.path, filepath.Ext(f.path)) + "-terrain.gpx"
	a.createFileFrom("Save with terrain elevation", name, func() (*track.File, error) {
		ctx, cancel := context.WithTimeout(context.Background(), profileTimeout)
		defer cancel()
		eles, err := svc.Elevations(ctx, pos, zoom)
		if err != nil {
			return nil, fmt.Errorf("terrain elevation: %w", err)
		}
		return withTerrain(data, eles), nil
	}, nil)
}

// trackPositions returns the positions of all track points of the file, in file order.
func trackPositions(d *track.File) []geo.LonLat {
	var pos []geo.LonLat
	for _, t := range d.Tracks {
		for _, s := range t.Segments {
			for _, p := range s.Points {
				pos = append(pos, p.Pos)
			}
		}
	}
	return pos
}

// withTerrain returns a copy of the file with the elevation of the track points replaced by eles,
// one per point in the order of [trackPositions]. Points keep their elevation where eles is NaN.
// The routes, waypoints and everything else are kept, and d is not changed.
func withTerrain(d *track.File, eles []float64) *track.File {
	c := *d
	c.Path, c.Format, c.Creator = "", track.FormatGPX, track.Creator
	c.Tracks = slices.Clone(d.Tracks)
	k := 0
	for i := range c.Tracks {
		t := &c.Tracks[i]
		t.Segments = slices.Clone(t.Segments)
		for j := range t.Segments {
			s := &t.Segments[j]
			s.Points = slices.Clone(s.Points)
			for n := range s.Points {
				if e := eles[k]; !math.IsNaN(e) {
					s.Points[n].Ele = e
				}
				k++
			}
		}
	}
	return &c
}

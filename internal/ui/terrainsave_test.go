package ui

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/mlange-42/tramp/internal/track"
)

func TestWithTerrain(t *testing.T) {
	_, f := editApp(t, testEleGPX)
	d := f.data
	pos := trackPositions(d)
	if len(pos) != 5 || pos[3] != d.Tracks[1].Segments[0].Points[0].Pos {
		t.Fatalf("expected the 5 track points in file order, got %v", pos)
	}
	// The terrain of the second point is missing, so it keeps its elevation.
	eles := []float64{200, math.NaN(), 220, 300, 310}
	c := withTerrain(d, eles)
	if c == d || c.Creator != track.Creator || c.Format != track.FormatGPX || c.Path != "" {
		t.Errorf("expected a new GPX file by TRAMP, got %+v", c)
	}
	var got []float64
	for _, tr := range c.Tracks {
		for _, s := range tr.Segments {
			for _, p := range s.Points {
				got = append(got, p.Ele)
			}
		}
	}
	want := []float64{200, 110, 220, 300, 310}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("point %d: expected %v, got %v", i, want[i], got[i])
		}
	}
	if d.Tracks[0].Segments[0].Points[0].Ele != 100 || !math.IsNaN(d.Tracks[1].Segments[0].Points[0].Ele) {
		t.Error("expected the original file unchanged")
	}

	// The copy reads back with the new elevations and the track names.
	path := filepath.Join(t.TempDir(), "terrain.gpx")
	if err := track.WriteFile(path, c); err != nil {
		t.Fatal(err)
	}
	r, err := track.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Tracks) != 2 || r.Tracks[0].Name != "Recorded" || r.Tracks[1].Segments[0].Points[1].Ele != 310 {
		t.Errorf("unexpected file read back %+v", r.Tracks)
	}
}

func TestTerrainSavable(t *testing.T) {
	a, f := terrainApp(t)
	if !a.terrainSavable(f) {
		t.Error("expected a file with tracks to be savable with terrain")
	}
	b, plan := editApp(t, testPlanGPX)
	b.elev = a.elev
	if b.terrainSavable(plan) {
		t.Error("expected a file without tracks not to be savable with terrain")
	}
	a.elev.service = nil
	if a.terrainSavable(f) {
		t.Error("expected nothing to save without elevations")
	}
}

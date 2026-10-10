package track

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
)

// sameFloat reports whether a and b are equal or both NaN.
func sameFloat(a, b float64) bool {
	return a == b || math.IsNaN(a) && math.IsNaN(b)
}

func sameWaypoints(t *testing.T, what string, got, want []Waypoint) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: expected %d points, got %d", what, len(want), len(got))
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Pos != w.Pos || !sameFloat(g.Ele, w.Ele) || !g.Time.Equal(w.Time) ||
			g.Name != w.Name || g.Desc != w.Desc || g.Symbol != w.Symbol || g.Type != w.Type {
			t.Errorf("%s %d: expected %+v, got %+v", what, i, w, g)
		}
	}
}

func roundTrip(t *testing.T, f *File) *File {
	t.Helper()
	var b bytes.Buffer
	if err := WriteGPX(&b, f); err != nil {
		t.Fatal(err)
	}
	out, err := ReadGPX(&b)
	if err != nil {
		t.Fatalf("%v\n%s", err, b.String())
	}
	return out
}

func TestWriteGPXRoundTrip(t *testing.T) {
	f, err := ReadGPX(strings.NewReader(gpx11))
	if err != nil {
		t.Fatal(err)
	}
	f.Desc = `Fish & chips <at> "the" lake`
	f.Waypoints[0].Desc = "Ünïcödé"
	out := roundTrip(t, f)

	if out.Name != f.Name || out.Desc != f.Desc || !out.Time.Equal(f.Time) {
		t.Errorf("unexpected metadata %q %q %v", out.Name, out.Desc, out.Time)
	}
	sameWaypoints(t, "waypoint", out.Waypoints, f.Waypoints)
	if len(out.Routes) != len(f.Routes) {
		t.Fatalf("expected %d routes, got %d", len(f.Routes), len(out.Routes))
	}
	for i := range f.Routes {
		if out.Routes[i].Name != f.Routes[i].Name || out.Routes[i].Desc != f.Routes[i].Desc || out.Routes[i].Type != f.Routes[i].Type {
			t.Errorf("route %d: unexpected info %+v", i, out.Routes[i])
		}
		sameWaypoints(t, "route point", out.Routes[i].Points, f.Routes[i].Points)
	}
	if len(out.Tracks) != len(f.Tracks) {
		t.Fatalf("expected %d tracks, got %d", len(f.Tracks), len(out.Tracks))
	}
	for i := range f.Tracks {
		tw, tg := &f.Tracks[i], &out.Tracks[i]
		if tg.Name != tw.Name || tg.Type != tw.Type || len(tg.Segments) != len(tw.Segments) {
			t.Fatalf("track %d: expected %+v, got %+v", i, tw, tg)
		}
		for j := range tw.Segments {
			sw, sg := &tw.Segments[j], &tg.Segments[j]
			if len(sg.Points) != len(sw.Points) {
				t.Fatalf("segment %d: expected %d points, got %d", j, len(sw.Points), len(sg.Points))
			}
			for k := range sw.Points {
				pw, pg := sw.Points[k], sg.Points[k]
				if pg.Pos != pw.Pos || !sameFloat(pg.Ele, pw.Ele) || !pg.Time.Equal(pw.Time) {
					t.Errorf("segment %d point %d: expected %+v, got %+v", j, k, pw, pg)
				}
			}
			for c := range NumChannels {
				if !reflect.DeepEqual(nanString(sg.Channel(c)), nanString(sw.Channel(c))) {
					t.Errorf("segment %d %v: expected %v, got %v", j, c, sw.Channel(c), sg.Channel(c))
				}
			}
		}
	}
}

// nanString replaces NaN by a marker, as NaN values are never deeply equal.
func nanString(vals []float64) []any {
	if vals == nil {
		return nil
	}
	out := make([]any, len(vals))
	for i, v := range vals {
		if math.IsNaN(v) {
			out[i] = "NaN"
		} else {
			out[i] = v
		}
	}
	return out
}

func TestWriteGPXAllChannels(t *testing.T) {
	seg := Segment{Points: []Point{
		{Pos: geo.LonLat{Lon: 12.4086787, Lat: 51.3288433}, Ele: 52.2, Time: time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)},
		{Pos: geo.LonLat{Lon: 12.4, Lat: 51.3}, Ele: math.NaN()},
	}}
	for c := range NumChannels {
		seg.SetChannel(c, 0, float64(c)+1.5)
	}
	seg.SetChannel(Satellites, 0, 7)
	f := &File{Tracks: []Track{{Segments: []Segment{seg}}}}
	out := roundTrip(t, f)
	s := &out.Tracks[0].Segments[0]
	for c := range NumChannels {
		want := seg.Channel(c)
		if got := s.Channel(c); len(got) != 2 || got[0] != want[0] || !math.IsNaN(got[1]) {
			t.Errorf("%v: expected %v, got %v", c, want, got)
		}
	}
	if p := s.Points[0]; p.Pos != seg.Points[0].Pos {
		t.Errorf("expected exact position %v, got %v", seg.Points[0].Pos, p.Pos)
	}
}

func TestWriteGPXEmpty(t *testing.T) {
	out := roundTrip(t, &File{})
	if len(out.Tracks)+len(out.Routes)+len(out.Waypoints) != 0 || out.Name != "" {
		t.Errorf("expected empty file, got %+v", out)
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.gpx")
	f := &File{Routes: []Route{{Name: "R", Points: []Waypoint{
		{Pos: geo.LonLat{Lon: 12, Lat: 51}, Ele: math.NaN()},
		{Pos: geo.LonLat{Lon: 12.1, Lat: 51.1}, Ele: 100},
	}}}}
	for range 2 {
		if err := WriteFile(path, f); err != nil {
			t.Fatal(err)
		}
	}
	out, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sameWaypoints(t, "route point", out.Routes[0].Points, f.Routes[0].Points)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("expected only the written file, got %v", entries)
	}
	if err := WriteFile(filepath.Join(dir, "plan.kml"), f); err == nil {
		t.Errorf("expected error for unsupported format")
	}
}

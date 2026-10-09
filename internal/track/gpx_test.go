package track

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// devicePoint is a track point of a generated device file.
type devicePoint struct {
	lat, lon, ele, speed float64
	time                 time.Time
}

// writeDeviceGPX writes points in the GPX 1.0 format of the navilink handheld,
// as a single track with a single segment, and returns the file path.
func writeDeviceGPX(tb testing.TB, name string, pts []devicePoint) string {
	tb.Helper()
	num := func(v float64) string { return strconv.FormatFloat(math.Round(v*1e7)/1e7, 'f', -1, 64) }

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<gpx xmlns="http://www.topografix.com/GPX/1/0" version="1.0" creator="navilink">
  <trk>
    <trkseg>
`)
	for _, p := range pts {
		fmt.Fprintf(&b, `      <trkpt lat="%s" lon="%s">
        <ele>%s</ele>
        <time>%s</time>
        <speed>%s</speed>
      </trkpt>
`, num(p.lat), num(p.lon), num(p.ele), p.time.Format(time.RFC3339), strconv.FormatFloat(p.speed, 'f', -1, 64))
	}
	b.WriteString("    </trkseg>\n  </trk>\n</gpx>\n")

	path := filepath.Join(tb.TempDir(), name)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		tb.Fatal(err)
	}
	return path
}

var deviceStart = time.Date(2026, 10, 7, 9, 52, 59, 0, time.UTC)

// Indices of the flaws in generated device files.
const (
	logStationary = 20  // points standing still with GPS jitter at the start of a log
	logBackwards  = 150 // the time of this log point is 3 s before the previous one
	pauseAt       = 300 // a 37 minute pause starts after this point, without a new segment
)

// deviceLog generates a 1 Hz log like the handheld writes it: full coordinate precision,
// standing still with jitter at the start, a time step backwards, and a pause within the segment.
func deviceLog(n int) []devicePoint {
	pts := make([]devicePoint, n)
	tm := deviceStart
	lat, lon := 51.3288433, 12.4086787
	for i := range pts {
		speed := 5.27 + float64(i%7)*0.13
		if i < logStationary {
			speed = 0.01 * float64(1+i%9)
			lat += 1e-7 * float64(i%3-1)
		} else {
			lat += speed / 111_320
			lon += speed / 69_600
		}
		pts[i] = devicePoint{lat: lat, lon: lon, ele: 52.2 + float64(i%50)*0.37, speed: speed, time: tm}
		switch i {
		case logBackwards - 1:
			tm = tm.Add(-3 * time.Second)
		case pauseAt:
			tm = tm.Add(37 * time.Minute)
		default:
			tm = tm.Add(time.Second)
		}
	}
	return pts
}

// deviceTrack generates a track like the handheld writes it: a stale first fix,
// 2 s sampling, coordinates quantized to 0.0001 arc minutes, speed quantized to 2 km/h,
// and a pause within the segment.
func deviceTrack(n int) []devicePoint {
	pts := make([]devicePoint, n)
	pts[0] = devicePoint{lat: 51.331265, lon: 12.4111633, ele: 0, speed: 0, time: deviceStart.Add(-10 * time.Minute)}
	tm := deviceStart
	lat, lon := 51.3289883, 12.4087516
	for i := 1; i < n; i++ {
		speed := float64(7+i%5) * 2 / 3.6
		pts[i] = devicePoint{
			lat:   quantizeMinutes(lat),
			lon:   quantizeMinutes(lon),
			ele:   77.42 + float64(i%40)*0.5,
			speed: math.Round(speed*100) / 100,
			time:  tm,
		}
		lat += 2 * speed / 111_320
		lon += 2 * speed / 69_600
		if i == pauseAt {
			tm = tm.Add(37 * time.Minute)
		} else {
			tm = tm.Add(2 * time.Second)
		}
	}
	return pts
}

// quantizeMinutes rounds degrees to 0.0001 arc minutes.
func quantizeMinutes(deg float64) float64 {
	return math.Round(deg*600_000) / 600_000
}

// readDeviceFile writes the points, reads them back and checks that the result
// is a single segment with exactly the written points.
func readDeviceFile(t *testing.T, name string, pts []devicePoint) *Segment {
	t.Helper()
	path := writeDeviceGPX(t, name, pts)
	f, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.Creator != "navilink" || f.Format != FormatGPX || f.Path != path || f.Name != "" || !f.Time.IsZero() {
		t.Errorf("unexpected file info %+v", f)
	}
	if len(f.Tracks) != 1 || len(f.Tracks[0].Segments) != 1 {
		t.Fatalf("expected 1 track with 1 segment")
	}
	s := &f.Tracks[0].Segments[0]
	if s.Len() != len(pts) {
		t.Fatalf("expected %d points, got %d", len(pts), s.Len())
	}
	if !s.Has(Speed) || s.Has(HeartRate) || s.Has(HDOP) {
		t.Errorf("unexpected channels")
	}
	speed := s.Channel(Speed)
	for i, p := range s.Points {
		want := pts[i]
		if math.Abs(p.Pos.Lat-want.lat) > 1e-7 || math.Abs(p.Pos.Lon-want.lon) > 1e-7 ||
			math.Abs(p.Ele-want.ele) > 1e-7 || !p.Time.Equal(want.time) || speed[i] != want.speed {
			t.Fatalf("point %d: got %+v speed %v, want %+v", i, p, speed[i], want)
		}
	}
	return s
}

func TestReadGPXDeviceLog(t *testing.T) {
	s := readDeviceFile(t, "log.gpx", deviceLog(400))
	pts := s.Points

	// Flaws are kept as recorded.
	if d := pts[logBackwards].Time.Sub(pts[logBackwards-1].Time); d != -3*time.Second {
		t.Errorf("expected time step of -3s, got %v", d)
	}
	if d := pts[pauseAt+1].Time.Sub(pts[pauseAt].Time); d != 37*time.Minute {
		t.Errorf("expected pause of 37m, got %v", d)
	}
	if v := s.Channel(Speed)[0]; v != 0.01 {
		t.Errorf("expected jitter speed 0.01, got %v", v)
	}
}

func TestReadGPXDeviceTrack(t *testing.T) {
	s := readDeviceFile(t, "track.gpx", deviceTrack(400))
	pts := s.Points

	// Flaws are kept as recorded.
	if p := pts[0]; !p.HasEle() || p.Ele != 0 || pts[1].Time.Sub(p.Time) != 10*time.Minute {
		t.Errorf("expected stale first fix, got %+v", p)
	}
	if d := pts[pauseAt+1].Time.Sub(pts[pauseAt].Time); d != 37*time.Minute {
		t.Errorf("expected pause of 37m, got %v", d)
	}
	for i, p := range pts[1:] {
		if math.Abs(p.Pos.Lat-quantizeMinutes(p.Pos.Lat)) > 1e-7 {
			t.Fatalf("point %d: latitude %v not quantized", i+1, p.Pos.Lat)
		}
	}
	for i, v := range s.Channel(Speed) {
		if q := math.Round(math.Round(v*3.6/2)*2/3.6*100) / 100; v != q {
			t.Fatalf("point %d: speed %v not quantized to 2 km/h", i, v)
		}
	}
}

const gpx11 = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test" xmlns="http://www.topografix.com/GPX/1/1"
     xmlns:gpxtpx="http://www.garmin.com/xmlschemas/TrackPointExtension/v2">
  <metadata><name> Ride </name><time>2026-10-07T11:00:00+02:00</time></metadata>
  <wpt lat="51.5" lon="12.5"><ele>100</ele><name>Hut</name><sym>Lodge</sym></wpt>
  <rte><name>Plan</name>
    <rtept lat="51.0" lon="12.0"/>
    <rtept lat="51.1" lon="12.1"><name>Pass</name></rtept>
  </rte>
  <trk><name>Morning</name><type>cycling</type>
    <trkseg>
      <trkpt lat="51.0" lon="12.0"><ele>110.5</ele><time>2026-10-07T09:00:00.5Z</time>
        <extensions><gpxtpx:TrackPointExtension>
          <gpxtpx:atemp>15.5</gpxtpx:atemp><gpxtpx:hr>120</gpxtpx:hr><gpxtpx:cad>80</gpxtpx:cad><gpxtpx:speed>5.5</gpxtpx:speed>
        </gpxtpx:TrackPointExtension><power>250</power></extensions>
      </trkpt>
      <trkpt lat="-51.0" lon="-12.0"><ele></ele><time>garbage</time><hdop>1.5</hdop></trkpt>
    </trkseg>
    <trkseg><trkpt lat="51.2" lon="12.2"/></trkseg>
  </trk>
</gpx>`

func TestReadGPX11(t *testing.T) {
	f, err := ReadGPX(strings.NewReader(gpx11))
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "Ride" || !f.Time.Equal(time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("unexpected metadata %q %v", f.Name, f.Time)
	}
	if len(f.Waypoints) != 1 || f.Waypoints[0].Name != "Hut" || f.Waypoints[0].Symbol != "Lodge" || f.Waypoints[0].Ele != 100 {
		t.Errorf("unexpected waypoints %+v", f.Waypoints)
	}
	if len(f.Routes) != 1 || len(f.Routes[0].Points) != 2 || f.Routes[0].Points[1].Name != "Pass" || !math.IsNaN(f.Routes[0].Points[0].Ele) {
		t.Errorf("unexpected routes %+v", f.Routes)
	}
	if len(f.Tracks) != 1 || len(f.Tracks[0].Segments) != 2 {
		t.Fatalf("expected 1 track with 2 segments")
	}
	tr := f.Tracks[0]
	if tr.Name != "Morning" || tr.Type != "cycling" {
		t.Errorf("unexpected track info %q %q", tr.Name, tr.Type)
	}

	s := &tr.Segments[0]
	p0, p1 := s.Points[0], s.Points[1]
	if p0.Pos.Lat != 51 || p0.Pos.Lon != 12 || p0.Ele != 110.5 || p0.Time.Nanosecond() != 5e8 {
		t.Errorf("unexpected point %+v", p0)
	}
	if p1.Pos.Lat != -51 || p1.Pos.Lon != -12 || p1.HasEle() || p1.HasTime() {
		t.Errorf("expected point without elevation and time, got %+v", p1)
	}
	for c, want := range map[Channel]float64{Temperature: 15.5, HeartRate: 120, Cadence: 80, Speed: 5.5, Power: 250} {
		if v := s.Channel(c); len(v) != 2 || v[0] != want || !math.IsNaN(v[1]) {
			t.Errorf("%v: unexpected values %v", c, v)
		}
	}
	if v := s.Channel(HDOP); len(v) != 2 || !math.IsNaN(v[0]) || v[1] != 1.5 {
		t.Errorf("hdop: unexpected values %v", v)
	}
	if s.Has(Satellites) || tr.Segments[1].Has(HeartRate) {
		t.Errorf("unexpected channels")
	}
}

func TestReadGPXCharset(t *testing.T) {
	doc := []byte(`<?xml version="1.0" encoding="ISO-8859-1"?><gpx version="1.0"><name>M` + "\xfc" + `hle</name></gpx>`)
	f, err := ReadGPX(bytes.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "Mühle" {
		t.Errorf("expected Mühle, got %q", f.Name)
	}
}

func TestReadGPXErrors(t *testing.T) {
	for _, doc := range []string{
		`<kml></kml>`,
		`<gpx><trk><trkseg><trkpt lat="x" lon="12"/></trkseg></trk></gpx>`,
		`<gpx><wpt lat="91" lon="12"/></gpx>`,
		`<gpx><trk>`,
	} {
		if _, err := ReadGPX(strings.NewReader(doc)); err == nil {
			t.Errorf("expected error for %s", doc)
		}
	}
	if _, err := ReadFile("track.fit"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("expected ErrUnsupported, got %v", err)
	}
}

func BenchmarkReadGPX(b *testing.B) {
	path := writeDeviceGPX(b, "log.gpx", deviceLog(10_000))
	for b.Loop() {
		if _, err := ReadFile(path); err != nil {
			b.Fatal(err)
		}
	}
}

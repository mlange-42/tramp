package track

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Creator is written as the creator of GPX files.
const Creator = "TRAMP"

// WriteGPX writes the file as GPX 1.1.
//
// Track point channels go into the extensions that [ReadGPX] understands:
// Garmin TrackPointExtension v2 for temperature, heart rate, cadence, speed and course,
// Garmin PowerExtension for power and Cluetrust gpxdata for distance.
// Laps are not written, as GPX has no place for them.
func WriteGPX(w io.Writer, f *File) error {
	g := gpxWriter{w: bufio.NewWriter(w)}
	g.str(xml.Header)
	g.str(`<gpx version="1.1" creator="` + Creator + `"` +
		` xmlns="http://www.topografix.com/GPX/1/1"` +
		` xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"` +
		` xsi:schemaLocation="http://www.topografix.com/GPX/1/1 http://www.topografix.com/GPX/1/1/gpx.xsd"`)
	if hasChannels(f, Temperature, HeartRate, Cadence, Speed, Course) {
		g.str(` xmlns:gpxtpx="http://www.garmin.com/xmlschemas/TrackPointExtension/v2"`)
	}
	if hasChannels(f, Power) {
		g.str(` xmlns:pwr="http://www.garmin.com/xmlschemas/PowerExtension/v1"`)
	}
	if hasChannels(f, Distance) {
		g.str(` xmlns:gpxdata="http://www.cluetrust.com/XML/GPXDATA/1/0"`)
	}
	g.str(">\n")

	if f.Name != "" || f.Desc != "" || !f.Time.IsZero() {
		g.str("  <metadata>\n")
		g.elem("    ", "name", f.Name)
		g.elem("    ", "desc", f.Desc)
		g.time("    ", f.Time)
		g.str("  </metadata>\n")
	}
	for i := range f.Waypoints {
		g.waypoint("  ", "wpt", &f.Waypoints[i])
	}
	for i := range f.Routes {
		r := &f.Routes[i]
		g.str("  <rte>\n")
		g.elem("    ", "name", r.Name)
		g.elem("    ", "desc", r.Desc)
		g.elem("    ", "type", r.Type)
		for j := range r.Points {
			g.waypoint("    ", "rtept", &r.Points[j])
		}
		g.str("  </rte>\n")
	}
	for i := range f.Tracks {
		t := &f.Tracks[i]
		g.str("  <trk>\n")
		g.elem("    ", "name", t.Name)
		g.elem("    ", "desc", t.Desc)
		g.elem("    ", "type", t.Type)
		for j := range t.Segments {
			g.segment(&t.Segments[j])
		}
		g.str("  </trk>\n")
	}
	g.str("</gpx>\n")

	if g.err != nil {
		return g.err
	}
	return g.w.Flush()
}

// WriteFile writes the file as GPX to the given path.
// It writes to a temporary file first and replaces the target only when done,
// so that a failed write leaves the old file intact.
func WriteFile(path string, f *File) (err error) {
	if ext := strings.ToLower(filepath.Ext(path)); ext != ".gpx" {
		return fmt.Errorf("%s: %w", path, ErrUnsupported)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if err = WriteGPX(tmp, f); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// hasChannels reports whether any track segment of the file has one of the channels.
func hasChannels(f *File, cs ...Channel) bool {
	for i := range f.Tracks {
		for j := range f.Tracks[i].Segments {
			for _, c := range cs {
				if f.Tracks[i].Segments[j].Has(c) {
					return true
				}
			}
		}
	}
	return false
}

// gpxWriter writes GPX elements, keeping the first error.
type gpxWriter struct {
	w   *bufio.Writer
	err error
}

func (g *gpxWriter) str(s string) {
	if g.err == nil {
		_, g.err = g.w.WriteString(s)
	}
}

// elem writes a text element on its own line, if the text is not empty.
func (g *gpxWriter) elem(indent, name, text string) {
	if text == "" {
		return
	}
	g.str(indent + "<" + name + ">")
	if g.err == nil {
		g.err = xml.EscapeText(g.w, []byte(text))
	}
	g.str("</" + name + ">\n")
}

func (g *gpxWriter) num(indent, name string, v float64) {
	if !math.IsNaN(v) {
		g.str(indent + "<" + name + ">" + formatNum(v) + "</" + name + ">\n")
	}
}

func (g *gpxWriter) time(indent string, t time.Time) {
	if !t.IsZero() {
		g.str(indent + "<time>" + t.Format(time.RFC3339Nano) + "</time>\n")
	}
}

func (g *gpxWriter) waypoint(indent, tag string, w *Waypoint) {
	g.str(indent + "<" + tag + ` lat="` + formatNum(w.Pos.Lat) + `" lon="` + formatNum(w.Pos.Lon) + `"`)
	if math.IsNaN(w.Ele) && w.Time.IsZero() && w.Name == "" && w.Desc == "" && w.Symbol == "" && w.Type == "" {
		g.str("/>\n")
		return
	}
	g.str(">\n")
	in := indent + "  "
	g.num(in, "ele", w.Ele)
	g.time(in, w.Time)
	g.elem(in, "name", w.Name)
	g.elem(in, "desc", w.Desc)
	g.elem(in, "sym", w.Symbol)
	g.elem(in, "type", w.Type)
	g.str(indent + "</" + tag + ">\n")
}

// tpxChannels are the channels in the Garmin TrackPointExtension, in schema order.
var tpxChannels = [...]struct {
	c    Channel
	name string
}{{Temperature, "atemp"}, {HeartRate, "hr"}, {Cadence, "cad"}, {Speed, "speed"}, {Course, "course"}}

func (g *gpxWriter) segment(s *Segment) {
	g.str("    <trkseg>\n")
	val := func(c Channel, i int) float64 {
		if ch := s.channels[c]; ch != nil {
			return ch[i]
		}
		return math.NaN()
	}
	for i := range s.Points {
		p := &s.Points[i]
		g.str(`      <trkpt lat="` + formatNum(p.Pos.Lat) + `" lon="` + formatNum(p.Pos.Lon) + `">`)
		g.num("", "ele", p.Ele)
		g.time("", p.Time)
		if v := val(Satellites, i); !math.IsNaN(v) {
			g.str("<sat>" + strconv.Itoa(int(math.Round(v))) + "</sat>")
		}
		g.num("", "hdop", val(HDOP, i))

		var tpx strings.Builder
		for _, t := range tpxChannels {
			if v := val(t.c, i); !math.IsNaN(v) {
				tpx.WriteString("<gpxtpx:" + t.name + ">" + formatNum(v) + "</gpxtpx:" + t.name + ">")
			}
		}
		power, dist := val(Power, i), val(Distance, i)
		if tpx.Len() > 0 || !math.IsNaN(power) || !math.IsNaN(dist) {
			g.str("<extensions>")
			if tpx.Len() > 0 {
				g.str("<gpxtpx:TrackPointExtension>" + tpx.String() + "</gpxtpx:TrackPointExtension>")
			}
			if !math.IsNaN(power) {
				g.str("<pwr:PowerInWatts>" + formatNum(power) + "</pwr:PowerInWatts>")
			}
			if !math.IsNaN(dist) {
				g.str("<gpxdata:distance>" + formatNum(dist) + "</gpxdata:distance>")
			}
			g.str("</extensions>")
		}
		g.str("</trkpt>\n")
	}
	g.str("    </trkseg>\n")
}

// formatNum formats a number with the fewest digits that read back to the same value.
func formatNum(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

package track

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html/charset"

	"github.com/mlange-42/tramp/internal/geo"
)

// ReadGPX reads a GPX 1.0 or 1.1 file.
//
// Elements are matched by name regardless of namespace, so both versions and
// extensions from any vendor namespace are understood.
func ReadGPX(r io.Reader) (*File, error) {
	d := xml.NewDecoder(r)
	d.CharsetReader = charset.NewReaderLabel

	var doc gpxDoc
	if err := d.Decode(&doc); err != nil {
		return nil, fmt.Errorf("reading GPX: %w", err)
	}
	if doc.XMLName.Local != "gpx" {
		return nil, fmt.Errorf("reading GPX: root element is <%s>, not <gpx>", doc.XMLName.Local)
	}

	f := &File{
		Format:  FormatGPX,
		Creator: doc.Creator,
		Name:    text(doc.Name),
		Desc:    text(doc.Desc),
		Time:    parseTime(doc.Time),
	}
	if m := doc.Metadata; m != nil {
		f.Name = text(m.Name)
		f.Desc = text(m.Desc)
		f.Time = parseTime(m.Time)
	}

	var err error
	f.Waypoints, err = convertWaypoints(doc.Wpts)
	if err != nil {
		return nil, err
	}
	for _, r := range doc.Rtes {
		pts, err := convertWaypoints(r.Pts)
		if err != nil {
			return nil, err
		}
		f.Routes = append(f.Routes, Route{Name: text(r.Name), Desc: text(r.Desc), Type: text(r.Type), Points: pts})
	}
	for _, t := range doc.Trks {
		tr := Track{Name: text(t.Name), Desc: text(t.Desc), Type: text(t.Type)}
		for _, s := range t.Segs {
			seg, err := convertSegment(s.Pts)
			if err != nil {
				return nil, err
			}
			tr.Segments = append(tr.Segments, seg)
		}
		f.Tracks = append(f.Tracks, tr)
	}
	return f, nil
}

type gpxDoc struct {
	XMLName  xml.Name
	Creator  string       `xml:"creator,attr"`
	Name     string       `xml:"name"` // GPX 1.0
	Desc     string       `xml:"desc"` // GPX 1.0
	Time     string       `xml:"time"` // GPX 1.0
	Metadata *gpxMetadata `xml:"metadata"`
	Wpts     []gpxPoint   `xml:"wpt"`
	Rtes     []gpxRoute   `xml:"rte"`
	Trks     []gpxTrack   `xml:"trk"`
}

type gpxMetadata struct {
	Name string `xml:"name"`
	Desc string `xml:"desc"`
	Time string `xml:"time"`
}

type gpxRoute struct {
	Name string     `xml:"name"`
	Desc string     `xml:"desc"`
	Type string     `xml:"type"`
	Pts  []gpxPoint `xml:"rtept"`
}

type gpxTrack struct {
	Name string       `xml:"name"`
	Desc string       `xml:"desc"`
	Type string       `xml:"type"`
	Segs []gpxSegment `xml:"trkseg"`
}

type gpxSegment struct {
	Pts []gpxPoint `xml:"trkpt"`
}

// gpxPoint is a wpt, rtept or trkpt. Values are kept as strings,
// so that missing and malformed values can be told apart from zero.
type gpxPoint struct {
	Lat    string        `xml:"lat,attr"`
	Lon    string        `xml:"lon,attr"`
	Ele    string        `xml:"ele"`
	Time   string        `xml:"time"`
	Speed  string        `xml:"speed"`  // GPX 1.0
	Course string        `xml:"course"` // GPX 1.0
	Sat    string        `xml:"sat"`
	HDOP   string        `xml:"hdop"`
	Name   string        `xml:"name"`
	Desc   string        `xml:"desc"`
	Sym    string        `xml:"sym"`
	Type   string        `xml:"type"`
	Ext    gpxExtensions `xml:"extensions"`
}

func (p *gpxPoint) pos() (geo.LonLat, error) {
	lat, err1 := strconv.ParseFloat(strings.TrimSpace(p.Lat), 64)
	lon, err2 := strconv.ParseFloat(strings.TrimSpace(p.Lon), 64)
	if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return geo.LonLat{}, fmt.Errorf("reading GPX: invalid position lat=%q lon=%q", p.Lat, p.Lon)
	}
	return geo.LonLat{Lon: lon, Lat: lat}, nil
}

// gpxExtensions collects the text of all leaf elements in <extensions>, by lower-case local name.
// Nesting and namespaces are ignored, as vendors put the same values in different places.
type gpxExtensions map[string]string

func (e *gpxExtensions) UnmarshalXML(d *xml.Decoder, _ xml.StartElement) error {
	var buf []byte
	leaf := false
	for depth := 0; ; {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			leaf = true
			buf = buf[:0]
		case xml.CharData:
			buf = append(buf, t...)
		case xml.EndElement:
			if depth == 0 {
				return nil
			}
			depth--
			if leaf {
				if *e == nil {
					*e = gpxExtensions{}
				}
				(*e)[strings.ToLower(t.Name.Local)] = strings.TrimSpace(string(buf))
			}
			leaf = false
		}
	}
}

// extChannels maps extension element names to channels.
// Covers Garmin TrackPointExtension v1/v2, Cluetrust gpxdata and common power extensions.
var extChannels = map[string]Channel{
	"speed":        Speed,
	"course":       Course,
	"distance":     Distance,
	"hr":           HeartRate,
	"heartrate":    HeartRate,
	"cad":          Cadence,
	"cadence":      Cadence,
	"power":        Power,
	"powerinwatts": Power,
	"watts":        Power,
	"atemp":        Temperature,
	"temp":         Temperature,
	"temperature":  Temperature,
}

func convertWaypoints(pts []gpxPoint) ([]Waypoint, error) {
	if len(pts) == 0 {
		return nil, nil
	}
	wps := make([]Waypoint, len(pts))
	for i := range pts {
		p := &pts[i]
		pos, err := p.pos()
		if err != nil {
			return nil, err
		}
		wps[i] = Waypoint{
			Pos:    pos,
			Ele:    parseFloat(p.Ele),
			Time:   parseTime(p.Time),
			Name:   text(p.Name),
			Desc:   text(p.Desc),
			Symbol: text(p.Sym),
			Type:   text(p.Type),
		}
	}
	return wps, nil
}

func convertSegment(pts []gpxPoint) (Segment, error) {
	seg := Segment{Points: make([]Point, len(pts))}
	for i := range pts {
		p := &pts[i]
		pos, err := p.pos()
		if err != nil {
			return Segment{}, err
		}
		seg.Points[i] = Point{Pos: pos, Ele: parseFloat(p.Ele), Time: parseTime(p.Time)}
	}
	for i := range pts {
		p := &pts[i]
		setChannel(&seg, Speed, i, p.Speed)
		setChannel(&seg, Course, i, p.Course)
		setChannel(&seg, Satellites, i, p.Sat)
		setChannel(&seg, HDOP, i, p.HDOP)
		for name, v := range p.Ext {
			if c, ok := extChannels[name]; ok && (seg.channels[c] == nil || math.IsNaN(seg.channels[c][i])) {
				setChannel(&seg, c, i, v)
			}
		}
	}
	return seg, nil
}

func setChannel(s *Segment, c Channel, i int, v string) {
	if f := parseFloat(v); !math.IsNaN(f) {
		s.SetChannel(c, i, f)
	}
}

// parseFloat parses a number, returning NaN if it is missing or malformed.
func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return math.NaN()
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(v, 0) {
		return math.NaN()
	}
	return v
}

// parseTime parses an ISO 8601 time, returning the zero time if it is missing or malformed.
// Times without a zone are taken as UTC.
func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999", "2006-01-02 15:04:05.999999999Z07:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func text(s string) string { return strings.TrimSpace(s) }

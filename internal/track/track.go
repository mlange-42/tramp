// Package track provides the format-independent representation of GPS tracks, routes and waypoints,
// and readers for the supported file formats.
//
// The model covers what GPX, KML/KMZ and FIT files can contain:
//
//   - GPX: trk → [Track], trkseg → [Segment], rte → [Route], wpt → [Waypoint].
//     Point fields like speed, hdop and sat, and Garmin-style extensions (hr, cad, atemp, power, ...)
//     become [Channel] values.
//   - KML/KMZ: gx:Track and LineString placemarks → [Track] (each gx:Track of a gx:MultiTrack is a [Segment];
//     LineString points have no time), ExtendedData arrays → [Channel] values, Point placemarks → [Waypoint].
//   - FIT: record messages → points with [Channel] values, timer stop/start events split [Segment]s,
//     lap messages → [Lap], the session's sport → [Track.Type], course files → [Route], course points → [Waypoint].
//
// Readers keep the data as recorded. Cleaning like sorting by time, dropping stale fixes,
// or splitting at pauses within a segment is left to later processing.
package track

import (
	"math"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
)

// Format is a supported file format.
type Format int

// Supported file formats.
const (
	FormatGPX Format = iota
	FormatKML
	FormatKMZ
	FormatFIT
)

var formatNames = [...]string{"GPX", "KML", "KMZ", "FIT"}

func (f Format) String() string {
	if f < 0 || int(f) >= len(formatNames) {
		return "unknown"
	}
	return formatNames[f]
}

// File is the content of one track file.
type File struct {
	// Path is the file the data was read from, if any.
	Path string
	// Format is the format the data was read from.
	Format Format
	// Name and Desc are the name and description of the file as a whole, if given.
	Name, Desc string
	// Creator is the software or device that wrote the file, if given.
	Creator string
	// Time is the creation time of the file. Zero if not given.
	Time time.Time

	// Tracks are recorded tracks.
	Tracks []Track
	// Routes are planned routes.
	Routes []Route
	// Waypoints are standalone points of interest.
	Waypoints []Waypoint
}

// Track is a recorded track, consisting of one or more segments.
type Track struct {
	// Name and Desc are the name and description of the track, if given.
	Name, Desc string
	// Type is the activity type, like "cycling" or "hiking", as given in the file.
	Type string
	// Segments are the continuous parts of the track.
	// A new segment starts where recording was paused or the signal was lost,
	// if the file marks it.
	Segments []Segment
	// Laps are the laps of the activity. Only FIT files have laps.
	Laps []Lap
}

// Lap is a lap of an activity, as a time interval.
type Lap struct {
	Start, End time.Time
}

// Point is a recorded track point.
type Point struct {
	// Pos is the position.
	Pos geo.LonLat
	// Ele is the elevation in meters above sea level. NaN if not known.
	Ele float64
	// Time is the time of the fix. Zero if not known.
	Time time.Time
}

// HasEle reports whether the point has an elevation.
func (p *Point) HasEle() bool { return !math.IsNaN(p.Ele) }

// HasTime reports whether the point has a time.
func (p *Point) HasTime() bool { return !p.Time.IsZero() }

// Channel is a per-point value recorded in addition to position, elevation and time.
type Channel int

// Recorded channels, all in SI units unless noted.
const (
	// Speed is the speed over ground in m/s, as reported by the device.
	Speed Channel = iota
	// Course is the direction of movement in degrees clockwise from north.
	Course
	// Distance is the cumulative distance in m, as reported by the device.
	Distance
	// HeartRate is the heart rate in beats per minute.
	HeartRate
	// Cadence is the cadence in revolutions or steps per minute.
	Cadence
	// Power is the power in W.
	Power
	// Temperature is the ambient temperature in °C.
	Temperature
	// HDOP is the horizontal dilution of precision.
	HDOP
	// Satellites is the number of satellites used for the fix.
	Satellites

	// NumChannels is the number of channels.
	NumChannels
)

var channelNames = [...]string{"speed", "course", "distance", "heart rate", "cadence", "power", "temperature", "hdop", "satellites"}

func (c Channel) String() string {
	if c < 0 || c >= NumChannels {
		return "unknown"
	}
	return channelNames[c]
}

// Segment is a continuous part of a track.
type Segment struct {
	// Points are the track points in recorded order.
	Points []Point
	// channels holds the recorded channels, nil if not recorded, otherwise one value per point (NaN if missing).
	channels [NumChannels][]float64
}

// Len returns the number of points.
func (s *Segment) Len() int { return len(s.Points) }

// Channel returns the values of a channel, one per point, with NaN for points without a value.
// Returns nil if no point has the channel.
func (s *Segment) Channel(c Channel) []float64 {
	return s.channels[c]
}

// Has reports whether any point has a value for the channel.
func (s *Segment) Has(c Channel) bool {
	return s.channels[c] != nil
}

// SetChannel sets the value of a channel for point i.
// Points must be added before setting channel values.
func (s *Segment) SetChannel(c Channel, i int, v float64) {
	ch := s.channels[c]
	if len(ch) < len(s.Points) {
		n := len(ch)
		ch = append(ch, make([]float64, len(s.Points)-n)...)
		for j := n; j < len(ch); j++ {
			ch[j] = math.NaN()
		}
		s.channels[c] = ch
	}
	ch[i] = v
}

// Route is a planned route, as a sequence of points to visit.
type Route struct {
	// Name and Desc are the name and description of the route, if given.
	Name, Desc string
	// Type is the activity type, if given.
	Type string
	// Points are the route points in order.
	Points []Waypoint
}

// Waypoint is a named point, standalone or as part of a route.
type Waypoint struct {
	// Pos is the position.
	Pos geo.LonLat
	// Ele is the elevation in meters above sea level. NaN if not known.
	Ele float64
	// Time is the time the point was recorded or created. Zero if not known.
	Time time.Time
	// Name, Desc, Symbol and Type are the name, description, symbol name and type, if given.
	Name, Desc, Symbol, Type string
}

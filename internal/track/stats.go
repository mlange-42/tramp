package track

import (
	"slices"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
)

// Length returns the length of the segment along its points in meters.
func (s *Segment) Length() float64 {
	d := 0.0
	for i := 1; i < len(s.Points); i++ {
		d += geo.Distance(s.Points[i-1].Pos, s.Points[i].Pos)
	}
	return d
}

// Length returns the summed length of all segments in meters.
// Gaps between segments are not counted.
func (t *Track) Length() float64 {
	d := 0.0
	for i := range t.Segments {
		d += t.Segments[i].Length()
	}
	return d
}

// TimeSpan returns the earliest and latest point time of the track.
// Both are zero if no point has a time.
func (t *Track) TimeSpan() (start, end time.Time) {
	for i := range t.Segments {
		for _, p := range t.Segments[i].Points {
			if !p.HasTime() {
				continue
			}
			if start.IsZero() || p.Time.Before(start) {
				start = p.Time
			}
			if end.IsZero() || p.Time.After(end) {
				end = p.Time
			}
		}
	}
	return start, end
}

// Interval returns the median time between consecutive points with increasing times.
// Zero if there are no such points.
func (t *Track) Interval() time.Duration {
	var dts []time.Duration
	for i := range t.Segments {
		pts := t.Segments[i].Points
		for j := 1; j < len(pts); j++ {
			if dt := pts[j].Time.Sub(pts[j-1].Time); pts[j-1].HasTime() && pts[j].HasTime() && dt > 0 {
				dts = append(dts, dt)
			}
		}
	}
	if len(dts) == 0 {
		return 0
	}
	slices.Sort(dts)
	return dts[len(dts)/2]
}

// NumPoints returns the number of points in all segments.
func (t *Track) NumPoints() int {
	n := 0
	for i := range t.Segments {
		n += t.Segments[i].Len()
	}
	return n
}

// Length returns the length of the route along its points in meters.
func (r *Route) Length() float64 {
	d := 0.0
	for i := 1; i < len(r.Points); i++ {
		d += geo.Distance(r.Points[i-1].Pos, r.Points[i].Pos)
	}
	return d
}

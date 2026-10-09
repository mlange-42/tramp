package track

import (
	"math"

	"github.com/mlange-42/tramp/internal/geo"
)

// Metric is a value along a track, for coloring it.
type Metric int

// Metrics for coloring tracks.
const (
	// NoMetric means no coloring by value.
	NoMetric Metric = iota
	// SpeedMetric is the speed in m/s, derived from positions and times.
	SpeedMetric
	// ElevationMetric is the elevation in m.
	ElevationMetric
	// SlopeMetric is the slope in percent, positive uphill.
	SlopeMetric

	// NumMetrics is the number of metrics, including [NoMetric].
	NumMetrics
)

const (
	// speedWindow is the time span over which speed is averaged, in seconds.
	// GPS positions jitter by a few meters, so speeds between single 1 s fixes are noisy.
	speedWindow = 10
	// slopeWindow is the distance over which slope is averaged, in meters.
	// GPS elevation is much noisier than position, so slopes over short distances are meaningless.
	slopeWindow = 100
	// minSlopeDistance is the minimum distance for a slope value, in meters.
	minSlopeDistance = 10
)

// SegmentValues returns a value of the metric for each line segment between consecutive points,
// with NaN where it is not known. It returns nil if no segment has a value,
// e.g. speed for points without times.
func SegmentValues(pts []Point, m Metric) []float64 {
	if len(pts) < 2 {
		return nil
	}
	var vals []float64
	switch m {
	case SpeedMetric:
		vals = speedValues(pts, cumulativeDistance(pts))
	case ElevationMetric:
		vals = elevationValues(pts)
	case SlopeMetric:
		vals = slopeValues(pts, cumulativeDistance(pts))
	default:
		return nil
	}
	for _, v := range vals {
		if !math.IsNaN(v) {
			return vals
		}
	}
	return nil
}

// cumulativeDistance returns the distance along the points from the first, for each point.
func cumulativeDistance(pts []Point) []float64 {
	cum := make([]float64, len(pts))
	for i := 1; i < len(pts); i++ {
		cum[i] = cum[i-1] + geo.Distance(pts[i-1].Pos, pts[i].Pos)
	}
	return cum
}

// speedValues returns the average speed in a time window around each segment.
// The window only extends over points with increasing times.
func speedValues(pts []Point, cum []float64) []float64 {
	vals := make([]float64, len(pts)-1)
	for j := range vals {
		vals[j] = math.NaN()
		a, b := pts[j], pts[j+1]
		if !a.HasTime() || !b.HasTime() || !b.Time.After(a.Time) {
			continue
		}
		mid := a.Time.Add(b.Time.Sub(a.Time) / 2)
		l, r := j, j+1
		for l > 0 && pts[l-1].HasTime() && pts[l-1].Time.Before(pts[l].Time) && mid.Sub(pts[l-1].Time).Seconds() <= speedWindow/2 {
			l--
		}
		for r < len(pts)-1 && pts[r+1].HasTime() && pts[r+1].Time.After(pts[r].Time) && pts[r+1].Time.Sub(mid).Seconds() <= speedWindow/2 {
			r++
		}
		vals[j] = (cum[r] - cum[l]) / pts[r].Time.Sub(pts[l].Time).Seconds()
	}
	return vals
}

// elevationValues returns the mean elevation of the ends of each segment.
func elevationValues(pts []Point) []float64 {
	vals := make([]float64, len(pts)-1)
	for j := range vals {
		a, b := pts[j].Ele, pts[j+1].Ele
		switch {
		case math.IsNaN(a):
			vals[j] = b
		case math.IsNaN(b):
			vals[j] = a
		default:
			vals[j] = (a + b) / 2
		}
	}
	return vals
}

// slopeValues returns the slope over a distance window around each segment.
// The window only extends over points with elevation.
func slopeValues(pts []Point, cum []float64) []float64 {
	vals := make([]float64, len(pts)-1)
	for j := range vals {
		vals[j] = math.NaN()
		if !pts[j].HasEle() || !pts[j+1].HasEle() {
			continue
		}
		mid := (cum[j] + cum[j+1]) / 2
		l, r := j, j+1
		for l > 0 && pts[l-1].HasEle() && mid-cum[l-1] <= slopeWindow/2 {
			l--
		}
		for r < len(pts)-1 && pts[r+1].HasEle() && cum[r+1]-mid <= slopeWindow/2 {
			r++
		}
		if d := cum[r] - cum[l]; d >= minSlopeDistance {
			vals[j] = (pts[r].Ele - pts[l].Ele) / d * 100
		}
	}
	return vals
}

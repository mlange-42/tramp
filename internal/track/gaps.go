package track

import (
	"slices"
	"time"
)

const (
	// GapMinDuration is the shortest time between consecutive points that counts as a gap in the recording.
	GapMinDuration = time.Minute
	// GapFactor is how many times longer than the median time between points a gap is at least.
	// It keeps tracks recorded at long intervals, like Garmin's smart recording, from being cut up.
	GapFactor = 5
)

// Gaps marks the segments of lines that are gaps in the recording, e.g. where it was paused or lost the fix:
// where the time between the points is longer than [GapMinDuration],
// and longer than [GapFactor] times the median time between consecutive points of all lines.
// There is one entry per line, which is nil if the line has no gaps.
// It returns nil if there are no gaps. Segments with a point without time are never gaps.
func Gaps(lines [][]Point) [][]bool {
	var steps []time.Duration
	for _, pts := range lines {
		for i := 1; i < len(pts); i++ {
			if dt, ok := timeStep(pts[i-1], pts[i]); ok && dt > 0 {
				steps = append(steps, dt)
			}
		}
	}
	if len(steps) == 0 {
		return nil
	}
	slices.Sort(steps)
	limit := max(GapMinDuration, GapFactor*steps[len(steps)/2])

	var gaps [][]bool
	for li, pts := range lines {
		for i := 1; i < len(pts); i++ {
			if dt, ok := timeStep(pts[i-1], pts[i]); !ok || dt <= limit {
				continue
			}
			if gaps == nil {
				gaps = make([][]bool, len(lines))
			}
			if gaps[li] == nil {
				gaps[li] = make([]bool, len(pts)-1)
			}
			gaps[li][i-1] = true
		}
	}
	return gaps
}

// timeStep returns the time from a to b, and whether both have a time.
func timeStep(a, b Point) (time.Duration, bool) {
	if a.Time.IsZero() || b.Time.IsZero() {
		return 0, false
	}
	return b.Time.Sub(a.Time), true
}

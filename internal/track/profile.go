package track

import (
	"math"

	"github.com/mlange-42/tramp/internal/geo"
)

// Densify returns positions along the straight lines between the points, at most spacing meters apart,
// including the points themselves. Positions are interpolated linearly in degrees,
// which is exact enough for the short lines between route points.
func Densify(pts []geo.LonLat, spacing float64) []geo.LonLat {
	if len(pts) == 0 {
		return nil
	}
	res := []geo.LonLat{pts[0]}
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		n := 1
		if spacing > 0 {
			n = max(1, int(math.Ceil(geo.Distance(a, b)/spacing)))
		}
		for k := 1; k <= n; k++ {
			t := float64(k) / float64(n)
			res = append(res, geo.LonLat{Lon: a.Lon + t*(b.Lon-a.Lon), Lat: a.Lat + t*(b.Lat-a.Lat)})
		}
	}
	return res
}

// Climb returns the total ascent and descent along elevations in meters, ignoring changes smaller than
// threshold meters, like the noise of measured elevations. Missing elevations (NaN) are skipped.
//
// It counts a climb only once the elevation moved threshold meters away from the last turning point,
// so that small ups and downs are not summed up.
func Climb(eles []float64, threshold float64) (ascent, descent float64) {
	ref := math.NaN()
	for _, e := range eles {
		switch {
		case math.IsNaN(e):
		case math.IsNaN(ref):
			ref = e
		case e-ref >= threshold:
			ascent += e - ref
			ref = e
		case ref-e >= threshold:
			descent += ref - e
			ref = e
		}
	}
	return ascent, descent
}

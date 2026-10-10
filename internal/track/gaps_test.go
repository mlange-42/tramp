package track

import (
	"slices"
	"testing"
	"time"
)

func TestGaps(t *testing.T) {
	t0 := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	at := func(secs ...int) []Point {
		pts := make([]Point, len(secs))
		for i, s := range secs {
			if s >= 0 {
				pts[i].Time = t0.Add(time.Duration(s) * time.Second)
			}
		}
		return pts
	}

	// 1 s steps: the minimum duration applies.
	gaps := Gaps([][]Point{at(0, 1, 2, 3, 70, 71), at(100, 101, 102)})
	if len(gaps) != 2 || !slices.Equal(gaps[0], []bool{false, false, false, true, false}) || gaps[1] != nil {
		t.Errorf("1 s steps: got %v", gaps)
	}

	// 30 s steps: gaps are longer than 5 times the median.
	if gaps := Gaps([][]Point{at(0, 30, 60, 90, 200, 230)}); gaps != nil {
		t.Errorf("30 s steps, 110 s: got %v, want no gaps", gaps)
	}
	if gaps := Gaps([][]Point{at(0, 30, 60, 90, 300, 330)}); !slices.Equal(gaps[0], []bool{false, false, false, true, false}) {
		t.Errorf("30 s steps, 210 s: got %v", gaps)
	}

	// Points without time.
	if gaps := Gaps([][]Point{at(0, -1, 1000)}); gaps != nil {
		t.Errorf("without times: got %v, want no gaps", gaps)
	}
}

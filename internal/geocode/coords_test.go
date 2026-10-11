package geocode

import (
	"math"
	"testing"

	"github.com/mlange-42/tramp/internal/geo"
)

func TestParseCoords(t *testing.T) {
	tests := []struct {
		in   string
		want geo.LonLat
	}{
		{"51.34, 12.37", geo.LonLat{Lon: 12.37, Lat: 51.34}},
		{"51.34 12.37", geo.LonLat{Lon: 12.37, Lat: 51.34}},
		{"-33.86, 151.21", geo.LonLat{Lon: 151.21, Lat: -33.86}},
		{"51,34 12,37", geo.LonLat{Lon: 12.37, Lat: 51.34}},
		{"51,12", geo.LonLat{Lon: 12, Lat: 51}},
		{"51.34N 12.37E", geo.LonLat{Lon: 12.37, Lat: 51.34}},
		{"33.86S 151.21E", geo.LonLat{Lon: 151.21, Lat: -33.86}},
		{"12.37E 51.34N", geo.LonLat{Lon: 12.37, Lat: 51.34}},
		{"N 51 20.4 E 12 22.2", geo.LonLat{Lon: 12.37, Lat: 51.34}},
		{"N 51° 20.4' E 12° 22.2'", geo.LonLat{Lon: 12.37, Lat: 51.34}},
		{"n51 20.4 w12 22.2", geo.LonLat{Lon: -12.37, Lat: 51.34}},
		{`51°20'24"N 12°22'12"E`, geo.LonLat{Lon: 12.37, Lat: 51.34}},
		{"51°20′24″N, 12°22′12″E", geo.LonLat{Lon: 12.37, Lat: 51.34}},
		{"51 20 24 12 22 12", geo.LonLat{Lon: 12.37, Lat: 51.34}},
		{"  51.34 ; 12.37  ", geo.LonLat{Lon: 12.37, Lat: 51.34}},
	}
	for _, tt := range tests {
		got, ok := ParseCoords(tt.in)
		if !ok {
			t.Errorf("%q: not parsed", tt.in)
			continue
		}
		if math.Abs(got.Lat-tt.want.Lat) > 1e-9 || math.Abs(got.Lon-tt.want.Lon) > 1e-9 {
			t.Errorf("%q: got %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseCoordsInvalid(t *testing.T) {
	for _, in := range []string{
		"", "Leipzig", "10115", "B 96", "51.3", "51 12 30",
		"91, 12", "51, 181",
		"N 51 N 12", "E 12 E 51",
		"51.3N -12.4E",
		"51 60 0 12 0 0",
		"N 51 12",
		"51 N 12 E 13",
	} {
		if ll, ok := ParseCoords(in); ok {
			t.Errorf("%q: parsed as %v", in, ll)
		}
	}
}

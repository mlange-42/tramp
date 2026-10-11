// Package geocode finds positions for search queries: coordinates typed in common formats,
// and place names, using the Nominatim service.
package geocode

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/mlange-42/tramp/internal/geo"
)

var (
	// coordToken matches the parts of a coordinate: hemisphere letters and numbers.
	coordToken = regexp.MustCompile(`[NSEW]|[-+]?\d+(?:\.\d+)?`)
	// decimalComma matches a comma between digits, as in "51,34".
	decimalComma = regexp.MustCompile(`(\d),(\d)`)
	// coordSeparators are replaced by spaces before splitting into tokens.
	coordSeparators = strings.NewReplacer("°", " ", "º", " ", "'", " ", "′", " ", "’", " ",
		`"`, " ", "″", " ", "”", " ", ",", " ", ";", " ", "/", " ")
)

// ParseCoords parses a position in one of the common formats, latitude first unless marked by letters:
//
//	51.34, 12.37
//	-33.86 151.21
//	51.34N 12.37E
//	N 51° 20.4' E 12° 22.2'
//	51°20'24"N 12°22'12"E
//	51,34 12,37 (decimal commas, if the numbers are separated by spaces)
//
// It returns false if s is not a position.
func ParseCoords(s string) (geo.LonLat, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if !strings.Contains(s, ".") && strings.ContainsAny(s, " \t") {
		s = decimalComma.ReplaceAllString(s, "$1.$2")
	}
	s = coordSeparators.Replace(s)
	tokens := coordToken.FindAllString(s, -1)
	// Anything besides the tokens and spaces means it's not a position, like a place name.
	if strings.TrimSpace(coordToken.ReplaceAllString(s, "")) != "" || len(tokens) == 0 {
		return geo.LonLat{}, false
	}

	groups, letters, ok := splitCoords(tokens)
	if !ok {
		return geo.LonLat{}, false
	}
	var ll geo.LonLat
	var hasLat, hasLon bool
	for i, g := range groups {
		v, ok := sexagesimal(g)
		if !ok {
			return geo.LonLat{}, false
		}
		letter := letters[i]
		if letter == "" {
			// Without letters, latitude comes first.
			letter = "N"
			if i == 1 {
				letter = "E"
			}
		} else if v < 0 {
			// A sign and a letter contradict each other.
			return geo.LonLat{}, false
		}
		if letter == "S" || letter == "W" {
			v = -v
		}
		if letter == "N" || letter == "S" {
			if hasLat {
				return geo.LonLat{}, false
			}
			ll.Lat, hasLat = v, true
		} else {
			if hasLon {
				return geo.LonLat{}, false
			}
			ll.Lon, hasLon = v, true
		}
	}
	if math.Abs(ll.Lat) > 90 || math.Abs(ll.Lon) > 180 {
		return geo.LonLat{}, false
	}
	return ll, true
}

// splitCoords splits the tokens into the numbers of two coordinates, with their hemisphere letters, if any.
// Letters come either all before or all after the numbers of their coordinate.
func splitCoords(tokens []string) (groups [2][]float64, letters [2]string, ok bool) {
	isLetter := func(t string) bool { return len(t) == 1 && strings.Contains("NSEW", t) }
	numLetters := 0
	for _, t := range tokens {
		if isLetter(t) {
			numLetters++
		}
	}
	switch numLetters {
	case 0:
		// Split the numbers in half: degrees, degrees and minutes, or degrees, minutes and seconds.
		n := len(tokens)
		if n%2 != 0 || n > 6 {
			return groups, letters, false
		}
		for i, t := range tokens {
			v, _ := strconv.ParseFloat(t, 64)
			groups[i/(n/2)] = append(groups[i/(n/2)], v)
		}
		return groups, letters, true
	case 2:
		prefix := isLetter(tokens[0])
		if !prefix && !isLetter(tokens[len(tokens)-1]) {
			return groups, letters, false
		}
		g := 0
		for _, t := range tokens {
			switch {
			case isLetter(t) && prefix:
				if letters[0] != "" {
					if len(groups[0]) == 0 {
						return groups, letters, false
					}
					g = 1
				}
				letters[g] = t
			case isLetter(t):
				if len(groups[g]) == 0 {
					return groups, letters, false
				}
				letters[g] = t
				g++
			default:
				if g > 1 {
					return groups, letters, false
				}
				v, _ := strconv.ParseFloat(t, 64)
				groups[g] = append(groups[g], v)
			}
		}
		return groups, letters, len(groups[0]) > 0 && len(groups[1]) > 0
	}
	return groups, letters, false
}

// sexagesimal combines degrees, and optionally minutes and seconds, to degrees.
// Only the degrees may have a sign, and minutes and seconds must be below 60.
func sexagesimal(parts []float64) (float64, bool) {
	if len(parts) == 0 || len(parts) > 3 {
		return 0, false
	}
	deg := parts[0]
	v, scale := math.Abs(deg), 1.0
	for _, p := range parts[1:] {
		if p < 0 || p >= 60 {
			return 0, false
		}
		scale /= 60
		v += p * scale
	}
	if math.Signbit(deg) {
		v = -v
	}
	return v, true
}

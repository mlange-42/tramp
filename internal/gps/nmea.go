// Package gps reads live positions from GPS receivers that send NMEA 0183 over a serial port,
// like handheld GPS devices and GPS mice connected by USB or Bluetooth.
package gps

import (
	"strconv"
	"strings"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
)

const (
	// knots is a knot in m/s.
	knots = 1852.0 / 3600
	// uere is the user equivalent range error in meters, for estimating the accuracy from the HDOP.
	uere = 5
	// hdopScale scales HDOP × UERE, about one standard deviation, to a radius containing about 95% of fixes.
	hdopScale = 2
	// minHDOPAccuracy is the smallest accuracy estimated from the HDOP, in meters.
	// The HDOP only reflects the satellite geometry, not errors from reflections near buildings,
	// so it can be very small with many satellites, like 0.6 on phones.
	minHDOPAccuracy = 10
)

// Fix is a position reported by a receiver.
type Fix struct {
	// Pos is the position.
	Pos geo.LonLat
	// Time is when the fix was received, by the local clock.
	Time time.Time
	// Alt is the altitude above mean sea level in meters, if HasAlt.
	Alt    float64
	HasAlt bool
	// Speed is the ground speed in m/s, and Course the direction of movement
	// in degrees clockwise from north, if HasCourse.
	Speed, Course float64
	HasCourse     bool
	// Sats is the number of satellites used, or 0 if not known.
	Sats int
	// HDOP is the horizontal dilution of precision, or 0 if not known.
	HDOP float64
	// Accuracy is the estimated horizontal error in meters, or 0 if not known.
	// It is reported by some receivers, and otherwise estimated from the HDOP,
	// cautiously, as a radius containing about 95% of fixes and at least [minHDOPAccuracy].
	Accuracy float64
}

// parser combines NMEA sentences into fixes.
// Receivers send several sentences per update, each with a part of the information.
type parser struct {
	fix Fix
	// valid is set while the receiver reports a position fix.
	valid bool
	// errEst is the horizontal error reported by the receiver in meters, or 0.
	errEst float64
}

// parse handles a sentence and reports whether it was a valid NMEA sentence,
// and whether it updated the position or the fix state.
func (p *parser) parse(line string, now time.Time) (ok, changed bool) {
	f, ok := splitSentence(line)
	if !ok {
		return false, false
	}
	typ := f[0]
	switch {
	case strings.HasPrefix(typ, "P"):
		// Proprietary sentence, like Garmin's $PGRME.
	case len(typ) == 5:
		// Strip the talker, like GP for GPS or GN for multiple systems.
		typ = typ[2:]
	default:
		return true, false
	}

	switch typ {
	case "GGA":
		// $GPGGA,time,lat,N,lon,E,quality,sats,hdop,alt,M,geoid,M,age,station
		if len(f) < 10 {
			return true, false
		}
		if f[6] == "" || f[6] == "0" {
			return true, p.lost()
		}
		pos, ok := parseLonLat(f[2], f[3], f[4], f[5])
		if !ok {
			return true, false
		}
		p.fix.Pos = pos
		p.fix.Sats, _ = strconv.Atoi(f[7])
		p.fix.HDOP, _ = strconv.ParseFloat(f[8], 64)
		p.fix.Alt, p.fix.HasAlt = parseFloat(f[9])
		return true, p.update(now)
	case "RMC":
		// $GPRMC,time,status,lat,N,lon,E,speed,course,date,...
		if len(f) < 9 {
			return true, false
		}
		if f[2] != "A" {
			return true, p.lost()
		}
		pos, ok := parseLonLat(f[3], f[4], f[5], f[6])
		if !ok {
			return true, false
		}
		p.fix.Pos = pos
		p.setCourse(f[7], f[8], knots)
		return true, p.update(now)
	case "GLL":
		// $GPGLL,lat,N,lon,E,time,status,...
		if len(f) < 7 {
			return true, false
		}
		if f[6] != "A" {
			return true, p.lost()
		}
		pos, ok := parseLonLat(f[1], f[2], f[3], f[4])
		if !ok {
			return true, false
		}
		p.fix.Pos = pos
		return true, p.update(now)
	case "VTG":
		// $GPVTG,course,T,course,M,speed,N,speed,K,...
		if len(f) >= 8 {
			p.setCourse(f[7], f[1], 1/3.6)
		}
	case "GSA":
		// $GPGSA,mode,fix type,12 satellites,PDOP,HDOP,VDOP
		if len(f) >= 17 {
			if h, ok := parseFloat(f[16]); ok {
				p.fix.HDOP = h
			}
		}
	case "GST":
		// $GPGST,time,rms,major,minor,orientation,lat error,lon error,alt error
		if len(f) >= 8 {
			lat, ok1 := parseFloat(f[6])
			lon, ok2 := parseFloat(f[7])
			if ok1 && ok2 {
				p.errEst = max(lat, lon)
			}
		}
	case "PGRME":
		// Garmin: $PGRME,horizontal error,M,vertical error,M,spherical error,M
		if len(f) >= 2 {
			if e, ok := parseFloat(f[1]); ok {
				p.errEst = e
			}
		}
	}
	return true, false
}

// update completes the fix after a position sentence. It reports true.
func (p *parser) update(now time.Time) bool {
	p.valid = true
	p.fix.Time = now
	switch {
	case p.errEst > 0:
		p.fix.Accuracy = p.errEst
	case p.fix.HDOP > 0:
		p.fix.Accuracy = max(p.fix.HDOP*uere*hdopScale, minHDOPAccuracy)
	default:
		p.fix.Accuracy = 0
	}
	return true
}

// lost marks the fix as lost, and reports whether it was valid before.
func (p *parser) lost() bool {
	was := p.valid
	p.valid = false
	return was
}

// setCourse sets speed and course from NMEA fields, with the speed converted to m/s by the factor.
func (p *parser) setCourse(speed, course string, factor float64) {
	s, ok := parseFloat(speed)
	if !ok {
		p.fix.HasCourse = false
		return
	}
	p.fix.Speed = s * factor
	// The course is empty while standing still on some receivers.
	p.fix.Course, p.fix.HasCourse = parseFloat(course)
}

// splitSentence checks the checksum of an NMEA sentence, and returns its comma-separated fields
// without the leading $ and the checksum. The first field is the sentence type, like GPGGA.
func splitSentence(s string) ([]string, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 7 || s[0] != '$' {
		return nil, false
	}
	star := strings.LastIndexByte(s, '*')
	if star < 0 || len(s) != star+3 {
		return nil, false
	}
	want, err := strconv.ParseUint(s[star+1:], 16, 8)
	if err != nil {
		return nil, false
	}
	var sum byte
	for i := 1; i < star; i++ {
		sum ^= s[i]
	}
	if sum != byte(want) {
		return nil, false
	}
	return strings.Split(s[1:star], ","), true
}

// parseLonLat converts NMEA coordinates in ddmm.mmmm and dddmm.mmmm format with hemispheres to a position.
func parseLonLat(lat, ns, lon, ew string) (geo.LonLat, bool) {
	la, ok1 := parseCoord(lat, ns, "S")
	lo, ok2 := parseCoord(lon, ew, "W")
	if !ok1 || !ok2 || la < -90 || la > 90 || lo < -180 || lo > 180 {
		return geo.LonLat{}, false
	}
	return geo.LonLat{Lon: lo, Lat: la}, true
}

// parseCoord converts an NMEA coordinate in (d)ddmm.mmmm format to decimal degrees,
// negated for the given negative hemisphere.
func parseCoord(v, hemi, neg string) (float64, bool) {
	dot := strings.IndexByte(v, '.')
	if dot < 0 {
		dot = len(v)
	}
	if dot < 3 {
		return 0, false
	}
	deg, err1 := strconv.ParseFloat(v[:dot-2], 64)
	minutes, err2 := strconv.ParseFloat(v[dot-2:], 64)
	if err1 != nil || err2 != nil || minutes >= 60 {
		return 0, false
	}
	d := deg + minutes/60
	if hemi == neg {
		d = -d
	}
	return d, true
}

func parseFloat(s string) (float64, bool) {
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

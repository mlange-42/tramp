package gps

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// sentence adds $ and the checksum to an NMEA sentence body.
func sentence(body string) string {
	var sum byte
	for i := range len(body) {
		sum ^= body[i]
	}
	return fmt.Sprintf("$%s*%02X", body, sum)
}

func near(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

func TestSplitSentence(t *testing.T) {
	// Recorded from a Locosys GT-31.
	f, ok := splitSentence("$GPGGA,085309.829,5119.7613,N,01224.5324,E,1,03,9.9,107.17,M,46.7,M,,0000*64\r\n")
	if !ok || f[0] != "GPGGA" || len(f) != 15 {
		t.Fatalf("got %v, %v", f, ok)
	}
	for _, s := range []string{
		"$GPGGA,085309.829,5119.7613,N,01224.5324,E,1,03,9.9,107.17,M,46.7,M,,0000*65",
		"GPGGA,085309.829*00",
		"$GPGGA,085309.829",
		"x\x86\xe0",
		"",
	} {
		if _, ok := splitSentence(s); ok {
			t.Errorf("%q accepted", s)
		}
	}
}

func TestParserGT31(t *testing.T) {
	var p parser
	now := time.Now()
	ok, changed := p.parse("$GPGGA,085309.829,5119.7613,N,01224.5324,E,1,03,9.9,107.17,M,46.7,M,,0000*64", now)
	if !ok || !changed || !p.valid {
		t.Fatalf("ok %v, changed %v, valid %v", ok, changed, p.valid)
	}
	f := p.fix
	if !near(f.Pos.Lat, 51+19.7613/60) || !near(f.Pos.Lon, 12+24.5324/60) {
		t.Errorf("position %v", f.Pos)
	}
	if f.Sats != 3 || f.HDOP != 9.9 || !f.HasAlt || f.Alt != 107.17 || !near(f.Accuracy, 9.9*uere*hdopScale) {
		t.Errorf("fix %+v", f)
	}
	if !f.Time.Equal(now) {
		t.Errorf("time %v", f.Time)
	}

	p.parse(sentence("GPRMC,085310.829,A,5119.7613,S,01224.5324,W,10.0,45.5,111026,,,A"), now)
	f = p.fix
	if f.Pos.Lat >= 0 || f.Pos.Lon >= 0 || !f.HasCourse || !near(f.Speed, 10*knots) || f.Course != 45.5 {
		t.Errorf("fix %+v", f)
	}
}

func TestParserLost(t *testing.T) {
	var p parser
	now := time.Now()
	p.parse(sentence("GNGGA,085309.00,5119.7613,N,01224.5324,E,1,08,1.0,100.0,M,46.7,M,,"), now)
	if !p.valid {
		t.Fatal("no fix")
	}
	if _, changed := p.parse(sentence("GNRMC,085310.00,V,,,,,,,111026,,,N"), now); !changed || p.valid {
		t.Errorf("changed %v, valid %v", changed, p.valid)
	}
	if _, changed := p.parse(sentence("GNGGA,085311.00,,,,,0,00,99.9,,,,,,"), now); changed || p.valid {
		t.Errorf("changed %v, valid %v", changed, p.valid)
	}
}

func TestParserAccuracy(t *testing.T) {
	var p parser
	now := time.Now()
	// A small HDOP, as from a phone (NetGPS), gives the minimum.
	p.parse(sentence("GPGGA,102332,5119.75635,N,01224.52799,E,1,14,0.6,171.000,M,,M,,"), now)
	if p.fix.Accuracy != minHDOPAccuracy {
		t.Errorf("accuracy %v", p.fix.Accuracy)
	}
	// Garmin's estimated position error.
	p.parse(sentence("PGRME,15.0,M,45.0,M,25.0,M"), now)
	p.parse(sentence("GPGLL,5119.7613,N,01224.5324,E,085309.00,A,A"), now)
	if p.fix.Accuracy != 15 {
		t.Errorf("accuracy %v", p.fix.Accuracy)
	}
	// u-blox: standard deviation of latitude and longitude.
	p.parse(sentence("GNGST,085310.00,20,,,,3.5,2.5,6.0"), now)
	p.parse(sentence("GNGGA,085310.00,5119.7613,N,01224.5324,E,1,08,1.0,100.0,M,46.7,M,,"), now)
	if p.fix.Accuracy != 3.5 {
		t.Errorf("accuracy %v", p.fix.Accuracy)
	}
	// Speed and course from VTG, in km/h.
	p.parse(sentence("GNVTG,90.0,T,,M,5.4,N,10.0,K,A"), now)
	if !p.fix.HasCourse || p.fix.Course != 90 || !near(p.fix.Speed, 10/3.6) {
		t.Errorf("fix %+v", p.fix)
	}
}

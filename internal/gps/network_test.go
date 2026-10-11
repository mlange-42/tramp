package gps

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"
)

func TestParseNetwork(t *testing.T) {
	tests := []struct {
		in, network, addr string
		ok, err           bool
	}{
		{"COM5", "", "", false, false},
		{"/dev/ttyUSB0", "", "", false, false},
		{"tcp://192.168.1.5:10110", "tcp", "192.168.1.5:10110", true, false},
		{"UDP://:10110", "udp", ":10110", true, false},
		{"gpsd://localhost", "gpsd", "localhost:2947", true, false},
		{"gpsd://pi:3000", "gpsd", "pi:3000", true, false},
		{"tcp://host", "", "", true, true},
		{"http://host:80", "", "", true, true},
	}
	for _, tt := range tests {
		network, addr, ok, err := parseNetwork(tt.in)
		if network != tt.network || addr != tt.addr || ok != tt.ok || (err != nil) != tt.err {
			t.Errorf("%s: got %q %q %v %v", tt.in, network, addr, ok, err)
		}
	}
}

// waitFix waits until the receiver has a valid fix.
func waitFix(t *testing.T, r *Receiver) Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s := r.Status(); s.Valid {
			return s
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no fix, status %+v", r.Status())
	return Status{}
}

var testGGA = sentence("GPGGA,085309.00,5119.7613,N,01224.5324,E,1,08,1.0,107.2,M,46.7,M,,")

// serveTCP accepts one connection, calls greet with it, and then sends sentences until the test ends.
func serveTCP(t *testing.T, greet func(c net.Conn)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		if greet != nil {
			greet(c)
		}
		for {
			if _, err := c.Write([]byte(testGGA + "\r\n")); err != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
	return ln.Addr().String()
}

func TestReceiverTCP(t *testing.T) {
	addr := serveTCP(t, nil)
	r := Start(Config{Port: "tcp://" + addr}, nil, nil)
	defer r.Wait()
	defer r.Stop()
	s := waitFix(t, r)
	if s.State != Connected || s.Port != "tcp://"+addr || s.Fix.Sats != 8 {
		t.Errorf("status %+v", s)
	}
}

func TestReceiverGpsd(t *testing.T) {
	watch := make(chan string, 1)
	addr := serveTCP(t, func(c net.Conn) {
		// gpsd greets with its version, and sends NMEA only after the watch command.
		_, _ = c.Write([]byte(`{"class":"VERSION","release":"3.25"}` + "\n"))
		line, _ := bufio.NewReader(c).ReadString('\n')
		watch <- line
	})
	r := Start(Config{Port: "gpsd://" + addr}, nil, nil)
	defer r.Wait()
	defer r.Stop()
	waitFix(t, r)
	if w := <-watch; !strings.Contains(w, `"nmea":true`) {
		t.Errorf("watch command %q", w)
	}
}

func TestReceiverUDP(t *testing.T) {
	// Find a free port.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := pc.LocalAddr().String()
	_ = pc.Close()

	r := Start(Config{Port: "udp://" + addr}, nil, nil)
	defer r.Wait()
	defer r.Stop()
	done := make(chan struct{})
	defer close(done)
	go func() {
		c, err := net.Dial("udp", addr)
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		for {
			select {
			case <-done:
				return
			case <-time.After(50 * time.Millisecond):
				// Several sentences in one datagram.
				_, _ = c.Write([]byte(sentence("GPGSA,A,3,,,,,,,,,,,,,2.0,1.0,1.7") + "\r\n" + testGGA + "\r\n"))
			}
		}
	}()
	waitFix(t, r)
}

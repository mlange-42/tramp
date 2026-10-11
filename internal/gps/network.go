package gps

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

const (
	// gpsdPort is the default port of gpsd.
	gpsdPort = "2947"
	// gpsdWatch asks gpsd to send NMEA sentences instead of its own JSON reports.
	gpsdWatch = `?WATCH={"enable":true,"nmea":true};` + "\n"
	// networkProbeTime is how long to wait for NMEA from a network source.
	// Apps on phones may send only when they have a fix, or at low rates.
	networkProbeTime = 10 * time.Second
)

// parseNetwork parses a network source like tcp://host:port, udp://:port or gpsd://host[:port].
// It returns the network ("tcp", "udp" or "gpsd") and the address, and false if s is not a network source.
func parseNetwork(s string) (network, addr string, ok bool, err error) {
	scheme, addr, ok := strings.Cut(s, "://")
	if !ok {
		return "", "", false, nil
	}
	network = strings.ToLower(scheme)
	switch network {
	case "tcp", "udp":
		if _, _, err := net.SplitHostPort(addr); err != nil {
			return "", "", true, fmt.Errorf("invalid address %q: %w", s, err)
		}
	case "gpsd":
		if _, _, err := net.SplitHostPort(addr); err != nil {
			addr = net.JoinHostPort(addr, gpsdPort)
		}
	default:
		return "", "", true, fmt.Errorf("unknown network %q in %q, expected tcp, udp or gpsd", scheme, s)
	}
	return network, addr, true, nil
}

// openNetwork connects to a TCP server or gpsd, or listens for UDP datagrams.
func openNetwork(ctx context.Context, p port) (stream, error) {
	if p.network == "udp" {
		var lc net.ListenConfig
		pc, err := lc.ListenPacket(ctx, "udp", p.addr)
		if err != nil {
			return nil, err
		}
		return &packetStream{pc: pc}, nil
	}
	d := net.Dialer{Timeout: openTimeout}
	c, err := d.DialContext(ctx, "tcp", p.addr)
	if err != nil {
		return nil, err
	}
	if p.network == "gpsd" {
		_ = c.SetWriteDeadline(time.Now().Add(openTimeout))
		if _, err := io.WriteString(c, gpsdWatch); err != nil {
			_ = c.Close()
			return nil, err
		}
	}
	return &netStream{c: c}, nil
}

// netStream reads from a TCP connection with [readTimeout], like a serial port:
// a read that times out returns no data and no error.
type netStream struct {
	c net.Conn
}

func (s *netStream) Read(b []byte) (int, error) {
	_ = s.c.SetReadDeadline(time.Now().Add(readTimeout))
	n, err := s.c.Read(b)
	if isTimeout(err) {
		return n, nil
	}
	if errors.Is(err, io.EOF) {
		return n, errors.New("connection closed")
	}
	return n, err
}

func (s *netStream) Close() error {
	return s.c.Close()
}

// packetStream reads UDP datagrams with [readTimeout], as a stream of their contents.
type packetStream struct {
	pc net.PacketConn
}

func (s *packetStream) Read(b []byte) (int, error) {
	_ = s.pc.SetReadDeadline(time.Now().Add(readTimeout))
	n, _, err := s.pc.ReadFrom(b)
	if isTimeout(err) {
		return n, nil
	}
	return n, err
}

func (s *packetStream) Close() error {
	return s.pc.Close()
}

func isTimeout(err error) bool {
	return errors.Is(err, os.ErrDeadlineExceeded)
}

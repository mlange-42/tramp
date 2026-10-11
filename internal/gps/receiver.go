package gps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.bug.st/serial"
)

const (
	// readTimeout is the timeout of a single read, so that stopping is noticed quickly.
	readTimeout = 200 * time.Millisecond
	// openTimeout is the longest wait for opening a port. Opening a Bluetooth port connects to the device.
	openTimeout = 15 * time.Second
	// probeTime is how long to listen for NMEA at a baud rate. Most receivers send once per second.
	probeTime = 2500 * time.Millisecond
	// silentTime is after how long without any data a port is given up when probing.
	// At a wrong baud rate, a receiver still sends junk bytes.
	silentTime = 1500 * time.Millisecond
	// bluetoothProbeTime is how long to listen on a Bluetooth port, which needs time to connect.
	bluetoothProbeTime = 10 * time.Second
	// dataTimeout is after how long without valid NMEA the connection is considered lost.
	dataTimeout = 10 * time.Second
	// retryDelay is the wait before searching again after no receiver was found.
	retryDelay = 3 * time.Second
	// maxLine is the longest line kept. NMEA sentences are at most 82 characters.
	maxLine = 256
)

// bauds are the baud rates to probe, most common for GPS receivers first.
var bauds = []int{4800, 9600, 38400, 115200, 57600, 19200}

// Config selects the serial port of the receiver.
type Config struct {
	// Port is the serial port, like COM5 or /dev/ttyUSB0. If empty, the receiver is searched on all ports.
	Port string
	// Baud is the baud rate. If 0, common baud rates are tried.
	Baud int
}

// State is the connection state of a [Receiver].
type State int

const (
	// Stopped is the state after [Receiver.Stop].
	Stopped State = iota
	// Searching is the state while looking for a receiver on the serial ports.
	Searching
	// Connected is the state while NMEA data is received.
	Connected
)

// Status is the state of a [Receiver] with its last fix.
type Status struct {
	State State
	// Port and Baud are the port while searching or connected, and the baud rate while connected.
	Port string
	Baud int
	// Fix is the last position, if HasFix. It is kept when the fix is lost or the receiver disconnects.
	Fix    Fix
	HasFix bool
	// Valid is set while connected and the receiver reports a position fix.
	Valid bool
	// Err is the last error, like no receiver found. It is reset on connecting.
	Err error
}

// Receiver reads positions from a GPS receiver in the background.
// It searches the receiver, and reconnects when the connection is lost.
type Receiver struct {
	cfg      Config
	onUpdate func()
	cancel   context.CancelFunc
	done     chan struct{}

	mu     sync.Mutex
	status Status

	// last is the port and baud rate of the last connection, tried first when reconnecting.
	// Only used by the background goroutine.
	last *candidate
}

// candidate is a port with a baud rate to try.
type candidate struct {
	port port
	baud int
}

// ErrNotFound is the error while no receiver is found on any port.
var ErrNotFound = errors.New("no GPS receiver found")

// Start starts reading from a receiver. onUpdate is called from a background goroutine
// whenever the status changes. If prev is not nil, it is a stopped receiver to wait for,
// so that the port is closed before it is opened again.
func Start(cfg Config, onUpdate func(), prev *Receiver) *Receiver {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Receiver{
		cfg:      cfg,
		onUpdate: onUpdate,
		cancel:   cancel,
		done:     make(chan struct{}),
		status:   Status{State: Searching},
	}
	go func() {
		if prev != nil {
			prev.Wait()
		}
		r.run(ctx)
	}()
	return r
}

// Stop stops reading and closes the port. It does not wait for the port to close.
func (r *Receiver) Stop() {
	r.cancel()
}

// Wait waits until the port is closed after [Receiver.Stop].
func (r *Receiver) Wait() {
	<-r.done
}

// Status returns the current status.
func (r *Receiver) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

func (r *Receiver) setStatus(f func(s *Status)) {
	r.mu.Lock()
	f(&r.status)
	r.mu.Unlock()
	if r.onUpdate != nil {
		r.onUpdate()
	}
}

func (r *Receiver) run(ctx context.Context) {
	defer close(r.done)
	defer r.setStatus(func(s *Status) { s.State, s.Valid = Stopped, false })
	for ctx.Err() == nil {
		conn, err := r.connect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			r.setStatus(func(s *Status) { s.State, s.Port, s.Valid, s.Err = Searching, "", false, err })
			sleep(ctx, retryDelay)
			continue
		}
		r.last = &conn.cand
		r.setStatus(func(s *Status) {
			s.State, s.Port, s.Baud, s.Err = Connected, conn.cand.port.Name, conn.cand.baud, nil
		})
		err = r.read(ctx, conn)
		conn.close()
		if err != nil && ctx.Err() == nil {
			r.setStatus(func(s *Status) { s.State, s.Valid, s.Err = Searching, false, err })
		}
	}
}

// candidates returns the ports and baud rates to try, in order.
func (r *Receiver) candidates() ([]candidate, error) {
	var ports []port
	if r.cfg.Port != "" {
		ports = []port{findPort(r.cfg.Port)}
	} else {
		var err error
		if ports, err = listPorts(); err != nil {
			return nil, fmt.Errorf("listing serial ports: %w", err)
		}
	}
	var cands []candidate
	if r.last != nil {
		cands = append(cands, *r.last)
	}
	for _, p := range ports {
		switch {
		case r.last != nil && p.Name == r.last.port.Name && (r.cfg.Baud > 0 || p.Kind == kindBluetooth):
			// Already tried first.
		case r.cfg.Baud > 0:
			cands = append(cands, candidate{p, r.cfg.Baud})
		case p.Kind == kindBluetooth:
			// The baud rate of Bluetooth ports is ignored.
			cands = append(cands, candidate{p, bauds[0]})
		default:
			for _, b := range bauds {
				cands = append(cands, candidate{p, b})
			}
		}
	}
	return cands, nil
}

// connect searches the receiver and returns the open connection.
func (r *Receiver) connect(ctx context.Context) (*conn, error) {
	cands, err := r.candidates()
	if err != nil {
		return nil, err
	}
	// Ports that sent nothing at all are not tried at other baud rates.
	silent := map[string]bool{}
	var lastErr error
	for _, c := range cands {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if silent[c.port.Name] {
			continue
		}
		r.setStatus(func(s *Status) { s.State, s.Port = Searching, c.port.Name })
		cn, got, err := probe(ctx, c)
		if err != nil {
			lastErr = err
			silent[c.port.Name] = true
			continue
		}
		if cn != nil {
			return cn, nil
		}
		if !got {
			silent[c.port.Name] = true
		}
	}
	if r.cfg.Port != "" && lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrNotFound
}

// probe opens a port and listens for valid NMEA. It returns the open connection if found,
// and whether any data was received.
func probe(ctx context.Context, c candidate) (cn *conn, got bool, err error) {
	cn, err = open(ctx, c)
	if err != nil {
		return nil, false, err
	}
	listen, silence := probeTime, silentTime
	if c.port.Kind == kindBluetooth {
		listen, silence = bluetoothProbeTime, bluetoothProbeTime
	}
	start := time.Now()
	for ctx.Err() == nil && time.Since(start) < listen {
		line, err := cn.readLine()
		if err != nil {
			cn.close()
			return nil, got, err
		}
		got = got || cn.received > 0
		if !got && time.Since(start) > silence {
			break
		}
		if line == "" {
			continue
		}
		if _, ok := splitSentence(line); ok {
			cn.pending = line
			return cn, true, nil
		}
	}
	cn.close()
	return nil, got, nil
}

// read reads sentences until the connection is lost or ctx is done.
func (r *Receiver) read(ctx context.Context, cn *conn) error {
	var p parser
	lastValid := time.Now()
	for ctx.Err() == nil {
		line := cn.pending
		cn.pending = ""
		if line == "" {
			var err error
			if line, err = cn.readLine(); err != nil {
				return fmt.Errorf("reading %s: %w", cn.cand.port.Name, err)
			}
		}
		now := time.Now()
		ok, changed := false, false
		if line != "" {
			ok, changed = p.parse(line, now)
		}
		if !ok {
			if now.Sub(lastValid) > dataTimeout {
				return fmt.Errorf("no data from %s", cn.cand.port.Name)
			}
			continue
		}
		lastValid = now
		if changed {
			fix, valid := p.fix, p.valid
			r.setStatus(func(s *Status) {
				s.Valid = valid
				if valid {
					s.Fix, s.HasFix = fix, true
				}
			})
		}
	}
	return nil
}

// conn is an open serial port with a line buffer.
type conn struct {
	cand candidate
	port serial.Port
	buf  []byte
	tmp  []byte
	// received counts the bytes received.
	received int
	// pending is a line read while probing, not parsed yet.
	pending string
}

// open opens a port, giving up after [openTimeout].
func open(ctx context.Context, c candidate) (*conn, error) {
	type result struct {
		port serial.Port
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		p, err := serial.Open(c.port.Name, &serial.Mode{
			BaudRate: c.baud, DataBits: 8, Parity: serial.NoParity, StopBits: serial.OneStopBit,
		})
		if err == nil {
			if err = p.SetReadTimeout(readTimeout); err != nil {
				p.Close()
				p = nil
			}
		}
		ch <- result{p, err}
	}()
	// Close the port when it opens too late.
	abandon := func() {
		go func() {
			if res := <-ch; res.port != nil {
				res.port.Close()
			}
		}()
	}
	timer := time.NewTimer(openTimeout)
	defer timer.Stop()
	select {
	case res := <-ch:
		if res.err != nil {
			return nil, fmt.Errorf("opening %s: %w", c.port.Name, res.err)
		}
		return &conn{cand: c, port: res.port, tmp: make([]byte, 256)}, nil
	case <-timer.C:
		abandon()
		return nil, fmt.Errorf("opening %s: timeout", c.port.Name)
	case <-ctx.Done():
		abandon()
		return nil, ctx.Err()
	}
}

// readLine returns the next line, or an empty string if none was completed before the read timeout.
func (c *conn) readLine() (string, error) {
	if line, ok := c.nextLine(); ok {
		return line, nil
	}
	n, err := c.port.Read(c.tmp)
	if err != nil {
		return "", err
	}
	c.received += n
	c.buf = append(c.buf, c.tmp[:n]...)
	if line, ok := c.nextLine(); ok {
		return line, nil
	}
	if len(c.buf) > maxLine {
		// Junk without line breaks, like at a wrong baud rate.
		c.buf = c.buf[:0]
	}
	return "", nil
}

// nextLine takes a complete line from the buffer.
func (c *conn) nextLine() (string, bool) {
	i := bytes.IndexByte(c.buf, '\n')
	if i < 0 {
		return "", false
	}
	line := string(bytes.TrimSpace(c.buf[:i]))
	c.buf = c.buf[:copy(c.buf, c.buf[i+1:])]
	return line, true
}

func (c *conn) close() {
	c.port.Close()
}

// sleep waits for d or until ctx is done.
func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

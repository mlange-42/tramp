package wms

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// LayerInfo is what the capabilities of a WMS tell about a layer.
type LayerInfo struct {
	// Name is the name of the layer, as in the capabilities.
	Name string
	// Time are the valid times of the layer. Nil if the layer has no time dimension.
	Time *TimeDimension
	// Reference is the default reference time, like the start of the latest model run. Empty if not given.
	Reference string
	// LegendURL is the address of the legend image of the default style. Empty if not given.
	LegendURL string
}

// Capabilities are the layers of a WMS, by name.
type Capabilities struct {
	Layers map[string]*LayerInfo
}

// Layer returns the information on a layer by its name. Names given with and without a
// workspace prefix like "dwd:" match, as servers list them with or without it.
func (c *Capabilities) Layer(name string) (*LayerInfo, bool) {
	if l, ok := c.Layers[name]; ok {
		return l, true
	}
	l, ok := c.Layers[localName(name)]
	return l, ok
}

// CapabilitiesURL returns the GetCapabilities request URL of a layer:
// for its own capabilities address if it has one, otherwise for the service URL.
func (l *Layer) CapabilitiesURL() string {
	base := l.Capabilities
	if base == "" {
		base = l.URL
	}
	version := l.Version
	if version == "" {
		version = "1.3.0"
	}
	q := url.Values{}
	q.Set("SERVICE", "WMS")
	q.Set("VERSION", version)
	q.Set("REQUEST", "GetCapabilities")
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + q.Encode()
}

// GetCapabilities fetches and parses the capabilities of the WMS of a layer.
func (c *Client) GetCapabilities(ctx context.Context, layer *Layer) (*Capabilities, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, layer.CapabilitiesURL(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
		return nil, fmt.Errorf("wms: capabilities: HTTP %d: %s", resp.StatusCode, excerpt(body))
	}
	return ParseCapabilities(io.LimitReader(resp.Body, 64<<20))
}

type xmlCapabilities struct {
	Layers []xmlLayer `xml:"Capability>Layer"`
}

type xmlLayer struct {
	Name string `xml:"Name"`
	// Dimensions hold the values in WMS 1.3.0. In WMS 1.1.1, they are in the extents.
	Dimensions []xmlDimension `xml:"Dimension"`
	Extents    []xmlDimension `xml:"Extent"`
	Styles     []struct {
		Legends []struct {
			Resource struct {
				Href string `xml:"href,attr"`
			} `xml:"OnlineResource"`
		} `xml:"LegendURL"`
	} `xml:"Style"`
	Layers []xmlLayer `xml:"Layer"`
}

type xmlDimension struct {
	Name    string `xml:"name,attr"`
	Default string `xml:"default,attr"`
	Values  string `xml:",chardata"`
}

// ParseCapabilities reads the layers from a WMS capabilities document.
// Layers inherit the dimensions of their parents.
func ParseCapabilities(r io.Reader) (*Capabilities, error) {
	var doc xmlCapabilities
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("wms: capabilities: %w", err)
	}
	c := &Capabilities{Layers: map[string]*LayerInfo{}}
	for i := range doc.Layers {
		c.add(&doc.Layers[i], nil)
	}
	return c, nil
}

func (c *Capabilities) add(l *xmlLayer, parent *LayerInfo) {
	info := &LayerInfo{Name: l.Name}
	if parent != nil {
		info.Time, info.Reference = parent.Time, parent.Reference
	}
	for _, dims := range [][]xmlDimension{l.Dimensions, l.Extents} {
		for _, d := range dims {
			values := strings.TrimSpace(d.Values)
			switch {
			case strings.EqualFold(d.Name, "time") && values != "":
				info.Time = &TimeDimension{Default: d.Default, Values: values}
			case strings.EqualFold(d.Name, "reference_time") && d.Default != "":
				info.Reference = d.Default
			}
		}
	}
	if len(l.Styles) > 0 && len(l.Styles[0].Legends) > 0 {
		info.LegendURL = l.Styles[0].Legends[0].Resource.Href
	}
	if l.Name != "" {
		c.Layers[l.Name] = info
		if local := localName(l.Name); local != l.Name {
			if _, ok := c.Layers[local]; !ok {
				c.Layers[local] = info
			}
		}
	}
	for i := range l.Layers {
		c.add(&l.Layers[i], info)
	}
}

// localName returns a layer name without its workspace prefix, like "radar" for "dwd:radar".
func localName(name string) string {
	if i := strings.LastIndexByte(name, ':'); i >= 0 {
		return name[i+1:]
	}
	return name
}

// TimeDimension are the valid times of a layer, as given in the capabilities.
type TimeDimension struct {
	// Default is the time used if a request gives none, like "current" or a time.
	Default string
	// Values is a comma-separated list of times and intervals like "2026-10-12T00:00Z/2026-10-20T12:00Z/PT3H".
	Values string
}

// Nearest returns the valid time closest to t, or false if there is none or the values are not understood.
func (d *TimeDimension) Nearest(t time.Time) (time.Time, bool) {
	var best time.Time
	var bestDist time.Duration = -1
	for item := range strings.SplitSeq(d.Values, ",") {
		v, ok := nearestIn(strings.TrimSpace(item), t)
		if !ok {
			continue
		}
		if dist := absDuration(v.Sub(t)); bestDist < 0 || dist < bestDist {
			best, bestDist = v, dist
		}
	}
	return best, bestDist >= 0
}

// nearestIn returns the time closest to t in a single time or an interval with a period.
func nearestIn(item string, t time.Time) (time.Time, bool) {
	parts := strings.Split(item, "/")
	start, err := parseTime(parts[0])
	if err != nil {
		return time.Time{}, false
	}
	if len(parts) == 1 {
		return start, true
	}
	if len(parts) != 3 {
		return time.Time{}, false
	}
	end, err1 := parseTime(parts[1])
	period, err2 := ParsePeriod(parts[2])
	if err1 != nil || err2 != nil || period <= 0 || end.Before(start) {
		return time.Time{}, false
	}
	switch {
	case !t.After(start):
		return start, true
	case !t.Before(end):
		// The end may not be on a step.
		return start.Add(end.Sub(start) / period * period), true
	}
	n := (t.Sub(start) + period/2) / period
	v := start.Add(n * period)
	if v.After(end) {
		v = v.Add(-period)
	}
	return v, true
}

// parseTime parses a time as given in the capabilities, with or without time of day.
func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse(time.DateOnly, s)
}

// FormatTime formats a time for the TIME parameter of a request.
func FormatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// ParsePeriod parses an ISO 8601 duration like "PT5M", "PT1H" or "P1D".
// Years and months are not supported, as they have no fixed length.
func ParsePeriod(s string) (time.Duration, error) {
	rest, ok := strings.CutPrefix(s, "P")
	if !ok || rest == "" {
		return 0, fmt.Errorf("wms: invalid period %q", s)
	}
	var d time.Duration
	inTime := false
	for rest != "" {
		if rest[0] == 'T' {
			inTime, rest = true, rest[1:]
			continue
		}
		i := strings.IndexFunc(rest, func(r rune) bool { return (r < '0' || r > '9') && r != '.' })
		if i <= 0 {
			return 0, fmt.Errorf("wms: invalid period %q", s)
		}
		n, err := strconv.ParseFloat(rest[:i], 64)
		if err != nil {
			return 0, fmt.Errorf("wms: invalid period %q", s)
		}
		var unit time.Duration
		switch {
		case !inTime && rest[i] == 'W':
			unit = 7 * 24 * time.Hour
		case !inTime && rest[i] == 'D':
			unit = 24 * time.Hour
		case inTime && rest[i] == 'H':
			unit = time.Hour
		case inTime && rest[i] == 'M':
			unit = time.Minute
		case inTime && rest[i] == 'S':
			unit = time.Second
		default:
			return 0, errors.New("wms: unsupported period " + strconv.Quote(s))
		}
		d += time.Duration(n * float64(unit))
		rest = rest[i+1:]
	}
	return d, nil
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

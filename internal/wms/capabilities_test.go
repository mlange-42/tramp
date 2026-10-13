package wms

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/httpx"
)

const testCapabilities = `<?xml version="1.0" encoding="UTF-8"?>
<WMS_Capabilities version="1.3.0" xmlns="http://www.opengis.net/wms" xmlns:xlink="http://www.w3.org/1999/xlink">
  <Service><Name>WMS</Name></Service>
  <Capability>
    <Layer>
      <Name>Root</Name>
      <Layer>
        <Name>Weather</Name>
        <Dimension name="time" default="current" units="ISO8601">2026-10-13T00:00:00.000Z/2026-10-13T12:00:00.000Z/PT1H</Dimension>
        <Layer queryable="1">
          <Name>dwd:Radar</Name>
          <Dimension name="time" default="current" units="ISO8601">2026-10-13T10:00:00.000Z/2026-10-13T12:55:00.000Z/PT5M</Dimension>
          <Dimension name="REFERENCE_TIME" default="2026-10-13T10:55:00.000Z" units="ISO8601">2026-10-13T10:50:00.000Z,2026-10-13T10:55:00.000Z</Dimension>
          <Style>
            <Name>default</Name>
            <LegendURL width="100" height="390">
              <Format>image/png</Format>
              <OnlineResource xlink:type="simple" xlink:href="https://example.com/legend?layer=dwd%3ARadar"/>
            </LegendURL>
          </Style>
        </Layer>
        <Layer><Name>Forecast</Name></Layer>
      </Layer>
      <Layer><Name>Static</Name></Layer>
    </Layer>
  </Capability>
</WMS_Capabilities>`

func TestParseCapabilities(t *testing.T) {
	c, err := ParseCapabilities(strings.NewReader(testCapabilities))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dwd:Radar", "Radar", "other:Radar"} {
		l, ok := c.Layer(name)
		if !ok || l.Name != "dwd:Radar" {
			t.Fatalf("layer %s not found", name)
		}
		if l.Time == nil || !strings.HasSuffix(l.Time.Values, "PT5M") || l.Time.Default != "current" ||
			l.Reference != "2026-10-13T10:55:00.000Z" || l.LegendURL != "https://example.com/legend?layer=dwd%3ARadar" {
			t.Errorf("unexpected layer info %+v %+v", l, l.Time)
		}
	}
	// Dimensions are inherited from the parents.
	if l, ok := c.Layer("Forecast"); !ok || l.Time == nil || !strings.HasSuffix(l.Time.Values, "PT1H") || l.LegendURL != "" {
		t.Errorf("unexpected inherited info %+v", l)
	}
	if l, ok := c.Layer("Static"); !ok || l.Time != nil {
		t.Errorf("unexpected static layer %+v", l)
	}
	if _, ok := c.Layer("Missing"); ok {
		t.Error("unexpected missing layer")
	}
	if _, err := ParseCapabilities(strings.NewReader("<html>")); err == nil {
		t.Error("expected error for invalid XML")
	}
}

func TestParseCapabilitiesExtent(t *testing.T) {
	// WMS 1.1.1 gives the values in Extent elements.
	doc := `<WMT_MS_Capabilities version="1.1.1"><Capability><Layer><Name>A</Name>
<Dimension name="time" units="ISO8601"/><Extent name="time" default="2026-10-11">2026-10-01/2026-10-11/P1D</Extent>
</Layer></Capability></WMT_MS_Capabilities>`
	c, err := ParseCapabilities(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	l, ok := c.Layer("A")
	if !ok || l.Time == nil || l.Time.Values != "2026-10-01/2026-10-11/P1D" || l.Time.Default != "2026-10-11" {
		t.Errorf("unexpected layer %+v", l)
	}
}

func TestTimeDimensionNearest(t *testing.T) {
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	steps := "2026-10-13T00:00:00.000Z/2026-10-13T12:00:00.000Z/PT3H"
	for _, tc := range []struct {
		values, t, want string
	}{
		{steps, "2026-10-13T04:20:00Z", "2026-10-13T03:00:00Z"},
		{steps, "2026-10-13T04:40:00Z", "2026-10-13T06:00:00Z"},
		{steps, "2026-10-12T20:00:00Z", "2026-10-13T00:00:00Z"},
		{steps, "2026-10-14T00:00:00Z", "2026-10-13T12:00:00Z"},
		// The end is not on a step.
		{"2026-10-13T00:00:00Z/2026-10-13T11:00:00Z/PT3H", "2026-10-13T10:50:00Z", "2026-10-13T09:00:00Z"},
		{"2026-10-13T00:00:00Z/2026-10-13T11:00:00Z/PT3H", "2026-10-13T12:00:00Z", "2026-10-13T09:00:00Z"},
		// Lists of times and intervals.
		{"2026-10-13T00:00:00Z,2026-10-13T01:00:00Z, 2026-10-13T05:00:00Z", "2026-10-13T03:10:00Z", "2026-10-13T05:00:00Z"},
		{"2026-10-01/2026-10-05/P1D,2026-10-08/2026-10-11/P1D", "2026-10-07T00:00:00Z", "2026-10-08T00:00:00Z"},
		{"bad,2026-10-13T00:00:00Z/x/PT1H,2026-10-13T02:00:00Z", "2026-10-13T00:00:00Z", "2026-10-13T02:00:00Z"},
	} {
		d := TimeDimension{Values: tc.values}
		got, ok := d.Nearest(at(tc.t))
		if !ok || !got.Equal(at(tc.want)) {
			t.Errorf("%s at %s: got %v, %v, want %s", tc.values, tc.t, got, ok, tc.want)
		}
	}
	for _, values := range []string{"", "bad", "2026-10-13/2026-10-12/P1D", "2026-10-13/2026-10-14/P1M", "a/b"} {
		d := TimeDimension{Values: values}
		if _, ok := d.Nearest(time.Now()); ok {
			t.Errorf("expected no time for %q", values)
		}
	}
}

func TestParsePeriod(t *testing.T) {
	for s, want := range map[string]time.Duration{
		"PT5M":    5 * time.Minute,
		"PT1H":    time.Hour,
		"PT1H30M": 90 * time.Minute,
		"P1D":     24 * time.Hour,
		"P1DT12H": 36 * time.Hour,
		"P2W":     14 * 24 * time.Hour,
		"PT0.5S":  500 * time.Millisecond,
	} {
		if got, err := ParsePeriod(s); err != nil || got != want {
			t.Errorf("%s: got %v, %v, want %v", s, got, err, want)
		}
	}
	for _, s := range []string{"", "P", "5M", "P1M", "P1Y", "PTM", "PT1D", "P1H"} {
		if _, err := ParsePeriod(s); err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
}

func TestFormatTime(t *testing.T) {
	v := time.Date(2026, 10, 13, 12, 5, 0, 0, time.FixedZone("CEST", 2*3600))
	if got := FormatTime(v); got != "2026-10-13T10:05:00.000Z" {
		t.Errorf("got %s", got)
	}
}

func TestCapabilitiesRequest(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fail" {
			http.Error(w, "nope", http.StatusInternalServerError)
			return
		}
		query = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(testCapabilities))
	}))
	defer srv.Close()
	c := &Client{Client: httpx.Client{HTTP: srv.Client()}}

	// The layer's own capabilities are used if given.
	layer := &Layer{URL: srv.URL + "/ows", Capabilities: srv.URL + "/dwd/Radar/ows?x=1"}
	caps, err := c.GetCapabilities(context.Background(), layer)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := caps.Layer("dwd:Radar"); !ok {
		t.Error("layer not found")
	}
	if query != "/dwd/Radar/ows?x=1&REQUEST=GetCapabilities&SERVICE=WMS&VERSION=1.3.0" {
		t.Errorf("unexpected request %s", query)
	}
	if _, err := c.GetCapabilities(context.Background(), &Layer{URL: srv.URL + "/fail"}); err == nil {
		t.Error("expected error")
	}
}

func TestMapURLTime(t *testing.T) {
	l := &Layer{URL: "https://example.com/wms", Layers: "a", Time: "2026-10-13T10:05:00.000Z"}
	if u := l.TileURL(geo.TileKey{}); !strings.Contains(u, "TIME=2026-10-13T10%3A05%3A00.000Z") {
		t.Errorf("missing time in %s", u)
	}
	l.Time = ""
	if u := l.TileURL(geo.TileKey{}); strings.Contains(u, "TIME") {
		t.Errorf("unexpected time in %s", u)
	}
}

func TestTimeDimensionBounds(t *testing.T) {
	d := TimeDimension{Values: "2026-10-13T06:00:00Z, 2026-10-12T00:00:00Z/2026-10-13T11:00:00Z/PT3H,bad"}
	first, last, ok := d.Bounds()
	want1, want2 := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	if !ok || !first.Equal(want1) || !last.Equal(want2) {
		t.Errorf("got %v %v %v", first, last, ok)
	}
	if _, _, ok := (&TimeDimension{Values: "bad"}).Bounds(); ok {
		t.Error("expected no bounds")
	}
}

func TestTimeDimensionLatest(t *testing.T) {
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	runs := TimeDimension{Values: "2026-10-12T18:00:00.000Z,2026-10-13T00:00:00.000Z,2026-10-13T06:00:00.000Z,2026-10-13T12:00:00.000Z"}
	got := runs.Latest(at("2026-10-13T09:00:00Z"), 4)
	if len(got) != 3 || !got[0].Equal(at("2026-10-13T06:00:00Z")) || !got[2].Equal(at("2026-10-12T18:00:00Z")) {
		t.Errorf("unexpected runs %v", got)
	}
	if got := runs.Latest(at("2026-10-13T12:00:00Z"), 1); len(got) != 1 || !got[0].Equal(at("2026-10-13T12:00:00Z")) {
		t.Errorf("a run at the time itself should be first, got %v", got)
	}
	if got := runs.Latest(at("2026-10-12T00:00:00Z"), 4); len(got) != 0 {
		t.Errorf("expected no runs before the first, got %v", got)
	}
	// Intervals, also between and after their steps.
	steps := TimeDimension{Values: "2026-10-13T00:00:00Z/2026-10-13T10:00:00Z/PT5M"}
	got = steps.Latest(at("2026-10-13T09:03:00Z"), 2)
	if len(got) != 2 || !got[0].Equal(at("2026-10-13T09:00:00Z")) || !got[1].Equal(at("2026-10-13T08:55:00Z")) {
		t.Errorf("unexpected steps %v", got)
	}
	if got := steps.Latest(at("2026-10-14T00:00:00Z"), 1); len(got) != 1 || !got[0].Equal(at("2026-10-13T10:00:00Z")) {
		t.Errorf("expected the last step, got %v", got)
	}
}

func TestReferenceRequests(t *testing.T) {
	doc := `<WMS_Capabilities><Capability><Layer><Name>A</Name>
<Dimension name="time">2026-10-13T00:00:00Z/2026-10-14T00:00:00Z/PT1H</Dimension>
<Dimension name="REFERENCE_TIME" default="2026-10-13T12:00:00.000Z">2026-10-13T06:00:00.000Z,2026-10-13T12:00:00.000Z</Dimension>
</Layer></Capability></WMS_Capabilities>`
	c, err := ParseCapabilities(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	l, _ := c.Layer("A")
	if l.Reference != "2026-10-13T12:00:00.000Z" || l.References == nil || !strings.Contains(l.References.Values, "06:00") {
		t.Errorf("unexpected reference times %+v", l)
	}
	layer := &Layer{URL: "https://example.com/wms", Layers: "A", Reference: "2026-10-13T06:00:00.000Z"}
	if u := layer.TileURL(geo.TileKey{}); !strings.Contains(u, "DIM_REFERENCE_TIME=2026-10-13T06%3A00%3A00.000Z") {
		t.Errorf("missing reference time in %s", u)
	}
}

func TestServiceException(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml;charset=UTF-8")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><ServiceExceptionReport version="1.3.0">   <ServiceException>
      Rendering process failed. No data available
</ServiceException></ServiceExceptionReport>`))
	}))
	defer srv.Close()
	c := &Client{Client: httpx.Client{HTTP: srv.Client()}}
	_, err := c.GetData(context.Background(), srv.URL, time.Time{})
	if !errors.Is(err, ErrServiceException) || !strings.HasSuffix(err.Error(), ": Rendering process failed. No data available") {
		t.Errorf("unexpected error %v", err)
	}
}

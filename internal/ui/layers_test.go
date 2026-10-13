package ui

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/wms"
	"go.yaml.in/yaml/v3"
)

func TestWMSConfigYAML(t *testing.T) {
	cfg := DefaultWMS()
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	sections := 0
	for _, m := range cfg.Maps {
		if m.Section != "" {
			sections++
		}
	}
	for _, o := range cfg.Overlays {
		if o.Section != "" {
			sections++
		}
	}
	// Overlay layers are inlined, options only written where set, and sections without layer fields.
	if strings.Contains(string(data), "layer:") || strings.Contains(string(data), "transparent") ||
		strings.Count(string(data), "section:") != sections || strings.Contains(string(data), `name: ""`) {
		t.Errorf("unexpected keys in:\n%s", data)
	}

	var got WMSConfig
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Errorf("got %+v, want %+v", got, cfg)
	}
}

func TestWMSConfigSections(t *testing.T) {
	data := `
maps:
  - section: Germany
  - name: A
    url: https://a.example.com
  - name: B
    url: https://b.example.com
  - section: World
  - section: Global
  - name: C
    url: https://c.example.com
  - section: Trailing
overlays:
  - name: D
    url: https://d.example.com
    opacity: 0.5
  - section: Other
  - name: E
    url: https://e.example.com
    refresh: 5m
`
	var got WMSConfig
	if err := yaml.Unmarshal([]byte(data), &got); err != nil {
		t.Fatal(err)
	}
	maps, mapSections := splitSections(got.Maps, func(m Map) (string, string) { return m.Section, m.Layer.Name })
	overlays, overlaySections := splitSections(got.Overlays, func(o Overlay) (string, Overlay) { return o.Section, o })
	if !slices.Equal(maps, []string{"A", "B", "C"}) || len(overlays) != 2 || overlays[0].Opacity != 0.5 ||
		overlays[1].Refresh != 5*time.Minute {
		t.Errorf("unexpected layers %v %+v", maps, overlays)
	}
	wantMaps := []Section{{0, "Germany"}, {2, "World"}, {2, "Global"}}
	if !reflect.DeepEqual(mapSections, wantMaps) || !reflect.DeepEqual(overlaySections, []Section{{1, "Other"}}) {
		t.Errorf("unexpected sections %+v %+v", mapSections, overlaySections)
	}

	for _, bad := range []string{
		"maps:\n  - section: X\n    name: A\n",
		"overlays:\n  - section: X\n    opacity: 0.5\n",
		"maps:\n  - {}\n",
	} {
		if err := yaml.Unmarshal([]byte(bad), new(WMSConfig)); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestOverlayRefreshYAML(t *testing.T) {
	data, err := yaml.Marshal(Overlay{Layer: wms.Layer{Name: "Radar"}, Refresh: 5 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "refresh: 5m0s") {
		t.Errorf("unexpected refresh in:\n%s", data)
	}
	var o Overlay
	if err := yaml.Unmarshal(data, &o); err != nil || o.Refresh != 5*time.Minute {
		t.Errorf("got %v, %v", o.Refresh, err)
	}
}

func TestWMSConfigWithoutDisabled(t *testing.T) {
	data := `
maps:
  - section: Germany
  - name: A
    url: https://a.example.com
    enabled: false
  - section: World
  - name: B
    url: https://b.example.com
  - name: C
    url: https://c.example.com
    enabled: false
  - section: Empty
overlays:
  - name: D
    url: https://d.example.com
    enabled: true
  - name: E
    url: https://e.example.com
    enabled: false
`
	var cfg WMSConfig
	if err := yaml.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatal(err)
	}
	got := cfg.WithoutDisabled()
	var names []string
	for _, m := range got.Maps {
		names = append(names, m.Section+m.Layer.Name)
	}
	// The section of only disabled maps is dropped, an empty one is kept as before.
	if !slices.Equal(names, []string{"World", "B", "Empty"}) {
		t.Errorf("unexpected maps %v", names)
	}
	if len(got.Overlays) != 1 || got.Overlays[0].Layer.Name != "D" {
		t.Errorf("unexpected overlays %+v", got.Overlays)
	}
	// The configuration itself is unchanged, so that it is saved with the disabled layers.
	if len(cfg.Maps) != 6 || len(cfg.Overlays) != 2 {
		t.Error("configuration was changed")
	}
}

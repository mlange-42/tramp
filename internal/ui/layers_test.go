package ui

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestWMSConfigYAML(t *testing.T) {
	cfg := DefaultWMS()
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Overlay layers are inlined, and options only written where set.
	if strings.Contains(string(data), "layer:") || strings.Contains(string(data), "transparent") ||
		strings.Count(string(data), "section:") != 5 || strings.Contains(string(data), `name: ""`) {
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
`
	var got WMSConfig
	if err := yaml.Unmarshal([]byte(data), &got); err != nil {
		t.Fatal(err)
	}
	maps, mapSections := splitSections(got.Maps, func(m Map) (string, string) { return m.Section, m.Layer.Name })
	overlays, overlaySections := splitSections(got.Overlays, func(o Overlay) (string, Overlay) { return o.Section, o })
	if !slices.Equal(maps, []string{"A", "B", "C"}) || len(overlays) != 2 || overlays[0].Opacity != 0.5 {
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

package ui

import (
	"reflect"
	"slices"
	"testing"

	"github.com/mlange-42/tramp/internal/wms"
	"go.yaml.in/yaml/v3"
)

func TestDefaultPresetsLayers(t *testing.T) {
	maps, _ := splitSections(DefaultMaps(), func(m Map) (string, string) { return m.Section, m.Layer.Name })
	overlays, _ := splitSections(DefaultOverlays(), func(o Overlay) (string, string) { return o.Section, o.Layer.Name })
	cfg := DefaultPresets()
	presets, err := cfg.Valid()
	if err != nil || len(presets) != len(cfg.Presets) {
		t.Fatalf("invalid default presets: %v", err)
	}
	for _, p := range presets {
		if !slices.Contains(maps, p.Map) {
			t.Errorf("preset %q: unknown map %q", p.Name, p.Map)
		}
		for _, o := range p.Overlays {
			if !slices.Contains(overlays, o) {
				t.Errorf("preset %q: unknown overlay %q", p.Name, o)
			}
		}
	}
}

func TestPresetConfigYAML(t *testing.T) {
	cfg := DefaultPresets()
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	var got PresetConfig
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Errorf("got %+v, want %+v", got, cfg)
	}
}

func TestPresetConfigValid(t *testing.T) {
	cfg := PresetConfig{Presets: []Preset{
		{Name: " A ", Map: "M"},
		{Name: "", Map: "M"},
		{Name: "A", Map: "N"},
		{Name: "B", Map: "N"},
	}}
	presets, err := cfg.Valid()
	if err == nil {
		t.Error("expected errors for the empty and the duplicate name")
	}
	if want := []Preset{{Name: "A", Map: "M"}, {Name: "B", Map: "N"}}; !reflect.DeepEqual(presets, want) {
		t.Errorf("got %+v, want %+v", presets, want)
	}
}

func TestApplyPreset(t *testing.T) {
	none := func(name string) Map { return Map{Layer: wms.Layer{Name: name, Type: wms.TypeNone}} }
	overlay := func(name string) Overlay { return Overlay{Layer: wms.Layer{Name: name, Type: wms.TypeNone}} }
	a := newApp(func() {}, Options{
		Maps:     []Map{none("M1"), none("M2")},
		Overlays: []Overlay{overlay("O1"), overlay("O2"), overlay("O3")},
	})
	a.overlaySel.SetChecked(0, true)
	a.setOverlay(0, true)
	defer a.closeTiles()

	p := Preset{Name: "P", Map: "M2", Overlays: []string{"O2", "O3", "X"}}
	if a.presetActive(p) {
		t.Error("preset active before applying it")
	}
	missing := a.applyPreset(p)
	if !reflect.DeepEqual(missing, []string{"X"}) {
		t.Errorf("missing %v, want [X]", missing)
	}
	if a.layerSelect.Selected() != 1 {
		t.Errorf("map %d, want 1", a.layerSelect.Selected())
	}
	for i, want := range []bool{false, true, true} {
		if a.overlaySel.Checked(i) != want || (a.overlays[i].tiles != nil) != want {
			t.Errorf("overlay %d: checked %v, tiles %v, want %v", i, a.overlaySel.Checked(i), a.overlays[i].tiles != nil, want)
		}
	}
	if !a.presetActive(p) {
		t.Error("preset not active after applying it")
	}

	// A missing map keeps the current one.
	missing = a.applyPreset(Preset{Name: "Q", Map: "X"})
	if !reflect.DeepEqual(missing, []string{"X"}) || a.layerSelect.Selected() != 1 {
		t.Errorf("missing %v, map %d", missing, a.layerSelect.Selected())
	}
}

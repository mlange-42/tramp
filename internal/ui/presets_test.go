package ui

import (
	"image"
	"reflect"
	"slices"
	"testing"
	"time"

	"gioui.org/widget"
	"github.com/mlange-42/tramp/internal/wms"
	"github.com/ncruces/zenity"
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

func TestPresetMenu(t *testing.T) {
	none := func(name string) Map { return Map{Layer: wms.Layer{Name: name, Type: wms.TypeNone}} }
	a := newApp(func() {}, Options{
		Maps:     []Map{none("M1"), none("M2")},
		Overlays: []Overlay{{Layer: wms.Layer{Name: "O1", Type: wms.TypeNone}}},
		Presets:  []Preset{{Name: "P1", Map: "M1"}, {Name: "P2", Map: "M2", Overlays: []string{"O1"}}},
	})
	t.Cleanup(a.closeTiles)
	u := &uiTester{a: a, size: image.Pt(1000, 700)}
	u.frame()

	a.presetMenu.open = true
	u.frame()
	a.presetMenu.items[1].Click()
	u.frame()
	u.frame()
	if a.presetMenu.open {
		t.Error("menu still open")
	}
	if a.layerSelect.Selected() != 1 || !a.overlaySel.Checked(0) {
		t.Errorf("map %d, overlay %v", a.layerSelect.Selected(), a.overlaySel.Checked(0))
	}
	if a.presetActive(a.presets[0]) || !a.presetActive(a.presets[1]) {
		t.Error("wrong active preset")
	}
}

func TestSaveDeletePreset(t *testing.T) {
	none := func(name string) Map { return Map{Layer: wms.Layer{Name: name, Type: wms.TypeNone}} }
	var saved []Preset
	a := newApp(func() {}, Options{
		Maps:        []Map{none("M1"), none("M2")},
		Overlays:    []Overlay{{Layer: wms.Layer{Name: "O1", Type: wms.TypeNone}}},
		Presets:     []Preset{{Name: "P1", Map: "M1"}},
		SavePresets: func(p []Preset) error { saved = p; return nil },
	})
	t.Cleanup(a.closeTiles)
	u := &uiTester{a: a, size: image.Pt(1000, 700)}
	u.frame()

	var entered string
	var confirm error
	askText = func(string, ...zenity.Option) (string, error) {
		if entered == "" {
			return "", zenity.ErrCanceled
		}
		return entered, nil
	}
	question = func(string, ...zenity.Option) error { return confirm }
	t.Cleanup(func() { askText, question = zenity.Entry, zenity.Question })
	// choose clicks an entry of the opened menu, and waits for the dialogs.
	choose := func(btn func() *widget.Clickable) {
		t.Helper()
		a.presetMenu.open = true
		u.frame()
		btn().Click()
		u.frame()
		for start := time.Now(); a.dialogOpen.Load(); {
			if time.Since(start) > 5*time.Second {
				t.Fatal("dialog not finished")
			}
			time.Sleep(time.Millisecond)
		}
		u.frame()
	}
	save := func() *widget.Clickable { return &a.presetMenu.save }

	a.setLayer(1)
	a.overlaySel.SetChecked(0, true)
	a.setOverlay(0, true)

	// Canceled: nothing stored.
	choose(save)
	if len(a.presets) != 1 || saved != nil {
		t.Fatalf("presets %v, saved %v", a.presets, saved)
	}
	// A new name is added.
	entered = " P2 "
	choose(save)
	want := []Preset{{Name: "P1", Map: "M1"}, {Name: "P2", Map: "M2", Overlays: []string{"O1"}}}
	if !reflect.DeepEqual(a.presets, want) || !reflect.DeepEqual(saved, want) {
		t.Fatalf("presets %v, saved %v", a.presets, saved)
	}
	// An existing name is replaced only if confirmed.
	entered, confirm = "P1", zenity.ErrCanceled
	choose(save)
	if !reflect.DeepEqual(a.presets, want) {
		t.Fatalf("replaced without confirmation: %v", a.presets)
	}
	confirm = nil
	choose(save)
	want[0] = Preset{Name: "P1", Map: "M2", Overlays: []string{"O1"}}
	if !reflect.DeepEqual(a.presets, want) || !reflect.DeepEqual(saved, want) {
		t.Fatalf("presets %v, saved %v", a.presets, saved)
	}

	// Deleting asks first.
	confirm = zenity.ErrCanceled
	choose(func() *widget.Clickable { return &a.presetMenu.removes[0] })
	if len(a.presets) != 2 {
		t.Fatalf("deleted without confirmation: %v", a.presets)
	}
	confirm = nil
	choose(func() *widget.Clickable { return &a.presetMenu.removes[0] })
	if want = want[1:]; !reflect.DeepEqual(a.presets, want) || !reflect.DeepEqual(saved, want) {
		t.Fatalf("presets %v, saved %v", a.presets, saved)
	}
}

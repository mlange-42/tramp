package ui

import (
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
)

// PresetsFile is the name of the layer presets file in the settings directory.
const PresetsFile = "presets.yaml"

// PresetConfig are the layer presets, as stored in [PresetsFile].
type PresetConfig struct {
	Presets []Preset `yaml:"presets"`
}

// Preset is a stored combination of a background map and overlays.
type Preset struct {
	// Name is shown in the list of presets. It must be unique.
	Name string `yaml:"name"`
	// Map is the name of the background map.
	Map string `yaml:"map"`
	// Overlays are the names of the enabled overlays.
	Overlays []string `yaml:"overlays,omitempty"`
}

// DefaultPresets returns the built-in presets. They refer to the layers of [DefaultWMS].
func DefaultPresets() PresetConfig {
	const hillshade = "Hillshade Germany (basemap.de)"
	return PresetConfig{Presets: []Preset{
		{Name: "Weather", Map: "Natural Earth", Overlays: []string{"Precipitation radar", "Weather warnings", "Borders & places"}},
		{Name: "Hiking", Map: "OpenTopoMap", Overlays: []string{hillshade, "Hiking routes"}},
		{Name: "Cycling", Map: "CyclOSM", Overlays: []string{hillshade, "Cycling routes"}},
		{Name: "Satellite", Map: "Satellite", Overlays: []string{"Labels & roads (bright)"}},
	}}
}

// Valid returns the presets that have a unique name, and an error for each other one.
func (c *PresetConfig) Valid() ([]Preset, error) {
	var presets []Preset
	var errs []error
	for i, p := range c.Presets {
		p.Name = strings.TrimSpace(p.Name)
		switch {
		case p.Name == "":
			errs = append(errs, fmt.Errorf("preset %d: missing name", i+1))
		case slices.ContainsFunc(presets, func(o Preset) bool { return o.Name == p.Name }):
			errs = append(errs, fmt.Errorf("preset %d: duplicate name %q", i+1, p.Name))
		default:
			presets = append(presets, p)
		}
	}
	return presets, errors.Join(errs...)
}

// applyPreset selects the map and the overlays of a preset, and disables the other overlays.
// It returns the names of the map and overlays that are not available. A missing map keeps the current one.
func (a *App) applyPreset(p Preset) (missing []string) {
	if i := slices.Index(a.layerNames, p.Map); i < 0 {
		missing = append(missing, p.Map)
	} else if i != a.layerSelect.Selected() {
		a.setLayer(i)
	}
	for _, name := range p.Overlays {
		if !slices.Contains(a.overlayNames, name) {
			missing = append(missing, name)
		}
	}
	for i, name := range a.overlayNames {
		if on := slices.Contains(p.Overlays, name); on != a.overlaySel.Checked(i) {
			a.overlaySel.SetChecked(i, on)
			a.setOverlay(i, on)
		}
	}
	if len(missing) > 0 {
		log.Printf("preset %q: layers not available: %s", p.Name, strings.Join(missing, ", "))
	}
	return missing
}

// presetActive reports whether the selected map and overlays are those of the preset.
// Unavailable layers of the preset are ignored.
func (a *App) presetActive(p Preset) bool {
	if i := slices.Index(a.layerNames, p.Map); i >= 0 && i != a.layerSelect.Selected() {
		return false
	}
	for i, name := range a.overlayNames {
		if slices.Contains(p.Overlays, name) != a.overlaySel.Checked(i) {
			return false
		}
	}
	return true
}

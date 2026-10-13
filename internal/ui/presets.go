package ui

import (
	"errors"
	"fmt"
	"image"
	"log"
	"slices"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/ncruces/zenity"
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
		{Name: "Hiking", Map: "OpenTopoMap", Overlays: []string{hillshade, "Hiking routes"}},
		{Name: "Cycling", Map: "CyclOSM", Overlays: []string{hillshade, "Cycling routes"}},
		{Name: "Weather", Map: "Natural Earth", Overlays: []string{"Precipitation radar", "Weather warnings", "Borders & places"}},
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

// presetTip returns the tooltip of a preset, with its map and its overlays on separate lines.
func presetTip(p Preset) string {
	return strings.Join(append([]string{p.Map}, p.Overlays...), "\n")
}

// askText shows a dialog for entering a text. It is a variable, so that tests can answer it.
var askText = zenity.Entry

// currentPreset returns the selected map and the enabled overlays as a preset without a name.
func (a *App) currentPreset() Preset {
	p := Preset{Map: a.layerNames[a.layerSelect.Selected()]}
	for i, name := range a.overlayNames {
		if a.overlaySel.Checked(i) {
			p.Overlays = append(p.Overlays, name)
		}
	}
	return p
}

// savePresetAs asks for a name, and stores the current map and overlays under it.
// An existing preset with this name is replaced after asking.
func (a *App) savePresetAs() {
	p := a.currentPreset()
	names := make([]string, len(a.presets))
	for i, q := range a.presets {
		names[i] = q.Name
	}
	opts := a.plat.dialogOptions()
	a.dialog(func() func() {
		name, err := askText("Name of the preset for the current map and overlays:", append([]zenity.Option{
			zenity.Title("Save preset"),
			zenity.OKLabel("Save"),
			zenity.CancelLabel("Cancel"),
		}, opts...)...)
		p.Name = strings.TrimSpace(name)
		if err != nil || p.Name == "" {
			return nil
		}
		if slices.Contains(names, p.Name) {
			if question(fmt.Sprintf("Replace the preset %q?", p.Name), append([]zenity.Option{
				zenity.Title("Replace preset"),
				zenity.OKLabel("Replace"),
				zenity.CancelLabel("Cancel"),
				zenity.WarningIcon,
			}, opts...)...) != nil {
				return nil
			}
		}
		return func() { a.putPreset(p) }
	})
}

// putPreset replaces the preset with the same name, or adds it at the end, and stores the presets.
func (a *App) putPreset(p Preset) {
	if i := slices.IndexFunc(a.presets, func(q Preset) bool { return q.Name == p.Name }); i >= 0 {
		a.presets[i] = p
	} else {
		a.presets = append(a.presets, p)
	}
	a.storePresets()
}

// deletePreset asks whether to delete preset i, and deletes it.
func (a *App) deletePreset(i int) {
	name := a.presets[i].Name
	opts := a.plat.dialogOptions()
	a.dialog(func() func() {
		if question(fmt.Sprintf("Delete the preset %q?", name), append([]zenity.Option{
			zenity.Title("Delete preset"),
			zenity.OKLabel("Delete"),
			zenity.CancelLabel("Cancel"),
			zenity.WarningIcon,
		}, opts...)...) != nil {
			return nil
		}
		return func() {
			a.presets = slices.DeleteFunc(a.presets, func(q Preset) bool { return q.Name == name })
			a.storePresets()
		}
	})
}

// storePresets saves the presets, if they can be saved.
func (a *App) storePresets() {
	if a.savePresets == nil {
		return
	}
	if err := a.savePresets(slices.Clone(a.presets)); err != nil {
		log.Printf("saving presets: %v", err)
	}
}

// presetAction is what was chosen in the preset menu.
type presetAction struct {
	// apply and remove are the index of the preset to apply or to delete, or -1.
	apply, remove int
	// save stores the current map and overlays as a preset.
	save bool
}

// noPresetAction is a [presetAction] with nothing chosen.
var noPresetAction = presetAction{apply: -1, remove: -1}

// saveLabel is the label of the menu item for storing a preset.
const saveLabel = "Save current as…"

// presetMenu is the drop-down list of the layer presets, opened by a button that shows only an icon.
type presetMenu struct {
	popup
	// items apply the presets, and removes delete them.
	items, removes []widget.Clickable
	save           widget.Clickable
}

// update processes input for n presets, and returns what was chosen.
func (m *presetMenu) update(gtx layout.Context, n int) presetAction {
	m.popup.update(gtx)
	if len(m.items) != n {
		m.items, m.removes = make([]widget.Clickable, n), make([]widget.Clickable, n)
	}
	act := noPresetAction
	for i := range m.items {
		if m.items[i].Clicked(gtx) {
			act.apply = i
		}
		if m.removes[i].Clicked(gtx) {
			act.remove = i
		}
	}
	act.save = m.save.Clicked(gtx)
	if act != noPresetAction {
		m.open = false
	}
	return act
}

// layout draws the button, and the list of presets if open. The active ones have a check mark.
// Each preset has a button for deleting it, and the last entry stores the current map and overlays.
func (m *presetMenu) layout(gtx layout.Context, st *Style, presets []Preset, active func(Preset) bool) layout.Dimensions {
	m.tips = m.tips[:0]
	for _, p := range presets {
		m.tips = append(m.tips, presetTip(p))
	}
	return m.popup.layout(gtx, st, "", nil, func(gtx layout.Context) layout.Dimensions {
		inset := gtx.Dp(st.Spacing / 2)
		size, delSize := gtx.Dp(st.IconSize), gtx.Dp(st.IconSize)*4/5
		labels := []string{saveLabel}
		for _, p := range presets {
			labels = append(labels, p.Name)
		}
		// The entries are equally wide, with the delete buttons aligned at the right.
		nameW := maxLabelWidth(gtx, st, labels)
		delW := delSize + 2*inset
		width := 3*inset + size + nameW + delW

		list := func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(st.Spacing/2).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := st.Label("No presets")
				l.Color = st.HintFg
				return l.Layout(gtx)
			})
		}
		if len(presets) > 0 {
			list = func(gtx layout.Context) layout.Dimensions {
				return m.layoutOptions(gtx, st, len(presets), func(i int) layout.Widget {
					return func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return m.layoutItem(gtx, st, &m.items[i], active(presets[i]), presets[i].Name, nameW)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Clickable(gtx, &m.removes[i], func(gtx layout.Context) layout.Dimensions {
									return layout.UniformInset(st.Spacing/2).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										layoutIcon(gtx, iconDelete, image.Point{}, delSize, st.HintFg)
										return layout.Dimensions{Size: image.Pt(delSize, delSize)}
									})
								})
							}),
						)
					}
				}, func(i int) bool { return m.items[i].Hovered() })
			}
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(list),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				h := gtx.Dp(st.Spacing)
				line := image.Rect(0, h/2, width, h/2+max(1, gtx.Dp(1)))
				paint.FillShape(gtx.Ops, st.DividerColor, clip.Rect(line).Op())
				return layout.Dimensions{Size: image.Pt(width, h)}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return m.layoutItem(gtx, st, &m.save, false, saveLabel, nameW+delW)
			}),
		)
	})
}

// layoutItem draws an entry of the menu, with room for a check mark before the label,
// which is at least labelW wide.
func (m *presetMenu) layoutItem(gtx layout.Context, st *Style, btn *widget.Clickable, checked bool, label string, labelW int) layout.Dimensions {
	return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(st.Spacing/2).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			size := gtx.Dp(st.IconSize)
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if checked {
						layoutIcon(gtx, iconCheck, image.Point{}, size, st.Theme.ContrastBg)
					}
					return layout.Dimensions{Size: image.Pt(size, size)}
				}),
				layout.Rigid(layout.Spacer{Width: st.Spacing / 2}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = labelW
					return st.Label(label).Layout(gtx)
				}),
			)
		})
	})
}

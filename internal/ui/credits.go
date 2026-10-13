package ui

import (
	"slices"
	"strings"

	"github.com/mlange-42/tramp/internal/wms"
)

// creditSep separates the parts of short credits, like "© OpenStreetMap · CyclOSM".
const creditSep = " · "

// credit is the attribution of a data source in use, as the licenses and terms of the services require.
type credit struct {
	// use is what the source is used for, like "Map" or "Elevation", and name the name of the layer or service.
	use, name string
	// full is the full credit, short the short one shown on the map, with parts separated by [creditSep],
	// and url a page with the terms or the credits of the data.
	full, short, url string
}

// layerCredit returns the credit of a map layer.
func layerCredit(use string, l *wms.Layer) credit {
	return credit{use: use, name: l.Name, full: l.Attribution, short: l.AttributionShort, url: l.AttributionURL}
}

// mapCredits returns the credits of the sources shown on the map: the background map, the enabled overlays,
// and the elevation source. Sources without any credit are left out.
func (a *App) mapCredits() []credit {
	var cs []credit
	add := func(c credit) {
		if c.full != "" || c.short != "" {
			cs = append(cs, c)
		}
	}
	if a.tiles != nil {
		add(layerCredit("Map", &a.layers[a.layerSelect.Selected()]))
	}
	for i := range a.overlays {
		if o := &a.overlays[i]; o.tiles != nil {
			add(layerCredit("Overlay", &o.Layer))
		}
	}
	if s := a.elev.service; s != nil {
		src := s.Source()
		add(credit{use: "Elevation", name: src.Name, full: src.Attribution, short: src.AttributionShort, url: src.AttributionURL})
	}
	return cs
}

// shortCredits returns the short credits joined into one line, with each part shown once,
// like "© OpenStreetMap · CyclOSM · Waymarked Trails". Credits without a short form show their full one.
func shortCredits(cs []credit) string {
	var parts []string
	for _, c := range cs {
		s := c.short
		if s == "" {
			s = c.full
		}
		for p := range strings.SplitSeq(s, creditSep) {
			if p = strings.TrimSpace(p); p != "" && !slices.Contains(parts, p) {
				parts = append(parts, p)
			}
		}
	}
	return strings.Join(parts, creditSep)
}

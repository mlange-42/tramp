package ui

import (
	"slices"

	"gioui.org/f32"
	"gioui.org/layout"
	"github.com/mlange-42/tramp/internal/mapview"
)

// viewOption is a feature of the map view that can be switched on and off in the Map menu.
type viewOption int

// The view options, in menu order.
const (
	showTrackColors viewOption = iota
	showRouteColors
	showRoutePoints
	showWaypointNames
	showRoutePointNames
	showHoverNames
	showMuted
	showColorLegend
	showScaleBar
	numViewOptions
)

// viewOptionInfo describes a view option.
type viewOptionInfo struct {
	// key is the name in the settings file.
	key   string
	label string
	// def is the state if the settings don't have it.
	def bool
}

var viewOptions = [numViewOptions]viewOptionInfo{
	showTrackColors:     {key: "track_colors", label: "Colored tracks", def: true},
	showRouteColors:     {key: "route_colors", label: "Colored routes", def: true},
	showRoutePoints:     {key: "route_points", label: "Route points", def: false},
	showWaypointNames:   {key: "waypoint_names", label: "Waypoint names", def: false},
	showRoutePointNames: {key: "route_point_names", label: "Route point names", def: false},
	showHoverNames:      {key: "hover_names", label: "Names on hover", def: true},
	showMuted:           {key: "muted", label: "Muted outside chart zoom", def: true},
	showColorLegend:     {key: "color_legend", label: "Color legend", def: true},
	showScaleBar:        {key: "scale_bar", label: "Scale bar", def: true},
}

// initViewOptions sets the view options from the settings, with the defaults for missing ones.
func (a *App) initViewOptions(show map[string]bool) {
	for i, o := range viewOptions {
		v, ok := show[o.key]
		if !ok {
			v = o.def
		}
		a.flipped[i] = v != o.def
	}
}

// showing reports whether a view option is on.
func (a *App) showing(o viewOption) bool {
	return viewOptions[o].def != a.flipped[o]
}

// viewOptionsState returns the view options for saving them.
func (a *App) viewOptionsState() map[string]bool {
	m := make(map[string]bool, numViewOptions)
	for i, o := range viewOptions {
		m[o.key] = a.showing(viewOption(i))
	}
	return m
}

// viewOptionItems returns the menu items of the view options.
func (a *App) viewOptionItems() []menuItem {
	items := make([]menuItem, numViewOptions)
	for i, o := range viewOptions {
		items[i] = menuItem{label: o.label, check: true, checked: a.showing(viewOption(i)), action: func() { a.toggleViewOption(viewOption(i)) }}
	}
	return items
}

// toggleViewOption switches a view option.
func (a *App) toggleViewOption(o viewOption) {
	a.flipped[o] = !a.flipped[o]
	switch o {
	case showTrackColors, showRouteColors, showRoutePoints, showMuted:
		a.tracksChanged = true
	}
}

// colored reports whether the item's lines are colored by value, if a metric is selected.
func (a *App) colored(it *fileItem) bool {
	switch it.kind {
	case trackItem:
		return a.showing(showTrackColors)
	case routeItem, navItem:
		return a.showing(showRouteColors)
	}
	return false
}

// coloredValues returns the values of the item's lines for the selected metric, or nil if it is not colored.
func (a *App) coloredValues(it *fileItem) [][]float64 {
	if !a.colored(it) {
		return nil
	}
	return it.lineValues(a.colorMetric())
}

// itemDots returns the dots of an item drawn on the map, and their names:
// those with a symbol, and the plain route points if switched on, below them.
func (a *App) itemDots(it *fileItem) ([]mapview.Dot, []string) {
	if len(it.plainDots) == 0 || !a.showing(showRoutePoints) {
		return it.dots, it.dotNames
	}
	return slices.Concat(it.plainDots, it.dots), slices.Concat(it.plainNames, it.dotNames)
}

// namesShown reports whether the names of all points of the item are drawn, see [App.layoutPointNames].
func (a *App) namesShown(it *fileItem) bool {
	switch it.kind {
	case waypointItem:
		return a.showing(showWaypointNames)
	case routeItem:
		return a.showing(showRoutePointNames)
	}
	return false
}

// layoutPointNames draws the names of the waypoints and route points of the shown files, if switched on,
// except those of the edited file, whose names are always shown, see [App.layoutEditMap].
// Route point names are drawn also for points not drawn themselves.
func (a *App) layoutPointNames(gtx layout.Context) {
	view := &a.mapView.View
	w, h := float32(gtx.Constraints.Max.X), float32(gtx.Constraints.Max.Y)
	visible := func(p f32.Point) bool { return p.X >= 0 && p.Y >= 0 && p.X <= w && p.Y <= h }
	draw := func(dots []mapview.Dot, names []string) {
		for i, d := range dots {
			if i >= len(names) || names[i] == "" {
				continue
			}
			x, y := view.ToScreen(d.Pos)
			a.drawPointLabel(gtx, f32.Pt(float32(x), float32(y)), a.dotRadius(gtx, d.Icon != nil), names[i], visible)
		}
	}
	for _, it := range a.shownItems() {
		if !a.namesShown(it) || (a.editing != nil && slices.Contains(a.editing.items, it)) {
			continue
		}
		draw(it.dots, it.dotNames)
		draw(it.plainDots, it.plainNames)
	}
}

// dotRadius returns the radius of a waypoint dot on the map, or of a marker if it has a symbol.
func (a *App) dotRadius(gtx layout.Context, marker bool) float32 {
	size := a.waypointSize
	if marker {
		size = a.symbolSize
	}
	return float32(gtx.Dp(size)) / 2
}

package ui

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/geocode"
	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/routing"
	"github.com/mlange-42/tramp/internal/settings"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

var (
	iconDirections = mustIcon(icons.MapsDirections)
	iconSwap       = mustIcon(icons.ActionSwapVert)
	iconAddVia     = mustIcon(icons.ContentAddCircleOutline)
	iconRemoveVia  = mustIcon(icons.ContentRemoveCircleOutline)
	// profileIcons are the icons of [routing.Profiles].
	profileIcons = []*widget.Icon{
		mustIcon(icons.MapsDirectionsWalk),
		mustIcon(icons.MapsDirectionsBike),
		mustIcon(icons.MapsDirectionsCar),
	}
	profileHelp = []string{"On foot", "By bike", "By car"}
)

// navigator shows a route from a start to a target on the map, found by a routing service.
// It is independent of the routes in GPX files. The route is shown while the panel is open,
// so that the attribution of the service is always shown with it. It is kept when the panel is closed,
// and its points are saved in the settings.
type navigator struct {
	open bool
	// btn opens and closes the panel, from the toolbar.
	btn iconButton
	// closeBtn closes the panel, clearBtn removes the route, swapBtn reverses it, viaBtn adds a via point,
	// and profileBtns select the profile.
	closeBtn, clearBtn, swapBtn, viaBtn iconButton
	profileBtns                         []iconButton
	// profile is the index of the selected profile in [routing.Profiles].
	profile int
	// report opens the page for reporting map errors.
	report widget.Clickable
	// panel is the event tag for the panel background.
	panel int

	// stops are the start, the via points, and the target, in route order. There are at least two.
	// Stops without position are set by a click on the map, or by searching them.
	stops []*navStop
	// pressing is set from a press on the map while picking a stop, until the pointer moves too far for a click.
	// pressAt is where the press was, also for dragging a stop.
	pressing bool
	pressAt  f32.Point
	// drag is the stop being dragged, if dragging. dragNew is set for a via point inserted by dragging the route,
	// and fromPos and fromName are the position and the name before for others.
	// moved is set once the pointer moved farther than for a click.
	dragging bool
	drag     int
	dragNew  bool
	fromPos  geo.LonLat
	fromName string
	moved    bool
	// swallow is set from a press on an alternative route on the map, which selects it, until the release.
	swallow bool
	// stale is set while the route of the stops was not requested yet, after loading them from the settings.
	// It is requested when the panel is opened.
	stale bool

	// seq numbers the routing requests, so that results of an outdated request are dropped.
	seq    int
	cancel context.CancelFunc
	busy   bool
	err    error
	route  *routing.Route
	// alts are alternative routes, for a route without via points, if the service found some.
	alts []*routing.Route
	// line is the route in map coordinates, and altLines the alternatives, for hitting them with the pointer.
	line     []mapview.Polyline
	altLines []mapview.Polyline
	lines    mapview.Lines
}

// navAlternatives is the largest number of alternative routes asked for.
const navAlternatives = 2

// navStop is a point of the route, with the field for searching it.
type navStop struct {
	// pos is the position, if set.
	pos geo.LonLat
	set bool
	// name is the name of the searched place at pos, or empty.
	name string

	field     searchBox
	removeBtn iconButton
}

// label returns the text of the stop's field: the name of the place, or the coordinates.
func (s *navStop) label() string {
	switch {
	case !s.set:
		return ""
	case s.name != "":
		return s.name
	}
	return fmt.Sprintf("%.5f, %.5f", s.pos.Lat, s.pos.Lon)
}

// newRouter returns the router for the configured routing service.
// An unknown service is logged, and the default is used.
func newRouter(cfg settings.Routing) routing.Router {
	client := &http.Client{Timeout: 30 * time.Second}
	r, err := routing.New(cfg.Service, cfg.URL, client, UserAgent)
	if err != nil {
		log.Printf("routing: %v", err)
		r, _ = routing.New("", "", client, UserAgent)
	}
	return r
}

// profileIndex returns the index of a profile in [routing.Profiles], or 0 for an unknown one.
func profileIndex(p string) int {
	return max(0, slices.Index(routing.Profiles, p))
}

// unset returns the index of the first stop without position, if any.
func (n *navigator) unset() (int, bool) {
	for i, s := range n.stops {
		if !s.set {
			return i, true
		}
	}
	return 0, false
}

// picking reports whether the next click on the map sets a stop.
func (n *navigator) picking() bool {
	_, ok := n.unset()
	return n.open && ok
}

// isVia reports whether stop i is a via point, not the start or the target.
func (n *navigator) isVia(i int) bool {
	return i > 0 && i < len(n.stops)-1
}

// points returns the positions of the stops that are set, in route order, and their indices in the stops.
func (n *navigator) points() ([]geo.LonLat, []int) {
	var pts []geo.LonLat
	var idx []int
	for i, s := range n.stops {
		if s.set {
			pts = append(pts, s.pos)
			idx = append(idx, i)
		}
	}
	return pts, idx
}

// routable reports whether start and target are set.
func (n *navigator) routable() bool {
	return len(n.stops) >= 2 && n.stops[0].set && n.stops[len(n.stops)-1].set
}

// setPoints replaces the stops by the given positions. With fewer than two, the missing start or target is unset.
func (n *navigator) setPoints(pts []geo.LonLat) {
	n.stops = n.stops[:0]
	for _, p := range pts {
		n.stops = append(n.stops, &navStop{pos: p, set: true})
	}
	for len(n.stops) < 2 {
		n.stops = append(n.stops, &navStop{})
	}
}

// updateNav handles the panel, clicks on the map while picking stops,
// and dragging the stops and the route. It takes the primary button events on the map,
// and returns those it didn't use, for the editor.
func (a *App) updateNav(gtx layout.Context, events []mapview.PointerEvent) []mapview.PointerEvent {
	n := &a.nav
	if len(n.stops) < 2 {
		n.setPoints(nil)
	}
	if n.btn.Clicked(gtx) {
		a.toggleNav()
	}
	if n.closeBtn.Clicked(gtx) {
		a.closeNav()
	}
	if n.clearBtn.Clicked(gtx) {
		a.clearNav()
	}
	for i := range n.profileBtns {
		if n.profileBtns[i].Clicked(gtx) && i != n.profile {
			n.profile = i
			a.routingCfg.Profile = routing.Profiles[i]
			a.startRoute()
		}
	}
	if n.swapBtn.Clicked(gtx) && !n.dragging {
		slices.Reverse(n.stops)
		a.startRoute()
	}
	if n.viaBtn.Clicked(gtx) && !n.dragging {
		// A new via point goes before the target, and is set by the next click on the map, or by searching it.
		st := &navStop{}
		n.stops = slices.Insert(n.stops, len(n.stops)-1, st)
		gtx.Execute(key.FocusCmd{Tag: &st.field.ed})
	}
	if n.report.Clicked(gtx) {
		openURL(routing.FixTheMapURL)
	}
	// The panel takes presses and scrolling, so that they don't reach the map below.
	for {
		_, ok := gtx.Event(pointer.Filter{
			Target:  &n.panel,
			Kinds:   pointer.Press | pointer.Scroll,
			ScrollY: pointer.ScrollRange{Min: math.MinInt32, Max: math.MaxInt32},
		})
		if !ok {
			break
		}
	}
	if !n.open {
		return events
	}
	a.updateNavFields(gtx)
	// Esc closes the panel, unless the editor or a text field takes it.
	if a.editing == nil && !a.searchFocused(gtx) {
		for {
			ev, ok := gtx.Event(key.Filter{Name: key.NameEscape})
			if !ok {
				break
			}
			if e, ok := ev.(key.Event); ok && e.State == key.Press {
				a.closeNav()
			}
		}
	}
	// Clicking the map ends typing in a field.
	if len(events) > 0 && a.searchFocused(gtx) && !gtx.Focused(&a.search.ed) {
		gtx.Execute(key.FocusCmd{})
	}
	return slices.DeleteFunc(events, func(ev mapview.PointerEvent) bool {
		return a.navPointer(gtx, ev)
	})
}

// updateNavFields handles the search fields and remove buttons of the stops,
// and shows the stops in the fields that are not edited.
func (a *App) updateNavFields(gtx layout.Context) {
	n := &a.nav
	for i := 0; i < len(n.stops); i++ {
		st := n.stops[i]
		if n.isVia(i) && st.removeBtn.Clicked(gtx) {
			n.stops = slices.Delete(n.stops, i, i+1)
			a.startRoute()
			i--
			continue
		}
		switch p, act := st.field.update(gtx, a); act {
		case searchPicked:
			a.setNavStop(i, p)
		case searchCleared:
			a.unsetNavStop(i)
		}
		if !gtx.Focused(&st.field.ed) && !st.field.open {
			if txt := st.label(); st.field.ed.Text() != txt {
				st.field.ed.SetText(txt)
			}
		}
	}
}

// setNavStop sets stop i to a found place and finds the route.
// If the place is outside the map view, the map shows all stops, or the place if it is the only one.
func (a *App) setNavStop(i int, p geocode.Place) {
	st := a.nav.stops[i]
	st.pos, st.set, st.name = p.Pos, true, p.Name
	if !a.mapView.View.Bounds().Contains(geo.EmptyRect().Extend(geo.ToMercator(p.Pos))) {
		pts, _ := a.nav.points()
		if len(pts) == 1 {
			a.showPlace(p)
		} else {
			a.gps.following = false
			a.mapView.Fit(navBounds(pts))
		}
	}
	a.startRoute()
}

// navMargin is the share of the extent of the stops added on each side when showing them.
const navMargin = 0.15

// navBounds returns the extent of the positions in map coordinates, with a margin of [navMargin].
func navBounds(pts []geo.LonLat) geo.Rect {
	r := geo.EmptyRect()
	for _, p := range pts {
		r = r.Extend(geo.ToMercator(p))
	}
	dx, dy := (r.Max.X-r.Min.X)*navMargin, (r.Max.Y-r.Min.Y)*navMargin
	r.Min.X, r.Min.Y, r.Max.X, r.Max.Y = r.Min.X-dx, r.Min.Y-dy, r.Max.X+dx, r.Max.Y+dy
	return r
}

// unsetNavStop removes the position of stop i, so that it is set again by a click on the map, or by searching it.
func (a *App) unsetNavStop(i int) {
	st := a.nav.stops[i]
	st.set, st.name = false, ""
	a.startRoute()
}

// navPointer handles a primary button event on the map, and reports whether it used it.
//
// Pressing on a stop drags it, and pressing on the route drags a new via point from there.
// The route is found again when released. Clicking a via point without moving it removes it.
// While picking, a click without moving sets the first stop without position.
func (a *App) navPointer(gtx layout.Context, ev mapview.PointerEvent) bool {
	n := &a.nav
	if !n.open {
		return false
	}
	if n.dragging {
		a.navDrag(ev)
		return true
	}
	if n.swallow {
		n.swallow = ev.Kind != pointer.Release && ev.Kind != pointer.Cancel
		return true
	}
	if ev.Kind == pointer.Press {
		if i, ok := a.navHit(gtx, ev.Screen); ok {
			n.dragging, n.drag, n.dragNew, n.moved = true, i, false, false
			n.fromPos, n.fromName = n.stops[i].pos, n.stops[i].name
			n.pressAt = ev.Screen
			return true
		}
		if i, ok := a.navHitLine(gtx, ev.Pos); ok {
			n.stops = slices.Insert(n.stops, i, &navStop{pos: geo.ToLonLat(ev.Pos), set: true})
			n.dragging, n.drag, n.dragNew, n.moved = true, i, true, false
			n.pressAt = ev.Screen
			return true
		}
		if k, ok := a.navHitAlt(gtx, ev.Pos); ok {
			a.selectNavAlt(k)
			n.swallow = true
			return true
		}
	}
	if !n.picking() {
		return false
	}
	switch ev.Kind {
	case pointer.Press:
		n.pressing, n.pressAt = true, ev.Screen
	case pointer.Drag:
		n.pressing = n.pressing && dist2(ev.Screen, n.pressAt) <= dragSlop*dragSlop
	case pointer.Release:
		if i, ok := n.unset(); ok && n.pressing {
			st := n.stops[i]
			st.pos, st.set, st.name = geo.ToLonLat(ev.Pos), true, ""
			a.startRoute()
		}
		n.pressing = false
	case pointer.Cancel:
		n.pressing = false
	}
	return true
}

// navDrag handles a pointer event while dragging a stop.
func (a *App) navDrag(ev mapview.PointerEvent) {
	n := &a.nav
	i := n.drag
	st := n.stops[i]
	switch ev.Kind {
	case pointer.Drag:
		n.moved = n.moved || dist2(ev.Screen, n.pressAt) > dragSlop*dragSlop
		if n.moved {
			st.pos, st.name = geo.ToLonLat(ev.Pos), ""
		}
	case pointer.Release:
		n.dragging = false
		switch {
		case n.moved:
			st.pos, st.name = geo.ToLonLat(ev.Pos), ""
			a.startRoute()
		case n.dragNew:
			// A click on the route adds nothing.
			n.stops = slices.Delete(n.stops, i, i+1)
		case n.isVia(i):
			// A click on a via point removes it.
			n.stops = slices.Delete(n.stops, i, i+1)
			a.startRoute()
		}
	case pointer.Cancel:
		n.dragging = false
		if n.dragNew {
			n.stops = slices.Delete(n.stops, i, i+1)
		} else {
			st.pos, st.name = n.fromPos, n.fromName
		}
	}
}

// navHit returns the stop whose marker is at the screen position.
// The markers drawn on top are checked first: the target, the start, and the via points from last to first.
func (a *App) navHit(gtx layout.Context, p f32.Point) (int, bool) {
	n := &a.nav
	view := &a.mapView.View
	ring := float32(gtx.Dp(2)) + dragSlop
	endR := float32(gtx.Dp(a.style.NavMarkerSize))/2 + ring
	viaR := float32(gtx.Dp(a.style.NavViaSize))/2 + ring
	last := len(n.stops) - 1
	order := make([]int, 0, len(n.stops))
	order = append(order, last, 0)
	for i := last - 1; i > 0; i-- {
		order = append(order, i)
	}
	for _, i := range order {
		st := n.stops[i]
		if !st.set {
			continue
		}
		r := endR
		if n.isVia(i) {
			r = viaR
		}
		if dist2(screenPos(view, st.pos), p) <= r*r {
			return i, true
		}
	}
	return 0, false
}

// navHitLine reports whether the route passes near the map position, and returns the index in the stops
// at which a via point at the position is inserted: after the via points the route passes before it.
func (a *App) navHitLine(gtx layout.Context, p geo.Point) (int, bool) {
	n := &a.nav
	if !n.routable() || len(n.line) == 0 || n.busy {
		return 0, false
	}
	radius := float64(gtx.Dp(a.style.HoverRadius)) * a.mapView.View.Resolution()
	_, seg, _, ok := nearestSegment(n.line, p, radius)
	if !ok {
		return 0, false
	}
	pts, idx := n.points()
	return idx[viaInsertIndex(n.line[0].Points, pts, seg)], true
}

// navHitAlt returns the alternative route that passes nearest to the map position, if one passes near it.
func (a *App) navHitAlt(gtx layout.Context, p geo.Point) (int, bool) {
	n := &a.nav
	if len(n.altLines) == 0 || n.busy {
		return 0, false
	}
	radius := float64(gtx.Dp(a.style.HoverRadius)) * a.mapView.View.Resolution()
	k, _, _, ok := nearestSegment(n.altLines, p, radius)
	return k, ok
}

// selectNavAlt makes alternative k the route, and the route an alternative.
func (a *App) selectNavAlt(k int) {
	n := &a.nav
	n.route, n.alts[k] = n.alts[k], n.route
	a.updateNavLines()
}

// viaInsertIndex returns the index in points at which a new via point on segment seg of the route line is inserted.
// Each via point is located on the line as the nearest line point after the one of the previous via point,
// and the new point goes after those located before the segment.
func viaInsertIndex(line []geo.Point, points []geo.LonLat, seg int) int {
	from := 0
	i := 1
	for ; i < len(points)-1; i++ {
		v := geo.ToMercator(points[i])
		best, bestD := from, math.Inf(1)
		for k := from; k < len(line); k++ {
			dx, dy := line[k].X-v.X, line[k].Y-v.Y
			if d := dx*dx + dy*dy; d < bestD {
				best, bestD = k, d
			}
		}
		if best > seg {
			break
		}
		from = best
	}
	return i
}

// navCursor returns the pointer cursor for the route on the map: for dragging a stop,
// over a stop or the route, or for picking a stop. It returns false if the route doesn't set the cursor.
func (a *App) navCursor(gtx layout.Context) (pointer.Cursor, bool) {
	n := &a.nav
	m := a.mapView
	switch {
	case !n.open:
		return pointer.CursorDefault, false
	case n.dragging:
		return pointer.CursorGrabbing, true
	}
	if m.HoverValid {
		x, y := m.View.ToScreen(m.Hover)
		if _, ok := a.navHit(gtx, f32.Pt(float32(x), float32(y))); ok {
			return pointer.CursorGrab, true
		}
		if _, ok := a.navHitLine(gtx, m.Hover); ok {
			return pointer.CursorGrab, true
		}
		if _, ok := a.navHitAlt(gtx, m.Hover); ok {
			return pointer.CursorPointer, true
		}
	}
	if n.picking() {
		return pointer.CursorCrosshair, true
	}
	return pointer.CursorDefault, false
}

// toggleNav opens or closes the panel. Opening shows the last route again,
// and finds it first if it was loaded from the settings.
func (a *App) toggleNav() {
	if a.nav.open {
		a.closeNav()
		return
	}
	n := &a.nav
	n.open = true
	if len(n.profileBtns) != len(routing.Profiles) {
		n.profileBtns = make([]iconButton, len(routing.Profiles))
	}
	if len(n.stops) < 2 {
		n.setPoints(nil)
	}
	if n.stale {
		n.stale = false
		a.startRoute()
	}
}

// closeNav closes the panel. The route is kept for opening it again.
func (a *App) closeNav() {
	n := &a.nav
	if n.dragging {
		a.navDrag(mapview.PointerEvent{Kind: pointer.Cancel})
	}
	for _, st := range n.stops {
		st.field.clear()
	}
	n.open, n.pressing, n.swallow = false, false, false
}

// clearNav removes the route and its stops, for picking a new start.
func (a *App) clearNav() {
	a.stopRoute()
	n := &a.nav
	for _, st := range n.stops {
		st.field.clear()
	}
	n.setPoints(nil)
	n.pressing, n.dragging, n.stale = false, false, false
	n.err = nil
	a.setNavRoute(nil)
}

// navState returns the routing settings with the stops that are set, with their positions and names.
func (a *App) navState() settings.Routing {
	cfg := a.routingCfg
	cfg.Points = nil
	for _, st := range a.nav.stops {
		if st.set {
			cfg.Points = append(cfg.Points, settings.RoutePoint{Lon: roundCoord(st.pos.Lon), Lat: roundCoord(st.pos.Lat), Name: st.name})
		}
	}
	return cfg
}

// coordDigits is the number of decimals of saved route points, about 1 m.
const coordDigits = 5

// roundCoord rounds a coordinate in degrees to [coordDigits] decimals.
func roundCoord(v float64) float64 {
	f := math.Pow10(coordDigits)
	return math.Round(v*f) / f
}

// loadNav takes the stops of the last route from the settings. The route is found when the panel is opened.
func (a *App) loadNav(cfg settings.Routing) {
	n := &a.nav
	pts := make([]geo.LonLat, len(cfg.Points))
	for i, p := range cfg.Points {
		pts[i] = geo.LonLat{Lon: p.Lon, Lat: p.Lat}
	}
	n.setPoints(pts)
	for i, p := range cfg.Points {
		n.stops[i].name = p.Name
	}
	n.stale = n.routable()
}

// startRoute finds the route through the stops in the background, if start and target are set.
// Via points without position are left out. The previous route stays shown until the new one arrives.
func (a *App) startRoute() {
	a.stopRoute()
	n := &a.nav
	n.err = nil
	if !n.routable() || a.router == nil {
		a.setNavRoute(nil)
		return
	}
	n.seq++
	seq := n.seq
	ctx, cancel := context.WithCancel(context.Background())
	n.cancel, n.busy = cancel, true
	pts, _ := n.points()
	req := routing.Request{Points: pts, Profile: routing.Profiles[n.profile], Alternatives: navAlternatives}
	router := a.router
	go func() {
		route, err := router.Route(ctx, req)
		a.runOnUI(func() {
			if seq != n.seq {
				return
			}
			cancel()
			n.cancel, n.busy, n.err = nil, false, err
			a.setNavRoute(route)
		})
	}()
}

// stopRoute cancels a running routing request.
func (a *App) stopRoute() {
	n := &a.nav
	if n.cancel != nil {
		n.cancel()
		n.cancel = nil
	}
	n.seq++
	n.busy = false
}

// setNavRoute shows a route and its alternatives on the map, or none if nil.
func (a *App) setNavRoute(r *routing.Route) {
	n := &a.nav
	n.route, n.alts = r, nil
	if r != nil {
		n.alts, r.Alternatives = r.Alternatives, nil
	}
	a.updateNavLines()
}

// updateNavLines updates the lines of the route and its alternatives on the map.
// The alternatives are drawn below the route.
func (a *App) updateNavLines() {
	n := &a.nav
	n.line, n.altLines = nil, nil
	if n.route == nil {
		n.lines.Set(nil, nil)
		return
	}
	n.line = []mapview.Polyline{navPolyline(n.route)}
	for _, r := range n.alts {
		n.altLines = append(n.altLines, navPolyline(r))
	}
	st := a.style
	var groups []mapview.LineGroup
	if len(n.altLines) > 0 {
		groups = append(groups,
			mapview.LineGroup{Color: st.NavCasing, Width: st.NavAltWidth + 2*st.NavCasingWidth, Lines: n.altLines},
			mapview.LineGroup{Color: st.NavAlt, Width: st.NavAltWidth, Lines: n.altLines},
		)
	}
	groups = append(groups,
		mapview.LineGroup{Color: st.NavCasing, Width: st.NavLineWidth + 2*st.NavCasingWidth, Lines: n.line},
		mapview.LineGroup{Color: st.NavLine, Width: st.NavLineWidth, Lines: n.line},
	)
	n.lines.Set(groups, nil)
}

// navPolyline returns the line of a route in map coordinates.
func navPolyline(r *routing.Route) mapview.Polyline {
	pts := make([]geo.Point, len(r.Points))
	for i, p := range r.Points {
		pts[i] = geo.ToMercator(p)
	}
	return mapview.NewPolyline(pts)
}

// navStatus returns the text about the route for the panel: a hint on what to do,
// the state of the request, or the length and duration of the route.
func (a *App) navStatus() string {
	n := &a.nav
	switch {
	case len(n.stops) < 2:
		return ""
	case !n.stops[0].set:
		return "Click the map to set the start, or search it"
	case !n.stops[len(n.stops)-1].set:
		return "Click the map to set the target, or search it"
	case n.busy:
		return "Routing…"
	case n.err != nil:
		msg := n.err.Error()
		msg = strings.ToUpper(msg[:1]) + msg[1:]
		if strings.Contains(msg, "max distance limit") {
			msg += ". For longer routes, set the routing service osrm in the settings."
		}
		return msg
	case n.route != nil:
		return formatDistance(n.route.Distance) + " · " + formatDuration(n.route.Duration)
	}
	return ""
}

// layoutNavMap draws the route and its stops on the map.
func (a *App) layoutNavMap(gtx layout.Context) {
	n := &a.nav
	if !n.open || len(n.stops) < 2 {
		return
	}
	st := a.style
	n.lines.Layout(gtx, &a.mapView.View)
	a.drawNavBand(gtx)
	last := len(n.stops) - 1
	for i := 1; i < last; i++ {
		if n.stops[i].set {
			a.drawNavVia(gtx, n.stops[i].pos)
		}
	}
	if n.stops[last].set {
		a.drawNavEnd(gtx, n.stops[last].pos, "B", st.NavTarget)
	}
	if n.stops[0].set {
		a.drawNavEnd(gtx, n.stops[0].pos, "A", st.NavStart)
	}
}

// drawNavBand draws straight lines from a dragged stop to its neighbors with position, while dragging.
func (a *App) drawNavBand(gtx layout.Context) {
	n := &a.nav
	if !n.dragging || !n.moved {
		return
	}
	view := &a.mapView.View
	pts, idx := n.points()
	k := slices.Index(idx, n.drag)
	if k < 0 {
		return
	}
	var path clip.Path
	path.Begin(gtx.Ops)
	if k > 0 {
		path.MoveTo(screenPos(view, pts[k-1]))
		path.LineTo(screenPos(view, pts[k]))
	} else {
		path.MoveTo(screenPos(view, pts[k]))
	}
	if k < len(pts)-1 {
		path.LineTo(screenPos(view, pts[k+1]))
	}
	paint.FillShape(gtx.Ops, a.style.NavBand, clip.Stroke{Path: path.End(), Width: float32(gtx.Dp(2))}.Op())
}

// drawNavVia draws the marker of a via point: a white dot with a ring in the color of the route.
func (a *App) drawNavVia(gtx layout.Context, ll geo.LonLat) {
	st := a.style
	c := screenPos(&a.mapView.View, ll)
	r := float32(gtx.Dp(st.NavViaSize)) / 2
	ring := float32(max(1, gtx.Dp(2)))
	paint.FillShape(gtx.Ops, st.NavLine, circle(c, r+ring).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, st.HoverRing, circle(c, r).Op(gtx.Ops))
}

// drawNavEnd draws the marker of the start or the target: a dot with a letter.
func (a *App) drawNavEnd(gtx layout.Context, ll geo.LonLat, label string, col color.NRGBA) {
	st := a.style
	c := screenPos(&a.mapView.View, ll)
	r := float32(gtx.Dp(st.NavMarkerSize)) / 2
	ring := float32(max(1, gtx.Dp(2)))
	paint.FillShape(gtx.Ops, color.NRGBA{A: 0x40}, circle(c.Add(f32.Pt(0, 1)), r+ring+1).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, st.HoverRing, circle(c, r+ring).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, col, circle(c, r).Op(gtx.Ops))
	layoutNavLetter(gtx, st, c, r, label, st.HoverRing)
}

// layoutNavLetter draws a letter centered at c, in a circle of radius r.
func layoutNavLetter(gtx layout.Context, st *Style, c f32.Point, r float32, label string, col color.NRGBA) {
	l := st.SmallLabel(label)
	l.Color = col
	l.Font.Weight = font.Bold
	gtx.Constraints = layout.Constraints{Max: image.Pt(int(4*r), int(4*r))}
	macro := op.Record(gtx.Ops)
	dims := l.Layout(gtx)
	call := macro.Stop()
	defer op.Offset(image.Pt(int(c.X)-dims.Size.X/2, int(c.Y)-dims.Size.Y/2)).Push(gtx.Ops).Pop()
	call.Add(gtx.Ops)
}

// layoutNavPanel draws the panel at the top right of the map, below the directions button, if open.
func (a *App) layoutNavPanel(gtx layout.Context) {
	n := &a.nav
	if !n.open {
		return
	}
	st := a.style
	layout.NE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(st.Spacing).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			w := min(gtx.Dp(st.NavWidth), gtx.Constraints.Max.X)
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
			gtx.Constraints.Min.Y = 0
			return layoutPanel(gtx, st, &n.panel, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(st.Spacing/2).Layout(gtx, a.layoutNavContent)
			})
		})
	})
}

// layoutNavContent draws the content of the panel: the profiles and buttons, the stops, the route and the attribution.
func (a *App) layoutNavContent(gtx layout.Context) layout.Dimensions {
	st, n := a.style, &a.nav
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	small := func(txt string, col color.NRGBA) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := st.SmallLabel(txt)
			l.Color = col
			return l.Layout(gtx)
		})
	}
	gap := layout.Rigid(layout.Spacer{Height: st.Spacing / 2}.Layout)

	// The panel is at the right edge of the window, so the tooltips extend to the left.
	for _, b := range []*iconButton{&n.swapBtn, &n.viaBtn, &n.clearBtn, &n.closeBtn} {
		b.tip.right = true
	}
	for i := range n.profileBtns {
		n.profileBtns[i].tip.right = true
	}
	header := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, 2*len(n.profileBtns)+8)
		for i := range n.profileBtns {
			if i > 0 {
				children = append(children, layout.Rigid(layout.Spacer{Width: 1}.Layout))
			}
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return n.profileBtns[i].Layout(gtx, st, profileIcons[i], profileHelp[i], true, i == n.profile)
			}))
		}
		children = append(children,
			layout.Flexed(1, layout.Spacer{}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return n.viaBtn.Layout(gtx, st, iconAddVia, "Add a via point before the target", true, false)
			}),
			layout.Rigid(layout.Spacer{Width: 1}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return n.swapBtn.Layout(gtx, st, iconSwap, "Reverse the route", true, false)
			}),
			layout.Rigid(layout.Spacer{Width: 1}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return n.clearBtn.Layout(gtx, st, iconDelete, "Remove the route, to start a new one", true, false)
			}),
			layout.Rigid(layout.Spacer{Width: 1}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return n.closeBtn.Layout(gtx, st, iconClose, "Close (Esc). The route is kept for the next time", true, false)
			}),
		)
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
	})

	children := []layout.FlexChild{header, gap}
	for i := range n.stops {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutNavStop(gtx, i)
		}))
	}

	statusCol := st.Theme.Fg
	if n.err != nil {
		statusCol = st.EditActive
	}
	children = append(children, gap,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := st.Label(a.navStatus())
			l.Color = statusCol
			if n.route != nil && n.err == nil && !n.busy {
				l.Font.Weight = font.Bold
			}
			return l.Layout(gtx)
		}),
	)
	if n.route != nil && !n.busy {
		children = append(children, small("Drag the route to change it", st.HintFg))
		if len(n.alts) > 0 {
			children = append(children, small(navAltsHint(n.alts), st.HintFg))
		}
	}
	if a.router != nil {
		children = append(children, gap, small(a.router.Attribution(), st.HintFg),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return n.report.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					l := st.SmallLabel("Report a map error")
					l.Color = st.Theme.ContrastBg
					if n.report.Hovered() {
						l.Color = st.Theme.Fg
					}
					pointer.CursorPointer.Add(gtx.Ops)
					return l.Layout(gtx)
				})
			}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// navAltsHint returns the hint on the alternative routes, with their lengths and durations.
func navAltsHint(alts []*routing.Route) string {
	parts := make([]string, len(alts))
	for i, r := range alts {
		parts[i] = formatDistance(r.Distance) + " · " + formatDuration(r.Duration)
	}
	what := "an alternative"
	if len(alts) > 1 {
		what = "alternatives"
	}
	return fmt.Sprintf("Click a grey route for %s: %s", what, strings.Join(parts, ", "))
}

// layoutNavStop draws the row of stop i: its marker, its search field, and a button to remove a via point.
func (a *App) layoutNavStop(gtx layout.Context, i int) layout.Dimensions {
	st, n := a.style, &a.nav
	s := n.stops[i]
	hint, label, col := "Via point: search, or click the map", "", st.NavLine
	switch i {
	case 0:
		hint, label, col = "Start: search, or click the map", "A", st.NavStart
	case len(n.stops) - 1:
		hint, label, col = "Target: search, or click the map", "B", st.NavTarget
	}
	s.removeBtn.tip.right = true
	return layout.Inset{Bottom: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := gtx.Dp(st.NavMarkerSize)
				c := f32.Pt(float32(size)/2, float32(size)/2)
				r := float32(size) / 2
				if label == "" {
					// Via points get a small ring like on the map.
					vr := float32(gtx.Dp(st.NavViaSize)) / 2
					paint.FillShape(gtx.Ops, col, circle(c, vr+float32(gtx.Dp(2))).Op(gtx.Ops))
					paint.FillShape(gtx.Ops, st.HoverRing, circle(c, vr).Op(gtx.Ops))
				} else {
					paint.FillShape(gtx.Ops, col, circle(c, r).Op(gtx.Ops))
					layoutNavLetter(gtx, st, c, r, label, st.HoverRing)
				}
				return layout.Dimensions{Size: image.Pt(size+gtx.Dp(st.Spacing/2), size)}
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return s.field.layout(gtx, a, hint, "Clear the point, to set it again", false)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !n.isVia(i) {
					return layout.Dimensions{}
				}
				return s.removeBtn.LayoutFlat(gtx, st, iconRemoveVia, st.HintFg, "Remove the via point")
			}),
		)
	})
}

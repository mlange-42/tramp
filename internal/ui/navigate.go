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
	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/routing"
	"github.com/mlange-42/tramp/internal/settings"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

var (
	iconDirections = mustIcon(icons.MapsDirections)
	iconSwap       = mustIcon(icons.ActionSwapVert)
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
	// closeBtn closes the panel, clearBtn removes the route, swapBtn reverses it, profileBtns select the profile.
	closeBtn, clearBtn, swapBtn iconButton
	profileBtns       []iconButton
	// profile is the index of the selected profile in [routing.Profiles].
	profile int
	// report opens the page for reporting map errors.
	report widget.Clickable
	// panel is the event tag for the panel background.
	panel int

	// points are the start, the via points, and the target, in route order.
	// While picking, there is only the start, or nothing.
	points []geo.LonLat
	// stale is set while the route of the points was not requested yet, after loading them from the settings.
	// It is requested when the panel is opened.
	stale bool
	// pressing is set from a press on the map while picking an end, until the pointer moves too far for a click.
	// pressAt is where the press was, also for dragging a point.
	pressing bool
	pressAt  f32.Point
	// drag is the point being dragged, if dragging. dragNew is set for a via point inserted by dragging the route,
	// and dragFrom is the position before for others. moved is set once the pointer moved farther than for a click.
	dragging bool
	drag     int
	dragNew  bool
	dragFrom geo.LonLat
	moved    bool

	// seq numbers the routing requests, so that results of an outdated request are dropped.
	seq    int
	cancel context.CancelFunc
	busy   bool
	err    error
	route  *routing.Route
	// line is the route in map coordinates, for hitting it with the pointer.
	line  []mapview.Polyline
	lines mapview.Lines
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

// picking reports whether the next click on the map sets the start or the target.
func (n *navigator) picking() bool {
	return n.open && len(n.points) < 2
}

// isVia reports whether point i is a via point, not the start or the target.
func (n *navigator) isVia(i int) bool {
	return i > 0 && i < len(n.points)-1
}

// updateNav handles the panel, clicks on the map while picking the start or the target,
// and dragging the points and the route. It takes the primary button events on the map,
// and returns those it didn't use, for the editor.
func (a *App) updateNav(gtx layout.Context, events []mapview.PointerEvent) []mapview.PointerEvent {
	n := &a.nav
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
		slices.Reverse(n.points)
		a.startRoute()
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
	return slices.DeleteFunc(events, func(ev mapview.PointerEvent) bool {
		return a.navPointer(gtx, ev)
	})
}

// navPointer handles a primary button event on the map, and reports whether it used it.
//
// Pressing on a point drags it, and pressing on the route drags a new via point from there.
// The route is found again when released. Clicking a via point without moving it removes it.
// While picking, a click without moving sets the start, or the target if the start is set.
func (a *App) navPointer(gtx layout.Context, ev mapview.PointerEvent) bool {
	n := &a.nav
	if !n.open {
		return false
	}
	if n.dragging {
		a.navDrag(ev)
		return true
	}
	if ev.Kind == pointer.Press {
		if i, ok := a.navHit(gtx, ev.Screen); ok {
			n.dragging, n.drag, n.dragNew, n.dragFrom, n.moved = true, i, false, n.points[i], false
			n.pressAt = ev.Screen
			return true
		}
		if i, ok := a.navHitLine(gtx, ev.Pos); ok {
			n.points = slices.Insert(n.points, i, geo.ToLonLat(ev.Pos))
			n.dragging, n.drag, n.dragNew, n.moved = true, i, true, false
			n.pressAt = ev.Screen
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
		if n.pressing && n.picking() {
			n.points = append(n.points, geo.ToLonLat(ev.Pos))
			a.startRoute()
		}
		n.pressing = false
	case pointer.Cancel:
		n.pressing = false
	}
	return true
}

// navDrag handles a pointer event while dragging a point.
func (a *App) navDrag(ev mapview.PointerEvent) {
	n := &a.nav
	i := n.drag
	switch ev.Kind {
	case pointer.Drag:
		n.moved = n.moved || dist2(ev.Screen, n.pressAt) > dragSlop*dragSlop
		if n.moved {
			n.points[i] = geo.ToLonLat(ev.Pos)
		}
	case pointer.Release:
		n.dragging = false
		switch {
		case n.moved:
			n.points[i] = geo.ToLonLat(ev.Pos)
			a.startRoute()
		case n.dragNew:
			// A click on the route adds nothing.
			n.points = slices.Delete(n.points, i, i+1)
		case n.isVia(i):
			// A click on a via point removes it.
			n.points = slices.Delete(n.points, i, i+1)
			a.startRoute()
		}
	case pointer.Cancel:
		n.dragging = false
		if n.dragNew {
			n.points = slices.Delete(n.points, i, i+1)
		} else {
			n.points[i] = n.dragFrom
		}
	}
}

// navHit returns the point whose marker is at the screen position.
// The markers drawn on top are checked first: the target, the start, and the via points from last to first.
func (a *App) navHit(gtx layout.Context, p f32.Point) (int, bool) {
	n := &a.nav
	view := &a.mapView.View
	ring := float32(gtx.Dp(2)) + dragSlop
	endR := float32(gtx.Dp(a.style.NavMarkerSize))/2 + ring
	viaR := float32(gtx.Dp(a.style.NavViaSize))/2 + ring
	last := len(n.points) - 1
	order := make([]int, 0, len(n.points))
	if last > 0 {
		order = append(order, last)
	}
	if last >= 0 {
		order = append(order, 0)
	}
	for i := last - 1; i > 0; i-- {
		order = append(order, i)
	}
	for _, i := range order {
		r := endR
		if n.isVia(i) {
			r = viaR
		}
		if dist2(screenPos(view, n.points[i]), p) <= r*r {
			return i, true
		}
	}
	return 0, false
}

// navHitLine reports whether the route passes near the map position, and returns the index in the points
// at which a via point at the position is inserted: after the via points the route passes before it.
func (a *App) navHitLine(gtx layout.Context, p geo.Point) (int, bool) {
	n := &a.nav
	if len(n.points) < 2 || len(n.line) == 0 || n.busy {
		return 0, false
	}
	radius := float64(gtx.Dp(a.style.HoverRadius)) * a.mapView.View.Resolution()
	_, seg, _, ok := nearestSegment(n.line, p, radius)
	if !ok {
		return 0, false
	}
	return viaInsertIndex(n.line[0].Points, n.points, seg), true
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

// navCursor returns the pointer cursor for the route on the map: for dragging a point,
// over a point or the route, or for picking an end. It returns false if the route doesn't set the cursor.
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
	n.open, n.pressing = false, false
}

// clearNav removes the route and its points, for picking a new start.
func (a *App) clearNav() {
	a.stopRoute()
	n := &a.nav
	n.points, n.pressing, n.dragging, n.stale = nil, false, false, false
	n.err = nil
	a.setNavRoute(nil)
}

// navState returns the routing settings with the points of the route.
func (a *App) navState() settings.Routing {
	cfg := a.routingCfg
	cfg.Points = nil
	for _, p := range a.nav.points {
		cfg.Points = append(cfg.Points, settings.LonLat{Lon: roundCoord(p.Lon), Lat: roundCoord(p.Lat)})
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

// loadNav takes the points of the last route from the settings. The route is found when the panel is opened.
func (a *App) loadNav(cfg settings.Routing) {
	n := &a.nav
	n.points = nil
	for _, p := range cfg.Points {
		n.points = append(n.points, geo.LonLat{Lon: p.Lon, Lat: p.Lat})
	}
	n.stale = len(n.points) >= 2
}

// startRoute finds the route through the points in the background, if start and target are set.
// The previous route stays shown until the new one arrives.
func (a *App) startRoute() {
	a.stopRoute()
	n := &a.nav
	n.err = nil
	if len(n.points) < 2 || a.router == nil {
		a.setNavRoute(nil)
		return
	}
	n.seq++
	seq := n.seq
	ctx, cancel := context.WithCancel(context.Background())
	n.cancel, n.busy = cancel, true
	req := routing.Request{Points: slices.Clone(n.points), Profile: routing.Profiles[n.profile]}
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

// setNavRoute shows a route on the map, or none if nil.
func (a *App) setNavRoute(r *routing.Route) {
	n := &a.nav
	n.route = r
	if r == nil {
		n.line = nil
		n.lines.Set(nil, nil)
		return
	}
	pts := make([]geo.Point, len(r.Points))
	for i, p := range r.Points {
		pts[i] = geo.ToMercator(p)
	}
	n.line = []mapview.Polyline{mapview.NewPolyline(pts)}
	st := a.style
	n.lines.Set([]mapview.LineGroup{
		{Color: st.NavCasing, Width: st.NavLineWidth + 2*st.NavCasingWidth, Lines: n.line},
		{Color: st.NavLine, Width: st.NavLineWidth, Lines: n.line},
	}, nil)
}

// navStatus returns the text about the route for the panel: a hint on what to do,
// the state of the request, or the length and duration of the route.
func (a *App) navStatus() string {
	n := &a.nav
	switch {
	case len(n.points) == 0:
		return "Click the map to set the start"
	case len(n.points) == 1:
		return "Click the map to set the target"
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

// layoutNavMap draws the route and its points on the map.
func (a *App) layoutNavMap(gtx layout.Context) {
	n := &a.nav
	if !n.open {
		return
	}
	st := a.style
	n.lines.Layout(gtx, &a.mapView.View)
	a.drawNavBand(gtx)
	last := len(n.points) - 1
	for i := 1; i < last; i++ {
		a.drawNavVia(gtx, n.points[i])
	}
	if last > 0 {
		a.drawNavEnd(gtx, n.points[last], "B", st.NavTarget)
	}
	if last >= 0 {
		a.drawNavEnd(gtx, n.points[0], "A", st.NavStart)
	}
}

// drawNavBand draws straight lines from a dragged point to its neighbors, while dragging.
func (a *App) drawNavBand(gtx layout.Context) {
	n := &a.nav
	if !n.dragging || !n.moved {
		return
	}
	view := &a.mapView.View
	i := n.drag
	var path clip.Path
	path.Begin(gtx.Ops)
	if i > 0 {
		path.MoveTo(screenPos(view, n.points[i-1]))
		path.LineTo(screenPos(view, n.points[i]))
	} else {
		path.MoveTo(screenPos(view, n.points[i]))
	}
	if i < len(n.points)-1 {
		path.LineTo(screenPos(view, n.points[i+1]))
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

	l := st.SmallLabel(label)
	l.Color = st.HoverRing
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

// layoutNavContent draws the content of the panel: the profiles and buttons, the points, the route and the attribution.
func (a *App) layoutNavContent(gtx layout.Context) layout.Dimensions {
	st, n := a.style, &a.nav
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	small := func(txt string, col color.NRGBA) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := st.SmallLabel(txt)
			l.Color = col
			l.MaxLines = 1
			return l.Layout(gtx)
		})
	}
	gap := layout.Rigid(layout.Spacer{Height: st.Spacing / 2}.Layout)

	// The panel is at the right edge of the window, so the tooltips extend to the left.
	for _, b := range []*iconButton{&n.swapBtn, &n.clearBtn, &n.closeBtn} {
		b.tip.right = true
	}
	for i := range n.profileBtns {
		n.profileBtns[i].tip.right = true
	}
	header := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, 2*len(n.profileBtns)+6)
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
				return n.swapBtn.Layout(gtx, st, iconSwap, "Reverse the route", len(n.points) > 0, false)
			}),
			layout.Rigid(layout.Spacer{Width: 1}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return n.clearBtn.Layout(gtx, st, iconDelete, "Remove the route, to start a new one", len(n.points) > 0, false)
			}),
			layout.Rigid(layout.Spacer{Width: 1}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return n.closeBtn.Layout(gtx, st, iconClose, "Close (Esc). The route is kept for the next time", true, false)
			}),
		)
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
	})

	end := func(label, unset string, i int) layout.FlexChild {
		if i < 0 {
			return small(label+"  "+unset, st.HintFg)
		}
		return small(label+"  "+n.points[i].String(), st.Theme.Fg)
	}
	start, target := end("A", "Start", -1), end("B", "Target", -1)
	if len(n.points) > 0 {
		start = end("A", "", 0)
	}
	if len(n.points) > 1 {
		target = end("B", "", len(n.points)-1)
	}
	children := []layout.FlexChild{header, gap, start}
	if vias := len(n.points) - 2; vias > 0 {
		txt := "1 via point"
		if vias > 1 {
			txt = fmt.Sprintf("%d via points", vias)
		}
		children = append(children, small("   "+txt, st.HintFg))
	}

	statusCol := st.Theme.Fg
	if n.err != nil {
		statusCol = st.EditActive
	}
	children = append(children, target, gap,
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
	}
	if a.router != nil {
		children = append(children, gap, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := st.SmallLabel(a.router.Attribution())
			l.Color = st.HintFg
			return l.Layout(gtx)
		}),
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

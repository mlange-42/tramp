package ui

import (
	"context"
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
// It is independent of the routes in GPX files. The route exists while the panel is open,
// so that the attribution of the service is always shown with it.
type navigator struct {
	open bool
	// btn opens and closes the panel, from the toolbar.
	btn iconButton
	// closeBtn closes the panel, swapBtn swaps start and target, profileBtns select the profile.
	closeBtn, swapBtn iconButton
	profileBtns       []iconButton
	// profile is the index of the selected profile in [routing.Profiles].
	profile int
	// report opens the page for reporting map errors.
	report widget.Clickable
	// panel is the event tag for the panel background.
	panel int

	// ends are the start and the target, each if has.
	ends [2]geo.LonLat
	has  [2]bool
	// pressing is set from a press on the map while picking an end, until the pointer moves too far for a click.
	// pressAt is where the press was, also for dragging an end.
	pressing bool
	pressAt  f32.Point
	// dragging is set while the end dragEnd is dragged, which was at dragFrom.
	// dragMoved is set once the pointer moved farther than for a click.
	dragging  bool
	dragEnd   int
	dragFrom  geo.LonLat
	dragMoved bool

	// seq numbers the routing requests, so that results of an outdated request are dropped.
	seq    int
	cancel context.CancelFunc
	busy   bool
	err    error
	route  *routing.Route
	lines  mapview.Lines
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
	return n.open && (!n.has[0] || !n.has[1])
}

// updateNav handles the panel, clicks on the map while picking the start or the target,
// and dragging them. It takes the primary button events on the map, and returns those it didn't use,
// for the editor.
func (a *App) updateNav(gtx layout.Context, events []mapview.PointerEvent) []mapview.PointerEvent {
	n := &a.nav
	if n.btn.Clicked(gtx) {
		a.toggleNav()
	}
	if n.closeBtn.Clicked(gtx) {
		a.closeNav()
	}
	for i := range n.profileBtns {
		if n.profileBtns[i].Clicked(gtx) && i != n.profile {
			n.profile = i
			a.routingCfg.Profile = routing.Profiles[i]
			a.startRoute()
		}
	}
	if n.swapBtn.Clicked(gtx) {
		n.ends[0], n.ends[1] = n.ends[1], n.ends[0]
		n.has[0], n.has[1] = n.has[1], n.has[0]
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
// Pressing on the marker of the start or the target drags it, and the route is found again when released.
// While picking, a click without moving sets the start, or the target if the start is set.
func (a *App) navPointer(gtx layout.Context, ev mapview.PointerEvent) bool {
	n := &a.nav
	if !n.open {
		return false
	}
	if n.dragging {
		i := n.dragEnd
		switch ev.Kind {
		case pointer.Drag:
			n.dragMoved = n.dragMoved || dist2(ev.Screen, n.pressAt) > dragSlop*dragSlop
			if n.dragMoved {
				n.ends[i] = geo.ToLonLat(ev.Pos)
			}
		case pointer.Release:
			n.dragging = false
			if n.dragMoved {
				n.ends[i] = geo.ToLonLat(ev.Pos)
				a.startRoute()
			}
		case pointer.Cancel:
			n.dragging, n.ends[i] = false, n.dragFrom
		}
		return true
	}
	if ev.Kind == pointer.Press {
		if i, ok := a.navHit(gtx, ev.Screen); ok {
			n.dragging, n.dragEnd, n.dragFrom, n.dragMoved = true, i, n.ends[i], false
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
			i := 0
			if n.has[0] {
				i = 1
			}
			n.ends[i], n.has[i] = geo.ToLonLat(ev.Pos), true
			a.startRoute()
		}
		n.pressing = false
	case pointer.Cancel:
		n.pressing = false
	}
	return true
}

// navHit returns the end whose marker is at the screen position, the target first, as it is drawn on top.
func (a *App) navHit(gtx layout.Context, p f32.Point) (int, bool) {
	n := &a.nav
	r := float32(gtx.Dp(a.style.NavMarkerSize))/2 + float32(gtx.Dp(2)) + dragSlop
	for i := 1; i >= 0; i-- {
		if !n.has[i] {
			continue
		}
		if dist2(screenPos(&a.mapView.View, n.ends[i]), p) <= r*r {
			return i, true
		}
	}
	return 0, false
}

// navCursor returns the pointer cursor for the route on the map: for dragging an end,
// over an end, or for picking one. It returns false if the route doesn't set the cursor.
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
	}
	if n.picking() {
		return pointer.CursorCrosshair, true
	}
	return pointer.CursorDefault, false
}

// toggleNav opens or closes the panel.
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
}

// closeNav closes the panel, and removes the route and its ends.
func (a *App) closeNav() {
	a.stopRoute()
	n := &a.nav
	n.open, n.has, n.pressing, n.dragging = false, [2]bool{}, false, false
	n.err = nil
	a.setNavRoute(nil)
}

// startRoute finds the route between start and target in the background, if both are set.
// The previous route stays shown until the new one arrives.
func (a *App) startRoute() {
	a.stopRoute()
	n := &a.nav
	n.err = nil
	if !n.has[0] || !n.has[1] || a.router == nil {
		a.setNavRoute(nil)
		return
	}
	n.seq++
	seq := n.seq
	ctx, cancel := context.WithCancel(context.Background())
	n.cancel, n.busy = cancel, true
	req := routing.Request{Points: slices.Clone(n.ends[:]), Profile: routing.Profiles[n.profile]}
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
		n.lines.Set(nil, nil)
		return
	}
	pts := make([]geo.Point, len(r.Points))
	for i, p := range r.Points {
		pts[i] = geo.ToMercator(p)
	}
	line := []mapview.Polyline{mapview.NewPolyline(pts)}
	st := a.style
	n.lines.Set([]mapview.LineGroup{
		{Color: st.NavCasing, Width: st.NavLineWidth + 2*st.NavCasingWidth, Lines: line},
		{Color: st.NavLine, Width: st.NavLineWidth, Lines: line},
	}, nil)
}

// navStatus returns the text about the route for the panel: a hint on what to do,
// the state of the request, or the length and duration of the route.
func (a *App) navStatus() string {
	n := &a.nav
	switch {
	case !n.has[0]:
		return "Click the map to set the start"
	case !n.has[1]:
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

// layoutNavMap draws the route and its ends on the map.
func (a *App) layoutNavMap(gtx layout.Context) {
	n := &a.nav
	if !n.open {
		return
	}
	n.lines.Layout(gtx, &a.mapView.View)
	for i, label := range []string{"A", "B"} {
		if !n.has[i] {
			continue
		}
		col := a.style.NavStart
		if i == 1 {
			col = a.style.NavTarget
		}
		a.drawNavEnd(gtx, n.ends[i], label, col)
	}
}

// drawNavEnd draws the marker of the start or the target: a dot with a letter.
func (a *App) drawNavEnd(gtx layout.Context, ll geo.LonLat, label string, col color.NRGBA) {
	st := a.style
	x, y := a.mapView.View.ToScreen(geo.ToMercator(ll))
	c := f32.Pt(float32(x), float32(y))
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

// layoutNavContent draws the content of the panel: the profiles and buttons, the ends, the route and the attribution.
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

	header := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, 2*len(n.profileBtns)+4)
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
				return n.swapBtn.Layout(gtx, st, iconSwap, "Swap start and target", n.has[0] || n.has[1], false)
			}),
			layout.Rigid(layout.Spacer{Width: 1}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return n.closeBtn.Layout(gtx, st, iconClose, "Close and remove the route (Esc)", true, false)
			}),
		)
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
	})

	end := func(i int, label, unset string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			txt, col := unset, st.HintFg
			if n.has[i] {
				txt, col = n.ends[i].String(), st.Theme.Fg
			}
			l := st.SmallLabel(label + "  " + txt)
			l.Color = col
			l.MaxLines = 1
			return l.Layout(gtx)
		})
	}

	statusCol := st.Theme.Fg
	if n.err != nil {
		statusCol = st.EditActive
	}
	children := []layout.FlexChild{
		header, gap,
		end(0, "A", "Start"),
		end(1, "B", "Target"),
		gap,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := st.Label(a.navStatus())
			l.Color = statusCol
			if n.route != nil && n.err == nil && !n.busy {
				l.Font.Weight = font.Bold
			}
			return l.Layout(gtx)
		}),
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

package ui

import (
	"context"
	"fmt"
	"image"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/geocode"
)

const (
	// placeZoom is the zoom level for a found position without extent, like typed coordinates.
	// The map keeps a higher zoom level.
	placeZoom = 15
	// placeMaxZoom is the highest zoom level for showing the extent of a found place, like a building.
	placeMaxZoom = 17
	// typeDelay is how long typing must pause before searching while typing,
	// typeMinLen the shortest text searched while typing.
	typeDelay  = 500 * time.Millisecond
	typeMinLen = 3
	// searchAttributionHeight is the room kept below the result list for the attribution, which may take two lines.
	searchAttributionHeight unit.Dp = 40
	// searchBottomMargin is the space kept between the result panel and the bottom of the window.
	searchBottomMargin unit.Dp = 24
)

// coordLike matches text that may become coordinates, which is not searched while typing.
var coordLike = regexp.MustCompile(`^[\d\s.,;:/+\-°º'′’"″”NSEWnsew]*$`)

// searchBox is a search field for places and coordinates. Enter takes typed coordinates,
// or searches places by name and lists them in a panel below the field.
// With a service that allows it, places are also searched while typing.
// It is used for the search field in the toolbar, where the found position is marked on the map
// until the field is cleared, and for the points of a route.
type searchBox struct {
	ed       widget.Editor
	clearBtn iconButton
	// open shows the panel with the state of the search and the results.
	open bool
	// panel is the event tag for the panel background.
	panel int
	// due is when to search the typed text, or zero.
	due time.Time
	// seq numbers the searches, so that results of an outdated search are dropped.
	seq    int
	cancel context.CancelFunc
	busy   bool
	err    error
	places []geocode.Place
	rows   []widget.Clickable
	// list scrolls the places, if more than fit.
	list widget.List
	// sel is the place chosen by Enter, selected with the arrow keys.
	sel int
	// marker is the found position, if hasMarker. Only used by the search field in the toolbar.
	marker    geo.LonLat
	hasMarker bool
}

// searchAction is what happened in a search field.
type searchAction int

const (
	searchNone searchAction = iota
	// searchPicked is a chosen place, or typed coordinates.
	searchPicked
	// searchCleared is a cleared field.
	searchCleared
)

// updateSearch processes input on the search field in the toolbar and its result list.
func (a *App) updateSearch(gtx layout.Context) {
	switch p, act := a.search.update(gtx, a); act {
	case searchPicked:
		a.gotoPlace(p)
	case searchCleared:
		a.search.hasMarker = false
	}
}

// submitSearch jumps to typed coordinates, or searches places by name, in the toolbar.
func (a *App) submitSearch(gtx layout.Context) {
	if p, ok := a.search.submit(gtx, a); ok {
		a.gotoPlace(p)
	}
}

// clearSearch removes the results and the marker of the search in the toolbar.
func (a *App) clearSearch() {
	a.search.clear()
	a.search.hasMarker = false
}

// moveSearchSel moves the selection in the result list of the search in the toolbar.
func (a *App) moveSearchSel(d int) {
	a.search.moveSel(d)
}

// update processes input on the search field and the result list, and returns a chosen place.
func (s *searchBox) update(gtx layout.Context, a *App) (geocode.Place, searchAction) {
	s.ed.SingleLine, s.ed.Submit = true, true
	// The arrow keys and Enter choose from the list, before the field takes them.
	if s.open && len(s.places) > 0 {
		for {
			ev, ok := gtx.Event(
				key.Filter{Focus: &s.ed, Name: key.NameUpArrow},
				key.Filter{Focus: &s.ed, Name: key.NameDownArrow},
				key.Filter{Focus: &s.ed, Name: key.NameReturn},
				key.Filter{Focus: &s.ed, Name: key.NameEnter},
			)
			if !ok {
				break
			}
			e, ok := ev.(key.Event)
			if !ok || e.State != key.Press {
				continue
			}
			switch e.Name {
			case key.NameUpArrow:
				s.moveSel(-1)
			case key.NameDownArrow:
				s.moveSel(1)
			default:
				// While typed text is not searched yet, Enter searches it.
				if s.busy || !s.due.IsZero() {
					if p, ok := s.submit(gtx, a); ok {
						return p, searchPicked
					}
				} else {
					gtx.Execute(key.FocusCmd{})
					return s.pick(s.places[s.sel])
				}
			}
		}
	}
	for {
		ev, ok := s.ed.Update(gtx)
		if !ok {
			break
		}
		switch ev.(type) {
		case widget.SubmitEvent:
			if p, ok := s.submit(gtx, a); ok {
				return p, searchPicked
			}
		case widget.ChangeEvent:
			if s.typed(gtx, a) {
				return geocode.Place{}, searchCleared
			}
		}
	}
	for {
		ev, ok := gtx.Event(key.Filter{Focus: &s.ed, Name: key.NameEscape})
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			// Esc closes the result list, and then clears the field.
			if s.open {
				s.open = false
			} else {
				s.ed.SetText("")
				s.clear()
				gtx.Execute(key.FocusCmd{})
				return geocode.Place{}, searchCleared
			}
		}
	}
	if s.clearBtn.Clicked(gtx) {
		s.ed.SetText("")
		s.clear()
		return geocode.Place{}, searchCleared
	}
	// Clicking outside of the panel closes it.
	for {
		_, ok := gtx.Event(pointer.Filter{Target: s, Kinds: pointer.Press})
		if !ok {
			break
		}
		s.open = false
	}
	for i := range s.rows {
		if s.rows[i].Clicked(gtx) && i < len(s.places) {
			gtx.Execute(key.FocusCmd{})
			return s.pick(s.places[i])
		}
	}
	if !s.due.IsZero() && !gtx.Now.Before(s.due) {
		s.due = time.Time{}
		s.start(a, strings.TrimSpace(s.ed.Text()), true)
	}
	return geocode.Place{}, searchNone
}

// pick closes the result list, and returns the chosen place.
func (s *searchBox) pick(p geocode.Place) (geocode.Place, searchAction) {
	s.open = false
	s.stop()
	return p, searchPicked
}

// focusSearch moves the focus to the search field in the toolbar and selects its text.
func (a *App) focusSearch(gtx layout.Context) {
	gtx.Execute(key.FocusCmd{Tag: &a.search.ed})
	a.search.ed.SetCaret(a.search.ed.Len(), 0)
}

// searchFocused reports whether a search field has the focus: in the toolbar, or for a point of the route.
func (a *App) searchFocused(gtx layout.Context) bool {
	if gtx.Focused(&a.search.ed) {
		return true
	}
	for _, st := range a.nav.stops {
		if gtx.Focused(&st.field.ed) {
			return true
		}
	}
	return false
}

// moveSel moves the selection in the result list by d places, and scrolls it into view.
func (s *searchBox) moveSel(d int) {
	s.sel = max(0, min(len(s.places)-1, s.sel+d))
	pos := &s.list.Position
	switch {
	case s.sel <= pos.First:
		pos.First, pos.Offset = s.sel, 0
	case pos.Count > 0 && s.sel >= pos.First+pos.Count-1:
		// The last visible place may be cut off, so scroll until the one after it would be the last.
		pos.First, pos.Offset = s.sel-pos.Count+2, 0
	}
}

// searchWhileTyping reports whether text is searched while typing:
// if the service allows it, and the text is long enough and can't become coordinates.
func (a *App) searchWhileTyping(text string) bool {
	return a.geocoder != nil && a.geocoder.Interactive() &&
		utf8.RuneCountInString(text) >= typeMinLen && !coordLike.MatchString(text)
}

// typed handles a change of the text: it schedules searching it, if searched while typing,
// and otherwise drops the results, which no longer match. It reports whether the field was emptied.
func (s *searchBox) typed(gtx layout.Context, a *App) bool {
	text := strings.TrimSpace(s.ed.Text())
	if text == "" {
		s.clear()
		return true
	}
	if !a.searchWhileTyping(text) {
		s.stop()
		s.open, s.err, s.places = false, nil, nil
		return false
	}
	s.due = gtx.Now.Add(typeDelay)
	gtx.Execute(op.InvalidateCmd{At: s.due})
	return false
}

// submit returns typed coordinates, or searches places by name.
func (s *searchBox) submit(gtx layout.Context, a *App) (geocode.Place, bool) {
	q := strings.TrimSpace(s.ed.Text())
	if q == "" {
		return geocode.Place{}, false
	}
	if ll, ok := geocode.ParseCoords(q); ok {
		s.stop()
		s.open = false
		gtx.Execute(key.FocusCmd{})
		return geocode.Place{Pos: ll}, true
	}
	s.start(a, q, false)
	return geocode.Place{}, false
}

// start searches places by name in the background, preferring places in about the area of the map.
// keep keeps showing the previous results until the new ones arrive, while typing.
func (s *searchBox) start(a *App, text string, keep bool) {
	s.stop()
	if a.geocoder == nil || text == "" {
		return
	}
	s.seq++
	seq := s.seq
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.open, s.busy, s.err = true, true, nil
	if !keep {
		s.places = nil
	}
	v := &a.mapView.View
	q := geocode.Query{Text: text, Limit: a.searchCfg.SearchLimit(), Near: geo.ToLonLat(v.Center), HasNear: true, Zoom: int(math.Round(v.Zoom))}
	go func() {
		places, err := a.geocoder.Search(ctx, q)
		a.runOnUI(func() {
			if seq != s.seq {
				return
			}
			cancel()
			s.cancel = nil
			s.busy, s.err, s.places, s.sel = false, err, places, 0
			s.list.Position = layout.Position{}
			if len(s.rows) < len(places) {
				s.rows = make([]widget.Clickable, len(places))
			}
		})
	}()
}

// stop cancels a running or scheduled search and drops its results.
func (s *searchBox) stop() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.seq++
	s.busy, s.due = false, time.Time{}
}

// clear removes the results.
func (s *searchBox) clear() {
	s.stop()
	s.open, s.err, s.places = false, nil, nil
}

// gotoPlace shows a found place on the map and marks its position.
func (a *App) gotoPlace(p geocode.Place) {
	s := &a.search
	s.open = false
	s.marker, s.hasMarker = p.Pos, true
	s.stop()
	a.showPlace(p)
}

// showPlace shows a place on the map.
// A place with an extent is shown fully, others at [placeZoom] or closer.
func (a *App) showPlace(p geocode.Place) {
	a.gps.following = false
	m := a.mapView
	if p.HasBounds && (p.Min.Lon < p.Max.Lon || p.Min.Lat < p.Max.Lat) {
		m.Fit(geo.EmptyRect().Extend(geo.ToMercator(p.Min)).Extend(geo.ToMercator(p.Max)))
		if m.View.Size.X > 0 && m.View.Zoom > placeMaxZoom {
			m.SetZoom(placeMaxZoom)
		}
		return
	}
	m.View.Center = geo.ToMercator(p.Pos)
	if m.View.Zoom < placeZoom {
		m.SetZoom(placeZoom)
	}
}

// layoutSearch draws the search field in the toolbar, and the result panel below it if open.
// The toolbar gets the height of the window, which limits the height of the result panel.
func (a *App) layoutSearch(gtx layout.Context) layout.Dimensions {
	st := a.style
	gtx.Constraints.Min.X = min(gtx.Dp(st.SearchWidth), gtx.Constraints.Max.X)
	gtx.Constraints.Max.X = gtx.Constraints.Min.X
	// As high as the toolbar buttons.
	gtx.Constraints.Min.Y = gtx.Dp(st.ToolIconSize + 2*st.ToolIconPad)
	return a.search.layout(gtx, a, "Place or coordinates (Ctrl+F)", "Clear the search and its marker on the map", true)
}

// layout draws the search field with the hint while empty, as wide as the constraints, and the result panel below it if open.
// The panel is as high as the remaining height of the constraints allows. icon shows a search icon in the field.
// clearTip is the tooltip of the button for clearing the field.
func (s *searchBox) layout(gtx layout.Context, a *App, hint, clearTip string, icon bool) layout.Dimensions {
	st := a.style
	availY := gtx.Constraints.Max.Y
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	dims := layoutFieldBox(gtx, st, gtx.Focused(&s.ed), func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !icon {
					return layout.Dimensions{}
				}
				size := gtx.Sp(st.TextSize) * 5 / 4
				layoutIcon(gtx, iconSearch, image.Point{}, size, st.HintFg)
				return layout.Dimensions{Size: image.Pt(size+gtx.Dp(st.Spacing/2), size)}
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				ed := material.Editor(st.Theme, &s.ed, hint)
				ed.TextSize = st.TextSize
				ed.HintColor = st.HintFg
				return ed.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if s.ed.Len() == 0 {
					return layout.Dimensions{}
				}
				return s.layoutClear(gtx, st, clearTip)
			}),
		)
	})
	if !s.open {
		return dims
	}

	macro := op.Record(gtx.Ops)
	// Catch clicks outside the panel.
	scrim := clip.Rect{Min: image.Pt(-1e6, -1e6), Max: image.Pt(1e6, 1e6)}.Push(gtx.Ops)
	event.Op(gtx.Ops, s)
	scrim.Pop()
	op.Offset(image.Pt(0, dims.Size.Y)).Add(gtx.Ops)
	gtx.Constraints = layout.Exact(image.Pt(dims.Size.X, 0))
	// Keep the status bar visible below the panel.
	gtx.Constraints.Max.Y = max(0, min(gtx.Dp(600), availY-dims.Size.Y-gtx.Dp(searchBottomMargin)))
	layoutPanel(gtx, st, &s.panel, func(gtx layout.Context) layout.Dimensions {
		return s.layoutResults(gtx, a)
	})
	op.Defer(gtx.Ops, macro.Stop())
	return dims
}

// layoutClear draws the button for clearing the field, as high as the text so that the field keeps its height.
func (s *searchBox) layoutClear(gtx layout.Context, st *Style, tip string) layout.Dimensions {
	b := &s.clearBtn
	avail := gtx.Constraints.Max.X
	dims := b.btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Sp(st.TextSize) * 5 / 4
		col := st.HintFg
		if b.btn.Hovered() {
			col = st.Theme.Fg
		}
		layoutIcon(gtx, iconClose, image.Point{}, size, col)
		pointer.CursorPointer.Add(gtx.Ops)
		return layout.Dimensions{Size: image.Pt(size, size)}
	})
	b.tip.Layout(gtx, st, b.btn.Hovered(), dims.Size, avail, tip)
	return dims
}

// layoutResults draws the state of the search, or the found places.
func (s *searchBox) layoutResults(gtx layout.Context, a *App) layout.Dimensions {
	st := a.style
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	hint := func(txt string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(st.Spacing/2).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := st.SmallLabel(txt)
				l.Color = st.HintFg
				return l.Layout(gtx)
			})
		})
	}
	var children []layout.FlexChild
	switch {
	case s.busy && len(s.places) == 0:
		children = append(children, hint("Searching…"))
	case s.err != nil:
		children = append(children, hint("Search failed: "+s.err.Error()))
	case !s.busy && len(s.places) == 0:
		children = append(children, hint("No places found"))
	}
	if len(s.places) > 0 {
		center := geo.ToLonLat(a.mapView.View.Center)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// Leave room for the attribution below.
			gtx.Constraints.Max.Y = max(0, gtx.Constraints.Max.Y-gtx.Dp(searchAttributionHeight))
			s.list.Axis = layout.Vertical
			return material.List(st.Theme, &s.list).Layout(gtx, len(s.places), func(gtx layout.Context, i int) layout.Dimensions {
				return a.layoutPlace(gtx, &s.rows[i], s.places[i], i == s.sel, center)
			})
		}))
	}
	if !s.busy && s.err == nil && a.geocoder != nil {
		children = append(children, hint(a.geocoder.Attribution()))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// layoutPlace draws a found place as a clickable row: its name, and below its kind,
// and right-aligned the beeline distance from the map center. The selected place is highlighted.
func (a *App) layoutPlace(gtx layout.Context, btn *widget.Clickable, p geocode.Place, selected bool, center geo.LonLat) layout.Dimensions {
	st := a.style
	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		macro := op.Record(gtx.Ops)
		dims := layout.UniformInset(st.Spacing/2).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := st.Label(p.Name)
					l.MaxLines = 2
					return l.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							l := st.SmallLabel(strings.ReplaceAll(p.Kind, "_", " "))
							l.Color = st.HintFg
							l.Font.Style = font.Italic
							l.MaxLines = 1
							return l.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							l := st.SmallLabel(formatPlaceDistance(geo.Distance(center, p.Pos)))
							l.Color = st.HintFg
							return l.Layout(gtx)
						}),
					)
				}),
			)
		})
		call := macro.Stop()
		switch {
		case selected:
			paint.FillShape(gtx.Ops, st.SelectedBg, clip.Rect{Max: dims.Size}.Op())
		case btn.Hovered():
			paint.FillShape(gtx.Ops, st.DragBg, clip.Rect{Max: dims.Size}.Op())
		}
		call.Add(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		return dims
	})
}

// formatPlaceDistance formats the distance to a found place, in whole kilometers from 10 km on.
func formatPlaceDistance(m float64) string {
	if m >= 10_000 {
		return fmt.Sprintf("%.0f km", m/1000)
	}
	return formatDistance(m)
}

// layoutSearchMarker draws the marker of the found position on the map.
func (a *App) layoutSearchMarker(gtx layout.Context) {
	s := &a.search
	if !s.hasMarker {
		return
	}
	st := a.style
	x, y := a.mapView.View.ToScreen(geo.ToMercator(s.marker))
	c := f32.Pt(float32(x), float32(y))
	r := float32(gtx.Dp(st.SearchMarkerSize)) / 2
	ring := float32(max(1, gtx.Dp(2)))
	paint.FillShape(gtx.Ops, st.Theme.Fg, circle(c, r+ring).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, st.SearchMarker, circle(c, r).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, st.Theme.Fg, circle(c, r/3).Op(gtx.Ops))
}

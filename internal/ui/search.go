package ui

import (
	"context"
	"image"
	"strings"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/geocode"
)

const (
	// searchLimit is the maximum number of places shown for a search.
	searchLimit = 8
	// placeZoom is the zoom level for a found position without extent, like typed coordinates.
	// The map keeps a higher zoom level.
	placeZoom = 15
	// placeMaxZoom is the highest zoom level for showing the extent of a found place, like a building.
	placeMaxZoom = 17
)

// placeSearcher finds places by name.
type placeSearcher interface {
	Search(ctx context.Context, query string, limit int) ([]geocode.Place, error)
}

// searchBox is the search field in the toolbar. Enter jumps to typed coordinates,
// or searches places by name and lists them in a panel below the field.
// The found position is marked on the map until the field is cleared.
type searchBox struct {
	ed    widget.Editor
	clear iconButton
	// open shows the panel with the state of the search and the results.
	open bool
	// panel is the event tag for the panel background.
	panel int
	// seq numbers the searches, so that results of an outdated search are dropped.
	seq    int
	cancel context.CancelFunc
	busy   bool
	err    error
	places []geocode.Place
	rows   []widget.Clickable
	// marker is the found position, if hasMarker.
	marker    geo.LonLat
	hasMarker bool
}

// updateSearch processes input on the search field and the result list.
func (a *App) updateSearch(gtx layout.Context) {
	s := &a.search
	s.ed.SingleLine, s.ed.Submit = true, true
	for {
		ev, ok := s.ed.Update(gtx)
		if !ok {
			break
		}
		switch ev.(type) {
		case widget.SubmitEvent:
			a.submitSearch(gtx)
		case widget.ChangeEvent:
			if strings.TrimSpace(s.ed.Text()) == "" {
				a.clearSearch()
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
				a.clearSearch()
				gtx.Execute(key.FocusCmd{})
			}
		}
	}
	if s.clear.Clicked(gtx) {
		s.ed.SetText("")
		a.clearSearch()
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
			a.gotoPlace(s.places[i])
			gtx.Execute(key.FocusCmd{})
		}
	}
}

// focusSearch moves the focus to the search field and selects its text.
func (a *App) focusSearch(gtx layout.Context) {
	gtx.Execute(key.FocusCmd{Tag: &a.search.ed})
	a.search.ed.SetCaret(a.search.ed.Len(), 0)
}

// searchFocused reports whether the search field has the focus.
func (a *App) searchFocused(gtx layout.Context) bool {
	return gtx.Focused(&a.search.ed)
}

// submitSearch jumps to typed coordinates, or starts searching places by name in the background.
func (a *App) submitSearch(gtx layout.Context) {
	s := &a.search
	q := strings.TrimSpace(s.ed.Text())
	if q == "" {
		return
	}
	a.stopSearch()
	if ll, ok := geocode.ParseCoords(q); ok {
		s.open = false
		a.gotoPlace(geocode.Place{Pos: ll})
		gtx.Execute(key.FocusCmd{})
		return
	}
	if a.geocoder == nil {
		return
	}
	s.seq++
	seq := s.seq
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.open, s.busy, s.err, s.places = true, true, nil, nil
	go func() {
		places, err := a.geocoder.Search(ctx, q, searchLimit)
		a.runOnUI(func() {
			if seq != s.seq {
				return
			}
			cancel()
			s.cancel = nil
			s.busy, s.err, s.places = false, err, places
			if len(s.rows) < len(places) {
				s.rows = make([]widget.Clickable, len(places))
			}
		})
	}()
}

// stopSearch cancels a running search and drops its results.
func (a *App) stopSearch() {
	s := &a.search
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.seq++
	s.busy = false
}

// clearSearch removes the results and the marker.
func (a *App) clearSearch() {
	a.stopSearch()
	s := &a.search
	s.open, s.err, s.places, s.hasMarker = false, nil, nil, false
}

// gotoPlace shows a found place on the map and marks its position.
// A place with an extent is shown fully, others at [placeZoom] or closer.
func (a *App) gotoPlace(p geocode.Place) {
	s := &a.search
	s.open = false
	s.marker, s.hasMarker = p.Pos, true
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

// layoutSearch draws the search field, and the result panel below it if open.
func (a *App) layoutSearch(gtx layout.Context) layout.Dimensions {
	st, s := a.style, &a.search
	gtx.Constraints.Min.X = min(gtx.Dp(st.SearchWidth), gtx.Constraints.Max.X)
	gtx.Constraints.Max.X = gtx.Constraints.Min.X
	dims := layoutFieldBox(gtx, st, a.searchFocused(gtx), func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := gtx.Sp(st.TextSize) * 5 / 4
				layoutIcon(gtx, iconSearch, image.Point{}, size, st.HintFg)
				return layout.Dimensions{Size: image.Pt(size+gtx.Dp(st.Spacing/2), size)}
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				ed := material.Editor(st.Theme, &s.ed, "Place or coordinates (Ctrl+F)")
				ed.TextSize = st.TextSize
				ed.HintColor = st.HintFg
				return ed.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if s.ed.Len() == 0 {
					return layout.Dimensions{}
				}
				return a.layoutSearchClear(gtx)
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
	gtx.Constraints.Max.Y = gtx.Dp(600)
	layoutPanel(gtx, st, &s.panel, func(gtx layout.Context) layout.Dimensions {
		return a.layoutSearchResults(gtx)
	})
	op.Defer(gtx.Ops, macro.Stop())
	return dims
}

// layoutSearchClear draws the button for clearing the search, as high as the text so that the field keeps its height.
func (a *App) layoutSearchClear(gtx layout.Context) layout.Dimensions {
	st, b := a.style, &a.search.clear
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
	b.tip.Layout(gtx, st, b.btn.Hovered(), dims.Size, avail, "Clear the search and its marker on the map")
	return dims
}

// layoutSearchResults draws the state of the search, or the found places.
func (a *App) layoutSearchResults(gtx layout.Context) layout.Dimensions {
	st, s := a.style, &a.search
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
	case s.busy:
		children = append(children, hint("Searching…"))
	case s.err != nil:
		children = append(children, hint("Search failed: "+s.err.Error()))
	case len(s.places) == 0:
		children = append(children, hint("No places found"))
	}
	for i, p := range s.places {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.layoutPlace(gtx, &s.rows[i], p)
		}))
	}
	if !s.busy && s.err == nil {
		children = append(children, hint(geocode.NominatimAttribution))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// layoutPlace draws a found place as a clickable row: its name, and its kind below.
func (a *App) layoutPlace(gtx layout.Context, btn *widget.Clickable, p geocode.Place) layout.Dimensions {
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
					l := st.SmallLabel(strings.ReplaceAll(p.Kind, "_", " "))
					l.Color = st.HintFg
					l.Font.Style = font.Italic
					return l.Layout(gtx)
				}),
			)
		})
		call := macro.Stop()
		if btn.Hovered() {
			paint.FillShape(gtx.Ops, st.DragBg, clip.Rect{Max: dims.Size}.Op())
		}
		call.Add(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		return dims
	})
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

package ui

import (
	"image"
	"strings"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/mlange-42/tramp/internal/geocode"
	"github.com/mlange-42/tramp/internal/routing"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

// Pages of the routing and search services, linked in the list of data sources.
const (
	valhallaPage = "https://valhalla1.openstreetmap.de"
	osrmPage     = "https://routing.openstreetmap.de/about.html"
	photonPage   = "https://photon.komoot.io"
	nominatimURL = "https://nominatim.org"
)

var iconInfo = mustIcon(icons.ActionInfoOutline)

// sourcesWidth is the largest width of the list of data sources.
const sourcesWidth unit.Dp = 420

// sourcesView is the link to the list of data sources in the status bar, and the list.
type sourcesView struct {
	btn  widget.Clickable
	open bool
	// credits are the listed sources at the last layout, with a link for each, and panel the event tag
	// of the list's background. The view itself is the tag of the area around it, closing the list.
	credits []credit
	links   []widget.Clickable
	panel   int
}

// allCredits returns the credits of all data sources: those shown on the map, and the routing and search services.
func (a *App) allCredits() []credit {
	cs := a.mapCredits()
	if a.router != nil {
		c := credit{use: "Routing", full: a.router.Attribution(), url: serviceURL(a.routingCfg.URL, osmCopyrightURL)}
		switch strings.ToLower(strings.TrimSpace(a.routingCfg.Service)) {
		case routing.ServiceOSRM:
			c.name, c.url = "OSRM", serviceURL(a.routingCfg.URL, osrmPage)
		default:
			c.name, c.url = "Valhalla", serviceURL(a.routingCfg.URL, valhallaPage)
		}
		cs = append(cs, c)
	}
	if a.geocoder != nil {
		c := credit{use: "Search", full: a.geocoder.Attribution()}
		switch strings.ToLower(strings.TrimSpace(a.searchCfg.Service)) {
		case geocode.ServiceNominatim:
			c.name, c.url = "Nominatim", serviceURL(a.searchCfg.URL, nominatimURL)
		default:
			c.name, c.url = "Photon", serviceURL(a.searchCfg.URL, photonPage)
		}
		cs = append(cs, c)
	}
	return cs
}

// serviceURL returns the page of a public service, or the OpenStreetMap copyright page for another server,
// as the services' data comes from OpenStreetMap.
func serviceURL(configured, page string) string {
	if strings.TrimSpace(configured) != "" {
		return osmCopyrightURL
	}
	return page
}

// updateSources handles the link to the list of data sources, clicks outside the list, and the links in it.
func (a *App) updateSources(gtx layout.Context) {
	s := &a.sources
	if s.btn.Clicked(gtx) {
		s.open = !s.open
	}
	for {
		_, ok := gtx.Event(pointer.Filter{Target: s, Kinds: pointer.Press})
		if !ok {
			break
		}
		s.open = false
	}
	// The panel takes presses, so that they don't reach the map below.
	for {
		if _, ok := gtx.Event(pointer.Filter{Target: &s.panel, Kinds: pointer.Press}); !ok {
			break
		}
	}
	for i := range s.links {
		if s.links[i].Clicked(gtx) && i < len(s.credits) && s.credits[i].url != "" {
			openURL(s.credits[i].url)
		}
	}
}

// layoutSourcesLink draws the link to the list of data sources, with an info icon in front,
// separated from the short credits before it.
func (a *App) layoutSourcesLink(gtx layout.Context) layout.Dimensions {
	st, s := a.style, &a.sources
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := st.SmallLabel("|")
			l.Color = st.HintFg
			return layout.Inset{Left: st.Spacing, Right: st.Spacing}.Layout(gtx, l.Layout)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				col := st.Theme.ContrastBg
				if s.btn.Hovered() || s.open {
					col = st.Theme.Fg
				}
				pointer.CursorPointer.Add(gtx.Ops)
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						size := gtx.Sp(st.SmallTextSize)
						layoutIcon(gtx, iconInfo, image.Point{}, size, col)
						return layout.Dimensions{Size: image.Pt(size+gtx.Dp(st.Spacing/2), size)}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := st.SmallLabel("Data sources")
						l.Color = col
						l.MaxLines = 1
						return l.Layout(gtx)
					}),
				)
			})
		}),
	)
}

// layoutSources draws the list of data sources above the status bar at the right edge of the window, if open.
// bar is the size of the status bar, at the current origin. The list is drawn on top of everything.
func (a *App) layoutSources(gtx layout.Context, bar image.Point) {
	s := &a.sources
	if !s.open {
		return
	}
	st := a.style
	s.credits = a.allCredits()
	if len(s.links) != len(s.credits) {
		s.links = make([]widget.Clickable, len(s.credits))
	}

	macro := op.Record(gtx.Ops)
	// Catch clicks outside the list, including on the status bar.
	scrim := clip.Rect{Min: image.Pt(-1e6, -1e6), Max: image.Pt(1e6, 1e6)}.Push(gtx.Ops)
	event.Op(gtx.Ops, s)
	scrim.Pop()

	lgtx := gtx
	w := min(gtx.Dp(sourcesWidth), bar.X)
	lgtx.Constraints = layout.Constraints{Min: image.Pt(w, 0), Max: image.Pt(w, gtx.Dp(800))}
	inner := op.Record(gtx.Ops)
	dims := layoutPanel(lgtx, st, &s.panel, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(st.Spacing).Layout(gtx, a.layoutSourcesContent)
	})
	panel := inner.Stop()
	op.Offset(image.Pt(bar.X-dims.Size.X, -dims.Size.Y)).Add(gtx.Ops)
	panel.Add(gtx.Ops)
	op.Defer(gtx.Ops, macro.Stop())
}

// layoutSourcesContent draws the list of data sources: for each, what it is used for, its credit and its link.
func (a *App) layoutSourcesContent(gtx layout.Context) layout.Dimensions {
	st, s := a.style, &a.sources
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		l := st.Label("Data sources")
		l.Font.Weight = font.Bold
		return layout.Inset{Bottom: st.Spacing}.Layout(gtx, l.Layout)
	})}
	for i := range s.credits {
		c := &s.credits[i]
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: st.Spacing}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := st.SmallLabel(c.use + " · " + c.name)
						l.Font.Weight = font.Bold
						return l.Layout(gtx)
					}),
					layout.Rigid(st.SmallLabel(c.full).Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if c.url == "" {
							return layout.Dimensions{}
						}
						btn := &s.links[i]
						return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							l := st.SmallLabel(c.url)
							l.Color = st.Theme.ContrastBg
							if btn.Hovered() {
								l.Color = st.Theme.Fg
							}
							l.MaxLines = 1
							pointer.CursorPointer.Add(gtx.Ops)
							return l.Layout(gtx)
						})
					}),
				)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

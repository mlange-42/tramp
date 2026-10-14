package ui

import (
	"image"
	"log"
	"slices"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/mlange-42/tramp/internal/settings"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

// noticeWidth is the largest width of notices.
const noticeWidth unit.Dp = 420

var iconWarning = mustIcon(icons.AlertWarning)

// Notice is a message shown at the top of the map until it is closed, like a problem with a configuration file.
type Notice struct {
	Text string
	// LogLink adds a line saying that the details are in the log file,
	// with a link that opens the settings folder containing it.
	LogLink bool
}

// notice is a [Notice] with its widgets.
type notice struct {
	Notice
	close iconButton
	link  widget.Clickable
	// panel is the event tag of the notice, so that clicks on it don't go to the map.
	panel int
}

// notify shows a notice at the top of the map. It must be called on the UI goroutine.
func (a *App) notify(n Notice) {
	a.notices = append(a.notices, &notice{Notice: n})
	a.invalidate()
}

// updateNotices handles the links of the notices, and removes the closed ones.
func (a *App) updateNotices(gtx layout.Context) {
	for _, n := range a.notices {
		if n.link.Clicked(gtx) {
			if dir, err := settings.Dir(); err != nil {
				log.Printf("opening the settings folder: %v", err)
			} else {
				openFolder(dir)
			}
		}
	}
	a.notices = slices.DeleteFunc(a.notices, func(n *notice) bool { return n.close.Clicked(gtx) })
}

// layoutNotices draws the notices below each other at the top center of the map.
func (a *App) layoutNotices(gtx layout.Context) {
	if len(a.notices) == 0 {
		return
	}
	st := a.style
	margin := gtx.Dp(st.Spacing * 2)
	w := max(0, min(gtx.Dp(noticeWidth), gtx.Constraints.Max.X-2*margin))
	y := margin
	for _, n := range a.notices {
		ngtx := gtx
		ngtx.Constraints = layout.Constraints{Min: image.Pt(w, 0), Max: image.Pt(w, max(0, gtx.Constraints.Max.Y-y))}
		macro := op.Record(gtx.Ops)
		dims := layoutPanel(ngtx, st, &n.panel, func(gtx layout.Context) layout.Dimensions {
			return a.layoutNotice(gtx, n)
		})
		call := macro.Stop()
		off := op.Offset(image.Pt((gtx.Constraints.Max.X-dims.Size.X)/2, y)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		off.Pop()
		y += dims.Size.Y + gtx.Dp(st.Spacing)
	}
}

// layoutNotice draws a notice: a warning icon, the text and the close button.
func (a *App) layoutNotice(gtx layout.Context, n *notice) layout.Dimensions {
	st := a.style
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return layout.UniformInset(st.Spacing).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := gtx.Dp(st.IconSize * 1.5)
				gtx.Constraints = layout.Exact(image.Pt(size, size))
				return iconWarning.Layout(gtx, st.WarningFg)
			}),
			layout.Rigid(layout.Spacer{Width: st.Spacing}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				// The full width, so that the close buttons of all notices line up.
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				if !n.LogLink {
					return st.Label(n.Text).Layout(gtx)
				}
				return layoutFlowText(gtx, st,
					textSpan{text: n.Text + " Details are in "},
					textSpan{text: settings.LogFile, link: &n.link},
					textSpan{text: "."},
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return n.close.LayoutFlat(gtx, st, iconClose, st.HintFg, "Close")
			}),
		)
	})
}

package ui

import (
	"context"
	"image"
	"log"
	"time"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

// legendTimeout limits loading a legend image.
const legendTimeout = 30 * time.Second

// overlayLegend is the legend image of an overlay, as given in the capabilities of its service.
// It is loaded when first shown, and shown below the overlay's line in the legend box on the map.
type overlayLegend struct {
	// url is the address of the image, empty if there is none.
	url string
	// img is the loaded image, with its size in pixels. The size is zero until loaded.
	img  paint.ImageOp
	size image.Point
	// loading is set while the image is loaded, and failed if that failed.
	loading, failed bool
	// shown is set while the legend is shown. Only one legend is shown at a time.
	shown bool
	// btn toggles the legend.
	btn widget.Clickable
}

// setURL sets the address of the legend image, and drops the image if it changed.
func (l *overlayLegend) setURL(url string) {
	if url != l.url {
		l.url, l.img, l.size, l.loading, l.failed = url, paint.ImageOp{}, image.Point{}, false, false
	}
}

// available reports whether the legend can be shown.
func (l *overlayLegend) available() bool {
	return l.url != "" && !l.failed
}

// loadLegend loads the legend image of an overlay in the background.
func (a *App) loadLegend(i int) {
	l := &a.overlays[i].legend
	if l.loading || !l.available() || l.size != (image.Point{}) {
		return
	}
	l.loading = true
	url := l.url
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), legendTimeout)
		defer cancel()
		img, err := a.client.GetLegend(ctx, url)
		a.runOnUI(func() {
			l := &a.overlays[i].legend
			if l.url != url {
				return
			}
			l.loading = false
			if err != nil {
				log.Printf("legend of %s: %v", a.overlays[i].Layer.Name, err)
				l.failed = true
				return
			}
			l.img, l.size = paint.NewImageOp(img), img.Bounds().Size()
		})
	}()
}

// toggleLegend shows the legend of an overlay, or hides it if shown. Other legends are hidden.
func (a *App) toggleLegend(i int) {
	show := !a.overlays[i].legend.shown
	for j := range a.overlays {
		a.overlays[j].legend.shown = false
	}
	a.overlays[i].legend.shown = show
	if show {
		a.loadLegend(i)
	}
}

// overlayLegendRows returns the lines of the enabled overlays with a data time or a legend,
// for the legend box on the map. A click on a line with a legend toggles its image below it.
// maxHeight limits the height of the image, which is scaled down if needed.
func (a *App) overlayLegendRows(gtx layout.Context, maxHeight int) []layout.FlexChild {
	st := a.style
	var rows []layout.FlexChild
	if _, _, ok := a.timeBounds(); ok {
		rows = append(rows, layout.Rigid(a.layoutTimeControl))
	}
	for i := range a.overlays {
		o := &a.overlays[i]
		l := &o.legend
		if o.tiles == nil || (o.times.time.IsZero() && !o.times.outside && !l.available()) {
			continue
		}
		for l.btn.Clicked(gtx) {
			a.toggleLegend(i)
		}
		text := o.Layer.Name
		switch {
		case o.times.outside:
			text += "  no data"
		case !o.times.time.IsZero():
			text += "  " + formatDataTime(o.times.time, gtx.Now)
		}
		rows = append(rows,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !l.available() {
					return st.SmallLabel(text).Layout(gtx)
				}
				return l.btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					pointer.CursorPointer.Add(gtx.Ops)
					ic, col := iconExpand, st.Theme.Fg
					if l.shown {
						ic = iconCollapse
					}
					if l.btn.Hovered() {
						col = st.Theme.ContrastBg
					}
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := st.SmallLabel(text)
							lbl.Color = col
							return lbl.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							size := gtx.Sp(st.SmallTextSize)
							layoutIcon(gtx, ic, image.Pt(gtx.Dp(st.Spacing/2), 0), size, col)
							return layout.Dimensions{Size: image.Pt(size+gtx.Dp(st.Spacing/2), size)}
						}),
					)
				})
			}),
			layout.Rigid(layout.Spacer{Height: st.Spacing / 2}.Layout),
		)
		if l.shown && l.available() {
			rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutLegendImage(gtx, l, maxHeight)
			}))
		}
	}
	return rows
}

// layoutLegendImage draws a legend image with one pixel per dp, scaled down to maxHeight if needed,
// and a placeholder while it is loading.
func layoutLegendImage(gtx layout.Context, l *overlayLegend, maxHeight int) layout.Dimensions {
	if l.size == (image.Point{}) {
		return layout.Dimensions{Size: image.Pt(0, gtx.Dp(20))}
	}
	w, h := gtx.Dp(unit.Dp(l.size.X)), gtx.Dp(unit.Dp(l.size.Y))
	if maxHeight > 0 && h > maxHeight {
		w, h = w*maxHeight/h, maxHeight
	}
	gtx.Constraints = layout.Exact(image.Pt(w, h))
	dims := widget.Image{Src: l.img, Fit: widget.Contain, Position: layout.NW}.Layout(gtx)
	// Space below, before the next line.
	return layout.Dimensions{Size: image.Pt(dims.Size.X, dims.Size.Y+gtx.Dp(4))}
}

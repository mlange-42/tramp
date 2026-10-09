package ui

import (
	"image/color"
	"slices"

	"gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// trackColor is the color of tracks, routes and waypoints on the map.
var trackColor = color.NRGBA{R: 0xe0, G: 0x10, B: 0x10, A: 0xff}

// panelRow is a row in the side panel: a file, or an item of an expanded file.
type panelRow struct {
	file *openFile
	item *fileItem
}

// updateFiles processes input on the side panel.
func (a *App) updateFiles(gtx layout.Context) {
	for i := 0; i < len(a.files); i++ {
		f := a.files[i]
		if f.close.Clicked(gtx) {
			a.files = slices.Delete(a.files, i, i+1)
			a.tracksChanged = true
			i--
			continue
		}
		if f.visible.Update(gtx) {
			a.tracksChanged = true
		}
		if f.zoom.Clicked(gtx) && !f.bounds.Empty() {
			a.mapView.Fit(fitRect(f.bounds))
		}
		if f.expand.Clicked(gtx) {
			f.expanded = !f.expanded
		}
		for _, it := range f.items {
			if it.visible.Update(gtx) {
				a.tracksChanged = true
			}
			if it.zoom.Clicked(gtx) && !it.bounds.Empty() {
				a.mapView.Fit(fitRect(it.bounds))
			}
		}
	}
}

// layoutPanel draws the side panel with the opened files.
func (a *App) layoutPanel(gtx layout.Context) layout.Dimensions {
	st := a.style
	paint.Fill(gtx.Ops, st.SideBg)
	return st.SideInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if len(a.files) == 0 {
			l := st.SmallLabel("Nothing opened")
			l.Color = st.HintFg
			return l.Layout(gtx)
		}
		a.panelRows = a.panelRows[:0]
		for _, f := range a.files {
			a.panelRows = append(a.panelRows, panelRow{file: f})
			if f.expanded && f.expandable() {
				for _, it := range f.items {
					a.panelRows = append(a.panelRows, panelRow{file: f, item: it})
				}
			}
		}
		return material.List(st.Theme, &a.fileList).Layout(gtx, len(a.panelRows), func(gtx layout.Context, i int) layout.Dimensions {
			r := a.panelRows[i]
			return layout.Inset{Bottom: st.Spacing}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if r.item != nil {
					return a.layoutItemRow(gtx, r.file, r.item)
				}
				return a.layoutFileRow(gtx, r.file)
			})
		})
	})
}

func (a *App) layoutFileRow(gtx layout.Context, f *openFile) layout.Dimensions {
	st := a.style
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(a.checkBox(&f.visible)),
		layout.Flexed(1, a.rowText(&f.zoom, f.name, f.summary, f.visible.Value)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !f.expandable() {
				return layout.Dimensions{}
			}
			return material.Clickable(gtx, &f.expand, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(st.Spacing).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layoutArrow(gtx, float32(gtx.Sp(st.TextSize))*0.6, st.Theme.Fg, f.expanded)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &f.close, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: st.Spacing / 2, Right: st.Spacing / 2}.Layout(gtx, st.Label("×").Layout)
			})
		}),
	)
}

func (a *App) layoutItemRow(gtx layout.Context, f *openFile, it *fileItem) layout.Dimensions {
	st := a.style
	return layout.Inset{Left: st.IconSize + st.Spacing}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(a.checkBox(&it.visible)),
			layout.Flexed(1, a.rowText(&it.zoom, it.name, it.summary, f.visible.Value && it.visible.Value)),
		)
	})
}

// checkBox returns a check box without label.
func (a *App) checkBox(b *widget.Bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		cb := material.CheckBox(a.style.Theme, b, "")
		cb.Size = a.style.IconSize
		return cb.Layout(gtx)
	}
}

// rowText returns a clickable name with a summary line below.
// Hidden entries are drawn in the hint color.
func (a *App) rowText(clk *widget.Clickable, name, summary string, visible bool) layout.Widget {
	st := a.style
	return func(gtx layout.Context) layout.Dimensions {
		return material.Clickable(gtx, clk, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.Inset{Left: st.Spacing / 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := st.Label(name)
						l.MaxLines = 1
						if !visible {
							l.Color = st.HintFg
						}
						return l.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := st.SmallLabel(summary)
						l.MaxLines = 1
						l.Color = st.HintFg
						return l.Layout(gtx)
					}),
				)
			})
		})
	}
}

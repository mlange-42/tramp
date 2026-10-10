package ui

import (
	"image"
	"image/color"
	"math"
	"slices"

	"gioui.org/f32"
	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/mlange-42/tramp/internal/geo"
)

// panelRow is a row in the side panel: a file, or an item of an expanded file.
type panelRow struct {
	file *openFile
	item *fileItem
}

// rowDrag moves a side panel entry up or down by dragging its handle.
// While dragging, the entry swaps places with a neighbor once the pointer
// has moved past the middle of the neighbor.
type rowDrag struct {
	drag gesture.Drag
	// grabY is the pointer position in the handle when the drag started.
	grabY    float32
	dragging bool
	// height is the height of the row at the last layout.
	height int
}

// update processes drag events and returns -1 to move the entry up, 1 to move it down, or 0.
// prev and next are the heights of the neighbors, zero if there are none.
// At most one move is returned per frame, as the handle moves to its new place only with the next layout.
func (d *rowDrag) update(gtx layout.Context, prev, next int) int {
	var y float32
	moved := false
	for {
		e, ok := d.drag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			d.grabY = e.Position.Y
			d.dragging = true
		case pointer.Drag:
			y, moved = e.Position.Y, true
		case pointer.Release, pointer.Cancel:
			d.dragging = false
		}
	}
	if !moved || !d.dragging {
		return 0
	}
	return dragMove(y-d.grabY, prev, next)
}

// dragMove returns the move for a drag by dy pixels, with neighbors of the given heights.
func dragMove(dy float32, prev, next int) int {
	if next > 0 && dy > float32(next)/2 {
		return 1
	}
	if prev > 0 && dy < -float32(prev)/2 {
		return -1
	}
	return 0
}

// updateFiles processes input on the side panel.
func (a *App) updateFiles(gtx layout.Context) {
	a.updateDrags(gtx)
	for _, f := range a.files {
		if f.close.Clicked(gtx) {
			// Not while iterating the files.
			defer a.closeFile(f)
		}
		if f.editBtn.Clicked(gtx) {
			a.toggleEdit(f)
		}
		if f.visible.Update(gtx) {
			a.tracksChanged = true
		}
		a.updateRowClick(gtx, &f.click, f.firstChartable(), f.bounds)
		if f.expand.Clicked(gtx) {
			f.expanded = !f.expanded
		}
		if f.colorBtn.Clicked(gtx) {
			a.showColorDialog(f.items)
		}
		for _, it := range f.items {
			if it.visible.Update(gtx) {
				a.tracksChanged = true
			}
			a.updateRowClick(gtx, &it.click, it, it.bounds)
			if it.colorBtn.Clicked(gtx) {
				a.showColorDialog([]*fileItem{it})
			}
		}
	}
}

// updateRowClick selects sel for the chart on a click on a row's name, if it is not nil and has lines,
// and zooms to the bounds on a double click.
func (a *App) updateRowClick(gtx layout.Context, clk *widget.Clickable, sel *fileItem, bounds geo.Rect) {
	for {
		c, ok := clk.Update(gtx)
		if !ok {
			break
		}
		if sel != nil && sel.chartable() && sel != a.selected {
			a.selected = sel
			a.tracksChanged = true
		}
		if c.NumClicks >= 2 && !bounds.Empty() {
			a.mapView.Fit(fitRect(bounds))
		}
	}
}

// selectedRow reports whether a panel row shows the item selected for the chart:
// the item's own row, or the row of its file if the item has no row of its own.
func (a *App) selectedRow(r panelRow) bool {
	if a.selected == nil {
		return false
	}
	if r.item != nil {
		return r.item == a.selected
	}
	return (!r.file.expanded || !r.file.expandable()) && slices.Contains(r.file.items, a.selected)
}

// updateDrags moves dragged files, and dragged items within their file.
func (a *App) updateDrags(gtx layout.Context) {
	for i, f := range a.files {
		prev, next := 0, 0
		if i > 0 {
			prev = a.files[i-1].blockHeight()
		}
		if i < len(a.files)-1 {
			next = a.files[i+1].blockHeight()
		}
		if mv := f.drag.update(gtx, prev, next); mv != 0 {
			a.files[i], a.files[i+mv] = a.files[i+mv], a.files[i]
			a.tracksChanged = true
			gtx.Execute(op.InvalidateCmd{})
			return
		}
		for j, it := range f.items {
			prev, next := 0, 0
			if j > 0 {
				prev = f.items[j-1].drag.height
			}
			if j < len(f.items)-1 {
				next = f.items[j+1].drag.height
			}
			if mv := it.drag.update(gtx, prev, next); mv != 0 {
				f.items[j], f.items[j+mv] = f.items[j+mv], f.items[j]
				a.tracksChanged = true
				gtx.Execute(op.InvalidateCmd{})
				return
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
			d := &r.file.drag
			if r.item != nil {
				d = &r.item.drag
			}
			macro := op.Record(gtx.Ops)
			dims := layout.Inset{Bottom: st.Spacing}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if r.item != nil {
					return a.layoutItemRow(gtx, r.file, r.item)
				}
				return a.layoutFileRow(gtx, r.file)
			})
			call := macro.Stop()
			d.height = dims.Size.Y
			if d.dragging {
				paint.FillShape(gtx.Ops, st.DragBg, clip.Rect{Max: dims.Size}.Op())
			} else if a.selectedRow(r) {
				paint.FillShape(gtx.Ops, st.SelectedBg, clip.Rect{Max: dims.Size}.Op())
			}
			call.Add(gtx.Ops)
			return dims
		})
	})
}

func (a *App) layoutFileRow(gtx layout.Context, f *openFile) layout.Dimensions {
	st := a.style
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(a.checkBox(&f.visible)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if len(f.items) == 0 {
				return layout.Dimensions{}
			}
			return a.layoutSwatch(gtx, &f.colorBtn, f.colors())
		}),
		layout.Flexed(1, a.rowText(&f.click, a.fileTitle(f), f.summary, f.visible.Value)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !f.editable() {
				return layout.Dimensions{}
			}
			col := st.HintFg
			if a.editing == f {
				col = st.EditActive
			}
			return material.Clickable(gtx, &f.editBtn, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(st.Spacing/2).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layoutPencil(gtx, gtx.Dp(st.IconSize), col)
				})
			})
		}),
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
		layout.Rigid(a.dragHandle(&f.drag)),
	)
}

func (a *App) layoutItemRow(gtx layout.Context, f *openFile, it *fileItem) layout.Dimensions {
	st := a.style
	return layout.Inset{Left: st.IconSize + st.Spacing}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(a.checkBox(&it.visible)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutSwatch(gtx, &it.colorBtn, []color.NRGBA{it.color})
			}),
			layout.Flexed(1, a.rowText(&it.click, it.name, it.summary, f.visible.Value && it.visible.Value)),
			layout.Rigid(a.dragHandle(&it.drag)),
		)
	})
}

// fileTitle returns the name of the file, marked if it has unsaved changes.
func (a *App) fileTitle(f *openFile) string {
	if a.editing == f && f.dirty() {
		return f.name + " *"
	}
	return f.name
}

// layoutPencil draws a pencil icon of the given size, pointing to the lower left.
func layoutPencil(gtx layout.Context, size int, col color.NRGBA) layout.Dimensions {
	s := float32(size)
	l, w := s*1.15, s*0.28
	tip := w * 1.1
	var path clip.Path
	path.Begin(gtx.Ops)
	// Along the x axis, centered, with the tip at the left.
	path.MoveTo(f32.Pt(-l/2, 0))
	path.LineTo(f32.Pt(-l/2+tip, -w/2))
	path.LineTo(f32.Pt(l/2, -w/2))
	path.LineTo(f32.Pt(l/2, w/2))
	path.LineTo(f32.Pt(-l/2+tip, w/2))
	path.Close()
	spec := path.End()
	defer op.Affine(f32.AffineId().Rotate(f32.Pt(0, 0), -math.Pi/4).Offset(f32.Pt(s/2, s/2))).Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, col, clip.Outline{Path: spec}.Op())
	return layout.Dimensions{Size: image.Pt(size, size)}
}

// dragHandle returns a grip for dragging an entry, drawn as three horizontal lines.
func (a *App) dragHandle(d *rowDrag) layout.Widget {
	st := a.style
	return func(gtx layout.Context) layout.Dimensions {
		dims := layout.UniformInset(st.Spacing/2).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			size := gtx.Dp(st.IconSize)
			w := size * 3 / 4
			x := (size - w) / 2
			th := max(1, gtx.Dp(1.5))
			gap := gtx.Dp(4)
			for i := -1; i <= 1; i++ {
				y := size/2 + i*gap - th/2
				paint.FillShape(gtx.Ops, st.HintFg, clip.Rect(image.Rect(x, y, x+w, y+th)).Op())
			}
			return layout.Dimensions{Size: image.Pt(size, size)}
		})
		defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()
		d.drag.Add(gtx.Ops)
		if d.dragging {
			pointer.CursorGrabbing.Add(gtx.Ops)
		} else {
			pointer.CursorGrab.Add(gtx.Ops)
		}
		return dims
	}
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

package ui

import (
	"image"
	"slices"

	"gioui.org/font"
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

// rowTips are the tooltips of the check box, color swatch and name of a side panel row.
type rowTips struct {
	visible, color, name tooltip
}

// panelRow is a row in the side panel: a file, or an item of an expanded file.
type panelRow struct {
	file *openFile
	item *fileItem
}

// rowDrag moves a side panel entry up or down by dragging its handle.
// While dragging, the entry swaps places with a neighbor once the pointer
// has moved past the middle of the neighbor.
type rowDrag struct {
	drag  gesture.Drag
	hover gesture.Hover
	tip   tooltip
	// row is the hover state of the whole row, for showing its actions.
	row gesture.Hover
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
		if f.terrainBtn.Clicked(gtx) {
			a.saveWithTerrain(f)
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
			a.gps.following = false
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
// The waypoints of a file stay on top, see [openFile.waypointsFirst].
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
			if j > 0 && f.items[j-1].kind != waypointItem {
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
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Flexed(1, a.layoutFileList),
			layout.Rigid(a.layoutProps),
		)
	})
}

// layoutFileList draws the opened files and their items, scrolling if they don't fit.
func (a *App) layoutFileList(gtx layout.Context) layout.Dimensions {
	st := a.style
	return material.List(st.Theme, &a.fileList).Layout(gtx, len(a.panelRows), func(gtx layout.Context, i int) layout.Dimensions {
		r := a.panelRows[i]
		d := &r.file.drag
		if r.item != nil {
			d = &r.item.drag
		}
		// The actions of a row are shown while it is hovered, selected or dragged.
		hovered := d.row.Update(gtx.Source)
		actions := hovered || d.dragging || a.selectedRow(r)
		macro := op.Record(gtx.Ops)
		dims := layout.Inset{Bottom: st.Spacing}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if r.item != nil {
				return a.layoutItemRow(gtx, r.file, r.item, actions)
			}
			return a.layoutFileRow(gtx, r.file, actions)
		})
		call := macro.Stop()
		d.height = dims.Size.Y
		switch {
		case d.dragging:
			paint.FillShape(gtx.Ops, st.DragBg, clip.Rect{Max: dims.Size}.Op())
		case a.selectedRow(r):
			paint.FillShape(gtx.Ops, st.SelectedBg, clip.Rect{Max: dims.Size}.Op())
		case hovered:
			paint.FillShape(gtx.Ops, st.DragBg, clip.Rect{Max: dims.Size}.Op())
		}
		if r.item != nil {
			// A guide line below the chevron of the file connects its items.
			x, w := chevronWidth(gtx, st)/2, max(1, gtx.Dp(1))
			paint.FillShape(gtx.Ops, st.SeparatorColor, clip.Rect(image.Rect(x-w/2, 0, x-w/2+w, dims.Size.Y)).Op())
		}
		call.Add(gtx.Ops)
		// On top of the row, passing events on to its buttons.
		pass := pointer.PassOp{}.Push(gtx.Ops)
		area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
		d.row.Add(gtx.Ops)
		area.Pop()
		pass.Pop()
		return dims
	})
}

// rowAction draws w if show is set. Otherwise, it only takes the space of w,
// so that the name next to it doesn't change its width.
func rowAction(gtx layout.Context, show bool, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()
	if show {
		call.Add(gtx.Ops)
	}
	return dims
}

// layoutFileRow draws the row of a file. actions shows the buttons for closing, saving with terrain and dragging.
// The edit button is always shown. The chevron for expanding the file comes first, or its space if the file has no items to show.
func (a *App) layoutFileRow(gtx layout.Context, f *openFile, actions bool) layout.Dimensions {
	st := a.style
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !f.expandable() {
				sz := chevronWidth(gtx, st)
				return layout.Dimensions{Size: image.Pt(sz, sz)}
			}
			if f.expanded {
				return f.expand.LayoutFlat(gtx, st, iconExpand, st.Theme.Fg, "Hide the tracks, routes and waypoints of the file")
			}
			return f.expand.LayoutFlat(gtx, st, iconNext, st.Theme.Fg, "Show the tracks, routes and waypoints of the file")
		}),
		layout.Rigid(a.visibilityToggle(&f.visible, &f.tips.visible, "Show or hide the file on the map")),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if len(f.items) == 0 {
				// The space of the symbol, so that the names of all files are aligned.
				return layout.Dimensions{Size: image.Pt(fileSymbolWidth(gtx, st), 0)}
			}
			return withTooltip(gtx, st, &f.tips.color, f.colorBtn.Hovered(), "Change the color of all tracks, routes and waypoints of the file",
				func(gtx layout.Context) layout.Dimensions { return a.layoutFileSymbol(gtx, f) })
		}),
		layout.Flexed(1, a.rowText(&f.click, &f.tips.name, a.fileTitle(f), f.summary, f.visible.Value, true)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// Files with tracks are not edited, so they have this button in place of the edit button.
			if !a.terrainSavable(f) {
				return layout.Dimensions{}
			}
			return rowAction(gtx, actions, func(gtx layout.Context) layout.Dimensions {
				return f.terrainBtn.LayoutFlat(gtx, st, iconTerrain, st.HintFg, "Save a copy with the terrain elevation for all track points")
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !f.editable() {
				return layout.Dimensions{}
			}
			col, tip := st.HintFg, "Edit the routes and waypoints of the file"
			if a.editing == f {
				col, tip = st.EditActive, "Stop editing"
			}
			return f.editBtn.LayoutFlat(gtx, st, iconEdit, col, tip)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return rowAction(gtx, actions, func(gtx layout.Context) layout.Dimensions {
				return f.close.LayoutFlat(gtx, st, iconClose, st.Theme.Fg, "Close the file")
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return rowAction(gtx, actions, a.dragHandle(&f.drag))
		}),
	)
}

// chevronWidth returns the width of the chevron for expanding a file in the side panel, a flat icon button.
func chevronWidth(gtx layout.Context, st *Style) int {
	return gtx.Dp(st.IconSize) + 2*gtx.Dp(st.Spacing/2)
}

// layoutItemRow draws the row of an item of a file, indented so that its name is aligned with the name of the file.
// actions shows the drag handle.
func (a *App) layoutItemRow(gtx layout.Context, f *openFile, it *fileItem, actions bool) layout.Dimensions {
	st := a.style
	// The file row has the chevron and the wider file symbol where the item row has its kind symbol.
	indent := chevronWidth(gtx, st) + fileSymbolWidth(gtx, st) - kindSymbolWidth(gtx, st)
	defer op.Offset(image.Pt(indent, 0)).Push(gtx.Ops).Pop()
	gtx.Constraints.Max.X = max(0, gtx.Constraints.Max.X-indent)
	gtx.Constraints.Min.X = min(gtx.Constraints.Min.X, gtx.Constraints.Max.X)
	dims := layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(a.visibilityToggle(&it.visible, &it.tips.visible, "Show or hide on the map")),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return withTooltip(gtx, st, &it.tips.color, it.colorBtn.Hovered(), itemKindNames[it.kind]+". Click to change the color",
				func(gtx layout.Context) layout.Dimensions {
					return a.layoutKindSymbol(gtx, &it.colorBtn, itemKindIcons[it.kind], it.color, f.visible.Value && it.visible.Value)
				})
		}),
		layout.Flexed(1, a.rowText(&it.click, &it.tips.name, it.name, it.summary, f.visible.Value && it.visible.Value, false)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// The waypoints stay on top, see [openFile.waypointsFirst].
			if it.kind == waypointItem {
				sz := gtx.Dp(st.IconSize + st.Spacing)
				return layout.Dimensions{Size: image.Pt(sz, sz)}
			}
			return rowAction(gtx, actions, a.dragHandle(&it.drag))
		}),
	)
	dims.Size.X += indent
	return dims
}

// fileTitle returns the name of the file, marked if it has unsaved changes.
func (a *App) fileTitle(f *openFile) string {
	if a.editing == f && f.dirty() {
		return f.name + " *"
	}
	return f.name
}

// dragHandle returns a grip for dragging an entry, drawn as two columns of three dots.
func (a *App) dragHandle(d *rowDrag) layout.Widget {
	st := a.style
	return func(gtx layout.Context) layout.Dimensions {
		avail := gtx.Constraints.Max.X
		dims := layout.UniformInset(st.Spacing/2).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			size := gtx.Dp(st.IconSize)
			dot, gap := max(2, gtx.Dp(2.5)), gtx.Dp(5)
			for col := range 2 {
				for row := -1; row <= 1; row++ {
					x := size/2 + (2*col-1)*gap/2 - dot/2
					y := size/2 + row*gap - dot/2
					r := image.Rect(x, y, x+dot, y+dot)
					paint.FillShape(gtx.Ops, st.HintFg, clip.Ellipse(r).Op(gtx.Ops))
				}
			}
			return layout.Dimensions{Size: image.Pt(size, size)}
		})
		area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
		d.drag.Add(gtx.Ops)
		d.hover.Add(gtx.Ops)
		if d.dragging {
			pointer.CursorGrabbing.Add(gtx.Ops)
		} else {
			pointer.CursorGrab.Add(gtx.Ops)
		}
		area.Pop()
		d.tip.Layout(gtx, st, d.hover.Update(gtx.Source) && !d.dragging, dims.Size, avail, "Drag to reorder")
		return dims
	}
}

// visibilityToggle returns an eye button for showing and hiding, crossed out while hidden, with a tooltip.
func (a *App) visibilityToggle(b *widget.Bool, tip *tooltip, text string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		st := a.style
		return withTooltip(gtx, st, tip, b.Hovered(), text, func(gtx layout.Context) layout.Dimensions {
			return b.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				size, pad := gtx.Dp(st.IconSize), gtx.Dp(2)
				box := image.Pt(size+2*pad, size+2*pad)
				layoutButtonBg(gtx, st, box, b.Hovered(), false)
				ic, col := iconVisible, st.Theme.Fg
				if !b.Value {
					ic, col = iconHidden, st.HintFg
				}
				layoutIcon(gtx, ic, image.Pt(pad, pad), size, col)
				pointer.CursorPointer.Add(gtx.Ops)
				return layout.Dimensions{Size: box}
			})
		})
	}
}

// rowText returns a clickable name with a summary line below.
// Hidden entries are drawn in the hint color. bold is for the names of files.
func (a *App) rowText(clk *widget.Clickable, tip *tooltip, name, summary string, visible, bold bool) layout.Widget {
	st := a.style
	return func(gtx layout.Context) layout.Dimensions {
		return withTooltip(gtx, st, tip, clk.Hovered(), "Click to show the profile, double-click to zoom to it", func(gtx layout.Context) layout.Dimensions {
			return a.rowName(gtx, clk, name, summary, visible, bold)
		})
	}
}

// rowName draws a clickable name with a summary line below.
func (a *App) rowName(gtx layout.Context, clk *widget.Clickable, name, summary string, visible, bold bool) layout.Dimensions {
	st := a.style
	// Without a highlight of its own, as the whole row is highlighted while hovered.
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		pointer.CursorPointer.Add(gtx.Ops)
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Inset{Left: st.Spacing / 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := st.Label(name)
					l.MaxLines = 1
					if bold {
						l.Font.Weight = font.Bold
					}
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

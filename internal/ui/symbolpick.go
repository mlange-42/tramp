package ui

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/mlange-42/tramp/internal/symbols"
	"github.com/mlange-42/tramp/internal/track"
)

// symbolPicker is the symbol field of the properties box: a button showing the symbol of the point,
// which opens a searchable grid of the known symbols below it.
type symbolPicker struct {
	header widget.Clickable
	open   bool
	// target is the point the picker was opened for.
	target propTarget
	search widget.Editor
	list   widget.List
	// cells are the buttons of the symbols with an icon, by index in [symbols.All].
	cells []symbolCell
	// none is the button for clearing the symbol.
	none symbolCell
}

type symbolCell struct {
	btn widget.Clickable
	tip tooltip
}

// symbolCellSize is the size of the cells in the symbol grid, symbolGridHeight the maximum height of the grid.
const (
	symbolCellSize   unit.Dp = 30
	symbolGridHeight unit.Dp = 220
)

// pointSymbol returns the symbol name of a point target, and false if it doesn't exist.
func pointSymbol(d *track.File, t propTarget) (string, bool) {
	if t.isRoute() {
		return "", false
	}
	if _, ok := vertexPos(d, t.vertex()); !ok {
		return "", false
	}
	return waypointRef(d, t.vertex()).Symbol, true
}

// setSymbol sets the symbol of a point, and of all points linked to it, see [linked].
func setSymbol(d *track.File, t propTarget, sym string) {
	for _, g := range linkedGroup(d, t.vertex()) {
		waypointRef(d, g).Symbol = sym
	}
}

// canonicalSymbol returns the canonical name of a known symbol, or the trimmed name otherwise.
func canonicalSymbol(name string) string {
	name = strings.TrimSpace(name)
	if s, ok := symbols.Lookup(name); ok {
		return s.Name
	}
	return name
}

// updateSymbolPicker processes input on the symbol picker for the selected point, if shown.
// Choosing a symbol applies it and closes the picker. Enter in the search field applies the typed name,
// as the canonical name if it is a known symbol, or as typed otherwise.
func (a *App) updateSymbolPicker(gtx layout.Context, point propTarget, hasPoint bool) {
	p := &a.props.sym
	if p.cells == nil {
		p.cells = make([]symbolCell, len(symbols.All()))
		p.list.Axis = layout.Vertical
		p.search.SingleLine, p.search.Submit = true, true
	}
	if !hasPoint || (p.open && p.target != point) {
		p.close(gtx)
	}
	if p.header.Clicked(gtx) && hasPoint {
		if p.open {
			p.close(gtx)
		} else {
			p.open, p.target = true, point
			p.search.SetText("")
			p.list.Position = layout.Position{}
			gtx.Execute(key.FocusCmd{Tag: &p.search})
		}
	}
	for {
		ev, ok := p.search.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok && p.open {
			if v := strings.TrimSpace(p.search.Text()); v != "" {
				a.applySymbol(canonicalSymbol(v))
			}
			p.close(gtx)
		}
	}
	if p.none.btn.Clicked(gtx) && p.open {
		a.applySymbol("")
		p.close(gtx)
	}
	for i, s := range symbols.All() {
		if p.cells[i].btn.Clicked(gtx) && p.open {
			a.applySymbol(s.Name)
			p.close(gtx)
		}
	}
}

// close closes the picker, giving up the focus of the search field.
func (p *symbolPicker) close(gtx layout.Context) {
	if !p.open {
		return
	}
	p.open = false
	if gtx.Focused(&p.search) {
		gtx.Execute(key.FocusCmd{})
	}
}

// applySymbol sets the symbol of the point the picker was opened for, as a change that can be undone.
func (a *App) applySymbol(sym string) {
	f, t := a.editing, a.props.sym.target
	if f == nil {
		return
	}
	if cur, ok := pointSymbol(f.data, t); !ok || cur == sym {
		return
	}
	f.change(func(d *track.File) { setSymbol(d, t, sym) })
	a.changed()
}

// layoutSymbolField draws the symbol button for a point, and the picker below it if open.
func (a *App) layoutSymbolField(gtx layout.Context, point propTarget) layout.Dimensions {
	st, p := a.style, &a.props.sym
	sym, _ := pointSymbol(a.editing.data, point)
	header := func(gtx layout.Context) layout.Dimensions {
		return layoutPropRow(gtx, st, "Symbol", func(gtx layout.Context) layout.Dimensions {
			return p.header.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layoutFieldBox(gtx, st, p.open, func(gtx layout.Context) layout.Dimensions {
					return a.layoutSymbolLabel(gtx, sym)
				})
			})
		})
	}
	if !p.open {
		return header(gtx)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(header),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: st.Spacing / 2, Left: propLabelWidth}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layoutFieldBox(gtx, st, gtx.Focused(&p.search), func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(st.Theme, &p.search, "Search, or Enter for a custom name")
					ed.TextSize = st.TextSize
					ed.HintColor = st.HintFg
					return ed.Layout(gtx)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: st.Spacing / 2, Left: propLabelWidth}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutSymbolGrid(gtx, canonicalSymbol(sym))
			})
		}),
	)
}

// layoutSymbolLabel draws the icon and name of a symbol, "None" if empty.
// Unknown names are shown as they are, with the icon for unknown symbols.
func (a *App) layoutSymbolLabel(gtx layout.Context, sym string) layout.Dimensions {
	st := a.style
	size := gtx.Sp(st.TextSize) * 5 / 4
	var s *symbols.Symbol
	label, col := sym, st.Theme.Fg
	switch known, ok := symbols.Lookup(sym); {
	case sym == "":
		label, col = "None", st.HintFg
	case ok:
		s, label = known, known.Name
	default:
		s = symbols.Unknown
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if s == nil || s.Icon == nil {
				return layout.Dimensions{}
			}
			layoutIcon(gtx, s.Icon, image.Point{}, size, symbolColor(st, s))
			return layout.Dimensions{Size: image.Pt(size+gtx.Dp(st.Spacing/2), size)}
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			l := st.Label(label)
			l.Color = col
			l.MaxLines = 1
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutArrow(gtx, float32(gtx.Sp(st.TextSize))*0.6, st.Theme.Fg, a.props.sym.open)
		}),
	)
}

// symbolColor returns the color to draw the icon of a symbol in the picker.
func symbolColor(st *Style, s *symbols.Symbol) color.NRGBA {
	if s.Color.A != 0 {
		return s.Color
	}
	return st.Theme.Fg
}

// symbolRow is a row of the symbol grid: a category heading, or cells.
// A cell is an index in [symbols.All], or -1 for clearing the symbol.
type symbolRow struct {
	heading string
	cells   []int
}

// symbolRows returns the rows of the grid with cols columns, for the symbols matching the query, by category.
// The "None" cell comes first, unless searching.
func symbolRows(query string, cols int) []symbolRow {
	var rows []symbolRow
	cat := "\x00"
	if strings.TrimSpace(query) == "" {
		rows = append(rows, symbolRow{cells: []int{-1}})
	}
	for i, s := range symbols.All() {
		if s.Icon == nil || !s.Matches(query) {
			continue
		}
		if s.Category != cat {
			cat = s.Category
			rows = append(rows, symbolRow{heading: cat}, symbolRow{})
		}
		last := &rows[len(rows)-1]
		if len(last.cells) == cols {
			rows = append(rows, symbolRow{})
			last = &rows[len(rows)-1]
		}
		last.cells = append(last.cells, i)
	}
	return rows
}

// layoutSymbolGrid draws the symbols matching the search, by category, scrolling if they don't fit.
// The current symbol is highlighted.
func (a *App) layoutSymbolGrid(gtx layout.Context, current string) layout.Dimensions {
	st, p := a.style, &a.props.sym
	cell := gtx.Dp(symbolCellSize)
	cols := max(1, gtx.Constraints.Max.X/cell)
	rows := symbolRows(p.search.Text(), cols)
	if len(rows) == 0 {
		l := st.SmallLabel("No matching symbol, Enter uses the name as typed")
		l.Color = st.HintFg
		return l.Layout(gtx)
	}
	all := symbols.All()
	gtx.Constraints.Max.Y = min(gtx.Constraints.Max.Y, gtx.Dp(symbolGridHeight))
	gtx.Constraints.Min.Y = 0
	return material.List(st.Theme, &p.list).Layout(gtx, len(rows), func(gtx layout.Context, r int) layout.Dimensions {
		row := rows[r]
		if row.heading != "" {
			return sectionHeading(st, row.heading, r == 0)(gtx)
		}
		children := make([]layout.FlexChild, len(row.cells))
		for j, idx := range row.cells {
			c, name, ic, col, active := &p.none, "No symbol", iconClose, st.HintFg, current == ""
			if idx >= 0 {
				s := all[idx]
				c, name, ic, col, active = &p.cells[idx], s.Name, s.Icon, symbolColor(st, s), s.Name == current
			}
			children[j] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return withTooltip(gtx, st, &c.tip, c.btn.Hovered(), name, func(gtx layout.Context) layout.Dimensions {
					return c.btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						box := image.Rectangle{Max: image.Pt(cell, cell)}
						switch {
						case active:
							paint.FillShape(gtx.Ops, st.SelectedBg, clip.UniformRRect(box, gtx.Dp(st.CornerRadius)).Op(gtx.Ops))
						case c.btn.Hovered():
							paint.FillShape(gtx.Ops, st.DragBg, clip.UniformRRect(box, gtx.Dp(st.CornerRadius)).Op(gtx.Ops))
						}
						size := cell * 2 / 3
						layoutIcon(gtx, ic, image.Pt((cell-size)/2, (cell-size)/2), size, col)
						return layout.Dimensions{Size: box.Max}
					})
				})
			})
		}
		return layout.Flex{}.Layout(gtx, children...)
	})
}

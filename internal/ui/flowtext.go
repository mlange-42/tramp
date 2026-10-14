package ui

import (
	"image"
	"strings"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
)

// textSpan is a part of the text drawn by [layoutFlowText]. With a link, it is drawn in the link color and clickable.
type textSpan struct {
	text string
	link *widget.Clickable
}

// layoutFlowText draws the spans as one paragraph, wrapped at spaces to the maximum width.
// Unlike a label, it allows links within the text.
func layoutFlowText(gtx layout.Context, st *Style, spans ...textSpan) layout.Dimensions {
	type piece struct {
		text string
		link *widget.Clickable
	}
	// words are separated by spaces. The pieces of a word, from different spans, are drawn without a space between them.
	var words [][]piece
	newWord := true
	for _, sp := range spans {
		for i, w := range strings.Split(sp.text, " ") {
			if i > 0 {
				newWord = true
			}
			if w == "" {
				continue
			}
			if newWord {
				words = append(words, nil)
				newWord = false
			}
			words[len(words)-1] = append(words[len(words)-1], piece{text: w, link: sp.link})
		}
	}

	cgtx := gtx
	cgtx.Constraints = layout.Constraints{Max: gtx.Constraints.Max}
	measure := func(p piece) (op.CallOp, image.Point) {
		macro := op.Record(gtx.Ops)
		var dims layout.Dimensions
		if p.link == nil {
			dims = st.Label(p.text).Layout(cgtx)
		} else {
			dims = p.link.Layout(cgtx, func(gtx layout.Context) layout.Dimensions {
				l := st.Label(p.text)
				l.Color = st.Theme.ContrastBg
				if p.link.Hovered() {
					l.Color = st.Theme.Fg
				}
				pointer.CursorPointer.Add(gtx.Ops)
				return l.Layout(gtx)
			})
		}
		return macro.Stop(), dims.Size
	}
	// The width of a space, which labels don't measure on their own.
	_, two := measure(piece{text: "a a"})
	_, one := measure(piece{text: "aa"})
	space := two.X - one.X

	maxW := gtx.Constraints.Max.X
	var x, y, lineH, width int
	for _, w := range words {
		calls := make([]op.CallOp, len(w))
		sizes := make([]image.Point, len(w))
		wordW := 0
		for i, p := range w {
			calls[i], sizes[i] = measure(p)
			wordW += sizes[i].X
		}
		if x > 0 && x+space+wordW > maxW {
			x, y, lineH = 0, y+lineH, 0
		} else if x > 0 {
			x += space
		}
		for i, call := range calls {
			off := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			call.Add(gtx.Ops)
			off.Pop()
			x += sizes[i].X
			lineH = max(lineH, sizes[i].Y)
		}
		width = max(width, x)
	}
	return layout.Dimensions{Size: image.Pt(max(width, gtx.Constraints.Min.X), max(y+lineH, gtx.Constraints.Min.Y))}
}

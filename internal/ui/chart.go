package ui

import (
	"gioui.org/layout"
	"gioui.org/op/paint"
)

// layoutChart draws the chart of the selected track below the map.
func (a *App) layoutChart(gtx layout.Context) layout.Dimensions {
	st := a.style
	paint.Fill(gtx.Ops, st.SideBg)
	st.SideInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		l := st.SmallLabel("No track selected")
		l.Color = st.HintFg
		return l.Layout(gtx)
	})
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

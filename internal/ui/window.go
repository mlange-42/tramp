package ui

import (
	"image"

	"gioui.org/app"
	"gioui.org/unit"
	"github.com/mlange-42/tramp/internal/settings"
)

// defaultWindow is the window size used if none was saved.
var defaultWindow = settings.Window{Width: 1100, Height: 700}

// WindowOptions returns the options for creating the main window with the saved size and state.
func WindowOptions(w *settings.Window) []app.Option {
	if !w.Valid() {
		w = &defaultWindow
	}
	opts := []app.Option{app.Title("TRAMP"), app.Size(unit.Dp(w.Width), unit.Dp(w.Height))}
	// With placement support, the state is restored together with the position.
	if w.Maximized && !hasPlacement {
		opts = append(opts, app.Maximized.Option())
	}
	return opts
}

// trackConfig records the window size and state for saving them.
func (a *App) trackConfig(c app.Config) {
	if hasPlacement {
		// The placement is read when the window is closed.
		return
	}
	switch c.Mode {
	case app.Windowed:
		// The metric is only known after the first frame.
		if a.metric.PxPerDp > 0 {
			a.win.Width = float32(c.Size.X) / a.metric.PxPerDp
			a.win.Height = float32(c.Size.Y) / a.metric.PxPerDp
		}
		a.win.Maximized = false
	case app.Maximized:
		a.win.Maximized = true
	}
}

const (
	// titleHeight is the assumed height of the title bar, in physical pixels.
	titleHeight = 32
	// minVisibleTitle is the visible width of the title bar required to keep a restored window in place,
	// so that it can still be dragged.
	minVisibleTitle = 100
	// sizeTolerance allows for invisible window borders when checking whether a window fits on a monitor.
	sizeTolerance = 16
)

// titleBar returns the approximate title bar area of a window.
func titleBar(r image.Rectangle) image.Rectangle {
	return image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+titleHeight)
}

// titleVisible reports whether enough of the window's title bar is in the work area to drag it.
func titleVisible(r, work image.Rectangle) bool {
	title := titleBar(r)
	vis := title.Intersect(work)
	return vis.Dx() >= min(minVisibleTitle, title.Dx()) && vis.Dy() >= titleHeight/2
}

// fitWindow shrinks a window to fit into the work area of a monitor.
// If keep is true, the window stays in place unless it is too large.
// Otherwise, it is centered in the work area.
func fitWindow(r, work image.Rectangle, keep bool) image.Rectangle {
	size := r.Size()
	tooLarge := size.X > work.Dx()+sizeTolerance || size.Y > work.Dy()+sizeTolerance
	if keep && !tooLarge {
		return r
	}
	size.X = min(size.X, work.Dx())
	size.Y = min(size.Y, work.Dy())
	var pos image.Point
	if keep {
		pos.X = max(work.Min.X, min(r.Min.X, work.Max.X-size.X))
		pos.Y = max(work.Min.Y, min(r.Min.Y, work.Max.Y-size.Y))
	} else {
		pos = work.Min.Add(work.Size().Sub(size).Div(2))
	}
	return image.Rectangle{Min: pos, Max: pos.Add(size)}
}

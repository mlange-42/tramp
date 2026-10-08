// Package ui contains the TRAMP application window and its layout.
package ui

import (
	"fmt"
	"image"
	"image/color"
	"net/http"
	"time"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/tiles"
	"github.com/mlange-42/tramp/internal/wms"
)

// UserAgent is sent with all map requests.
const UserAgent = "TRAMP (+https://github.com/mlange-42/tramp)"

// Options configure the application.
type Options struct {
	Layers []wms.Layer
	Center geo.LonLat
	Zoom   float64
}

// App is the main application state.
type App struct {
	window *app.Window
	theme  *material.Theme
	client *wms.Client

	layers      []wms.Layer
	layerBtns   []widget.Clickable
	activeLayer int
	tiles       *tiles.Manager

	mapView *mapview.Map
}

// New creates the application for the given window.
func New(w *app.Window, opts Options) *App {
	a := &App{
		window:    w,
		theme:     material.NewTheme(),
		client:    &wms.Client{HTTP: &http.Client{Timeout: 30 * time.Second}, UserAgent: UserAgent},
		layers:    opts.Layers,
		layerBtns: make([]widget.Clickable, len(opts.Layers)),
		mapView:   mapview.New(opts.Center, opts.Zoom),
	}
	a.setLayer(0)
	return a
}

// Run processes window events until the window is closed.
func (a *App) Run() error {
	defer func() { a.tiles.Close() }()

	var ops op.Ops
	for {
		switch e := a.window.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			a.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (a *App) setLayer(i int) {
	if a.tiles != nil {
		// Closing waits for running downloads to abort, so don't block the UI.
		go a.tiles.Close()
	}
	a.activeLayer = i
	opts := tiles.DefaultOptions()
	opts.OnUpdate = a.window.Invalidate
	a.tiles = tiles.NewManager(&tiles.WMSFetcher{Client: a.client, Layer: &a.layers[i]}, opts)
}

func (a *App) layout(gtx layout.Context) layout.Dimensions {
	for i := range a.layerBtns {
		if a.layerBtns[i].Clicked(gtx) && i != a.activeLayer {
			a.setLayer(i)
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(a.layoutToolbar),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.mapView.Layout(gtx, a.tiles)
		}),
		layout.Rigid(a.layoutStatus),
	)
}

func (a *App) layoutToolbar(gtx layout.Context) layout.Dimensions {
	children := make([]layout.FlexChild, len(a.layers))
	for i := range a.layers {
		children[i] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			btn := material.Button(a.theme, &a.layerBtns[i], a.layers[i].Name)
			if i != a.activeLayer {
				btn.Background = color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff}
			}
			btn.Inset = layout.UniformInset(unit.Dp(6))
			return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, btn.Layout)
		})
	}
	return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{}.Layout(gtx, children...)
	})
}

func (a *App) layoutStatus(gtx layout.Context) layout.Dimensions {
	pos := "-"
	if a.mapView.HoverValid {
		ll := geo.ToLonLat(a.mapView.Hover)
		pos = fmt.Sprintf("%.5f°N  %.5f°E", ll.Lat, ll.Lon)
	}
	status := fmt.Sprintf("%s   zoom %.1f (tiles %d)", pos, a.mapView.View.Zoom, a.mapView.TileLevel())
	if n := a.tiles.Loading(); n > 0 {
		status += fmt.Sprintf("   loading %d", n)
	}

	macro := op.Record(gtx.Ops)
	dims := layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
			layout.Rigid(material.Caption(a.theme, status).Layout),
			layout.Rigid(material.Caption(a.theme, a.layers[a.activeLayer].Attribution).Layout),
		)
	})
	call := macro.Stop()

	paint.FillShape(gtx.Ops, color.NRGBA{R: 0xf4, G: 0xf4, B: 0xf4, A: 0xff}, clip.Rect(image.Rectangle{Max: dims.Size}).Op())
	call.Add(gtx.Ops)
	return dims
}

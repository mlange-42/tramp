// Package ui contains the TRAMP application window and its layout.
package ui

import (
	"fmt"
	"image"
	"net/http"
	"slices"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/tiles"
	"github.com/mlange-42/tramp/internal/wms"
)

// UserAgent is sent with all map requests.
const UserAgent = "TRAMP (+https://github.com/mlange-42/tramp)"

// Options configure the application.
type Options struct {
	Layers   []wms.Layer
	Overlays []Overlay
	Center   geo.LonLat
	Zoom     float64
}

// overlayState is an overlay with its tiles, which are only loaded while it is enabled.
type overlayState struct {
	Overlay
	tiles *tiles.Manager
}

// App is the main application state.
type App struct {
	window *app.Window
	// invalidate requests a redraw. Safe for concurrent use.
	invalidate func()
	style      *Style
	client     *wms.Client

	layers       []wms.Layer
	layerNames   []string
	layerSelect  Select
	tiles        *tiles.Manager
	overlays     []overlayState
	overlayNames []string
	overlaySel   *MultiSelect

	mapView   *mapview.Map
	mapLayers []mapview.Layer
}

// New creates the application for the given window.
func New(w *app.Window, opts Options) *App {
	a := newApp(w.Invalidate, opts)
	a.window = w
	return a
}

func newApp(invalidate func(), opts Options) *App {
	a := &App{
		invalidate: invalidate,
		style:      DefaultStyle(),
		client:     &wms.Client{HTTP: &http.Client{Timeout: 30 * time.Second}, UserAgent: UserAgent},
		layers:     opts.Layers,
		overlaySel: NewMultiSelect(len(opts.Overlays)),
		mapView:    mapview.New(opts.Center, opts.Zoom),
	}
	for _, l := range opts.Layers {
		a.layerNames = append(a.layerNames, l.Name)
	}
	for _, o := range opts.Overlays {
		a.overlays = append(a.overlays, overlayState{Overlay: o})
		a.overlayNames = append(a.overlayNames, o.Layer.Name)
	}
	a.setLayer(0)
	return a
}

// Run processes window events until the window is closed.
func (a *App) Run() error {
	defer a.closeTiles()

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

func (a *App) closeTiles() {
	a.tiles.Close()
	for _, o := range a.overlays {
		if o.tiles != nil {
			o.tiles.Close()
		}
	}
}

func (a *App) newManager(layer *wms.Layer, shade bool) *tiles.Manager {
	opts := tiles.DefaultOptions()
	opts.OnUpdate = a.invalidate
	return tiles.NewManager(&tiles.WMSFetcher{Client: a.client, Layer: layer, Shade: shade}, opts)
}

func (a *App) setLayer(i int) {
	if a.tiles != nil {
		// Closing waits for running downloads to abort, so don't block the UI.
		go a.tiles.Close()
	}
	a.layerSelect.SetSelected(i)
	a.tiles = a.newManager(&a.layers[i], false)
}

func (a *App) setOverlay(i int, enabled bool) {
	o := &a.overlays[i]
	if enabled && o.tiles == nil {
		layer := o.Layer
		layer.Transparent = true
		o.tiles = a.newManager(&layer, o.Shade)
	} else if !enabled && o.tiles != nil {
		go o.tiles.Close()
		o.tiles = nil
	}
}

func (a *App) update(gtx layout.Context) {
	if a.layerSelect.Update(gtx) {
		a.setLayer(a.layerSelect.Selected())
	}
	for _, i := range a.overlaySel.Update(gtx) {
		a.setOverlay(i, a.overlaySel.Checked(i))
	}
}

func (a *App) layout(gtx layout.Context) layout.Dimensions {
	a.update(gtx)
	paint.Fill(gtx.Ops, a.style.Theme.Bg)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(a.layoutToolbar),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			a.mapLayers = append(a.mapLayers[:0], mapview.Layer{Tiles: a.tiles})
			for _, o := range a.overlays {
				if o.tiles != nil {
					a.mapLayers = append(a.mapLayers, mapview.Layer{Tiles: o.tiles, Opacity: o.Opacity})
				}
			}
			return a.mapView.Layout(gtx, a.mapLayers...)
		}),
		layout.Rigid(a.layoutStatus),
	)
}

func (a *App) layoutToolbar(gtx layout.Context) layout.Dimensions {
	st := a.style
	return st.BarInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(st.Label("Map").Layout),
			layout.Rigid(layout.Spacer{Width: st.Spacing}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layerSelect.Layout(gtx, st, a.layerNames)
			}),
			layout.Rigid(layout.Spacer{Width: st.GroupSpacing}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.overlaySel.Layout(gtx, st, a.overlayLabel(), a.overlayNames)
			}),
		)
	})
}

func (a *App) overlayLabel() string {
	n := 0
	for _, o := range a.overlays {
		if o.tiles != nil {
			n++
		}
	}
	if n == 0 {
		return "Overlays"
	}
	return fmt.Sprintf("Overlays (%d)", n)
}

func (a *App) layoutStatus(gtx layout.Context) layout.Dimensions {
	pos := "-"
	if a.mapView.HoverValid {
		ll := geo.ToLonLat(a.mapView.Hover)
		pos = fmt.Sprintf("%.5f°N  %.5f°E", ll.Lat, ll.Lon)
	}
	status := fmt.Sprintf("%s   zoom %.1f (tiles %d)", pos, a.mapView.View.Zoom, a.mapView.TileLevel())

	loading := a.tiles.Loading()
	attribution := []string{a.layers[a.layerSelect.Selected()].Attribution}
	for _, o := range a.overlays {
		if o.tiles != nil {
			loading += o.tiles.Loading()
			if !slices.Contains(attribution, o.Layer.Attribution) {
				attribution = append(attribution, o.Layer.Attribution)
			}
		}
	}
	if loading > 0 {
		status += fmt.Sprintf("   loading %d", loading)
	}

	st := a.style
	macro := op.Record(gtx.Ops)
	dims := st.BarInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Flex{}.Layout(gtx,
			layout.Rigid(st.SmallLabel(status).Layout),
			layout.Rigid(layout.Spacer{Width: st.GroupSpacing}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				l := st.SmallLabel(strings.Join(attribution, " | "))
				l.Alignment = text.End
				l.MaxLines = 1
				return l.Layout(gtx)
			}),
		)
	})
	call := macro.Stop()

	paint.FillShape(gtx.Ops, st.StatusBg, clip.Rect(image.Rectangle{Max: dims.Size}).Op())
	call.Add(gtx.Ops)
	return dims
}

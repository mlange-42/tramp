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
	"github.com/mlange-42/tramp/internal/settings"
	"github.com/mlange-42/tramp/internal/tiles"
	"github.com/mlange-42/tramp/internal/wms"
)

// UserAgent is sent with all map requests.
const UserAgent = "TRAMP (+https://github.com/mlange-42/tramp)"

// worldBounds is the area shown if no initial view is given.
var worldBounds = geo.Rect{
	Min: geo.ToMercator(geo.LonLat{Lon: -180, Lat: -60}),
	Max: geo.ToMercator(geo.LonLat{Lon: 180, Lat: 75}),
}

// Options configure the application.
type Options struct {
	Layers   []wms.Layer
	Overlays []Overlay
	// State is the initial map, overlays and view, as returned by [App.State].
	// Unknown map and overlay names are ignored.
	// If State.View is nil, the map shows the whole world.
	State settings.Settings
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

	// tileCache is the number of tiles kept in memory, shared by all visible layers.
	tileCache    int
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
		tileCache:  opts.State.TileCache,
		layers:     opts.Layers,
		overlaySel: NewMultiSelect("Overlays", len(opts.Overlays)),
		mapView:    mapview.New(geo.LonLat{}, 0),
	}
	for _, l := range opts.Layers {
		a.layerNames = append(a.layerNames, l.Name)
	}
	for _, o := range opts.Overlays {
		a.overlays = append(a.overlays, overlayState{Overlay: o})
		a.overlayNames = append(a.overlayNames, o.Layer.Name)
	}
	a.setLayer(max(0, slices.Index(a.layerNames, opts.State.Map)))
	for i, name := range a.overlayNames {
		if slices.Contains(opts.State.Overlays, name) {
			a.overlaySel.SetChecked(i, true)
			a.setOverlay(i, true)
		}
	}
	if v := opts.State.View; v != nil {
		a.mapView.View.Center = geo.ToMercator(geo.LonLat{Lon: v.Lon, Lat: v.Lat})
		a.mapView.SetZoom(v.Zoom)
	} else {
		a.mapView.Fit(worldBounds)
	}
	return a
}

// State returns the selected map and overlays and the current view, for saving them.
func (a *App) State() settings.Settings {
	ll := geo.ToLonLat(a.mapView.View.Center)
	s := settings.Settings{
		TileCache: a.tileCache,
		Map:       a.layerNames[a.layerSelect.Selected()],
		View:      &settings.View{Lon: ll.Lon, Lat: ll.Lat, Zoom: a.mapView.View.Zoom},
	}
	for i, name := range a.overlayNames {
		if a.overlaySel.Checked(i) {
			s.Overlays = append(s.Overlays, name)
		}
	}
	return s
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
	a.updateCapacity()
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
	a.updateCapacity()
}

// updateCapacity splits the tile cache evenly between the visible layers.
func (a *App) updateCapacity() {
	if a.tileCache <= 0 {
		// Use the tile manager's default.
		return
	}
	managers := []*tiles.Manager{a.tiles}
	for _, o := range a.overlays {
		if o.tiles != nil {
			managers = append(managers, o.tiles)
		}
	}
	for _, m := range managers {
		m.SetCapacity(a.tileCache / len(managers))
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
				return a.overlaySel.Layout(gtx, st, a.overlayNames)
			}),
		)
	})
}

func (a *App) layoutStatus(gtx layout.Context) layout.Dimensions {
	pos := "-"
	if a.mapView.HoverValid {
		pos = geo.ToLonLat(a.mapView.Hover).String()
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

// Package ui contains the TRAMP application window and its layout.
package ui

import (
	"fmt"
	"image"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
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
	// Gradients are the color gradients for coloring tracks. If empty, the built-in ones are used.
	Gradients []Gradient
	// State is the initial map, overlays and view, as returned by [App.State].
	// Unknown map and overlay names are ignored.
	// If State.View is nil, the map shows the whole world.
	// State.Files are reopened.
	State settings.Settings
	// Files are opened in addition to State.Files, and shown on the map.
	Files []string
}

// overlayState is an overlay with its tiles, which are only loaded while it is enabled.
type overlayState struct {
	Overlay
	tiles *tiles.Manager
}

// App is the main application state.
type App struct {
	window *app.Window
	// win is the window size, position and state, for saving them.
	win    settings.Window
	metric unit.Metric
	plat   platformWindow
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

	split      Split
	chartSplit Split
	mapView    *mapview.Map
	mapLayers  []mapview.Layer
	tracks     mapview.Lines
	// Sizes of tracks, routes and waypoints on the map.
	trackWidth, routeWidth, waypointSize unit.Dp

	// colorBy selects the metric to color tracks by, gradientSel the gradient.
	colorBy     Select
	gradientSel Select
	// gradients are the available gradients, gradientNames their names.
	gradients     []Gradient
	gradientNames []string
	// metricGradients are the selected gradients per metric, as indices in gradients.
	metricGradients []int
	legend          *legend

	openBtn   widget.Clickable
	fileList  widget.List
	files     []*openFile
	panelRows []panelRow
	// tracksChanged requests updating the lines on the map.
	tracksChanged bool
	dialogOpen    atomic.Bool

	// nextColor is the index of the default color for the next opened file.
	nextColor int

	// bgMu guards the fields for results of background work: files being read and chosen colors.
	bgMu         sync.Mutex
	nextBatch    int
	pending      map[int][]settings.File
	loaded       []loadBatch
	colorChanges []colorChange
}

// New creates the application for the given window.
func New(w *app.Window, opts Options) *App {
	a := newApp(w.Invalidate, opts)
	a.window = w
	return a
}

func newApp(invalidate func(), opts Options) *App {
	a := &App{
		invalidate:   invalidate,
		style:        DefaultStyle(),
		client:       &wms.Client{HTTP: &http.Client{Timeout: 30 * time.Second}, UserAgent: UserAgent},
		tileCache:    opts.State.TileCache,
		layers:       opts.Layers,
		overlaySel:   NewMultiSelect("Overlays", len(opts.Overlays)),
		mapView:      mapview.New(geo.LonLat{}, 0),
		trackWidth:   dpOr(opts.State.Style.TrackWidth, settings.DefaultTrackWidth),
		routeWidth:   dpOr(opts.State.Style.RouteWidth, settings.DefaultRouteWidth),
		waypointSize: dpOr(opts.State.Style.WaypointSize, settings.DefaultWaypointSize),
		fileList:     widget.List{List: layout.List{Axis: layout.Vertical}},
		pending:      map[int][]settings.File{},
	}
	if opts.State.Window.Valid() {
		a.win = *opts.State.Window
	} else {
		a.win = defaultWindow
	}
	a.split.Size = dpOr(a.win.PanelWidth, settings.DefaultPanelWidth)
	a.chartSplit = Split{Axis: layout.Vertical, End: true, Size: dpOr(a.win.ChartHeight, settings.DefaultChartHeight)}
	var mapName string
	var overlays []string
	if v := opts.State.View; v != nil {
		mapName, overlays = v.Map, v.Overlays
	}
	for _, l := range opts.Layers {
		a.layerNames = append(a.layerNames, l.Name)
	}
	for _, o := range opts.Overlays {
		a.overlays = append(a.overlays, overlayState{Overlay: o})
		a.overlayNames = append(a.overlayNames, o.Layer.Name)
	}
	a.setLayer(max(0, slices.Index(a.layerNames, mapName)))
	for i, name := range a.overlayNames {
		if slices.Contains(overlays, name) {
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
	a.initColoring(opts.Gradients, opts.State.Style.ColorBy, opts.State.Style.Gradients)
	a.openFiles(slices.Clone(opts.State.Files), false)
	files := make([]settings.File, len(opts.Files))
	for i, p := range opts.Files {
		files[i].Path = p
	}
	a.openFiles(files, true)
	return a
}

// dpOr returns v, or def if v is not positive.
func dpOr(v, def float32) unit.Dp {
	if v <= 0 {
		return unit.Dp(def)
	}
	return unit.Dp(v)
}

// State returns the selected map and overlays and the current view, for saving them.
func (a *App) State() settings.Settings {
	ll := geo.ToLonLat(a.mapView.View.Center)
	s := settings.Settings{
		TileCache: a.tileCache,
		Style: settings.Style{
			TrackWidth:   float32(a.trackWidth),
			RouteWidth:   float32(a.routeWidth),
			WaypointSize: float32(a.waypointSize),
		},
		View: &settings.View{
			Lon:  ll.Lon,
			Lat:  ll.Lat,
			Zoom: a.mapView.View.Zoom,
			Map:  a.layerNames[a.layerSelect.Selected()],
		},
		Window: new(a.win),
		Files:  a.fileState(),
	}
	s.Window.PanelWidth = float32(a.split.Size)
	s.Window.ChartHeight = float32(a.chartSplit.Size)
	s.Style.ColorBy, s.Style.Gradients = a.coloringState()
	for i, name := range a.overlayNames {
		if a.overlaySel.Checked(i) {
			s.View.Overlays = append(s.View.Overlays, name)
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
		case app.ConfigEvent:
			a.trackConfig(e.Config)
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			a.metric = gtx.Metric
			a.layout(gtx)
			e.Frame(gtx.Ops)
		default:
			a.plat.event(a, e)
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

	open := a.openBtn.Clicked(gtx)
	for {
		ev, ok := gtx.Event(key.Filter{Name: "O", Required: key.ModShortcut})
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			open = true
		}
	}
	if open {
		a.showOpenDialog()
	}

	a.addLoaded()
	a.applyColors()
	a.updateColoring(gtx)
	a.updateFiles(gtx)
	if a.tracksChanged {
		a.tracksChanged = false
		a.updateTracks()
	}
}

func (a *App) layout(gtx layout.Context) layout.Dimensions {
	a.update(gtx)
	paint.Fill(gtx.Ops, a.style.Theme.Bg)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(a.layoutToolbar),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.split.Layout(gtx, a.style, a.layoutPanel, func(gtx layout.Context) layout.Dimensions {
				return a.chartSplit.Layout(gtx, a.style, a.layoutChart, a.layoutMap)
			})
		}),
		layout.Rigid(a.layoutStatus),
	)
}

func (a *App) layoutMap(gtx layout.Context) layout.Dimensions {
	base := &a.layers[a.layerSelect.Selected()]
	a.mapLayers = append(a.mapLayers[:0], mapview.Layer{Tiles: a.tiles, MaxTileLevel: base.MaxZoom})
	for _, o := range a.overlays {
		if o.tiles != nil {
			a.mapLayers = append(a.mapLayers, mapview.Layer{Tiles: o.tiles, Opacity: o.Opacity, MaxTileLevel: o.Layer.MaxZoom})
		}
	}
	dims := a.mapView.Layout(gtx, a.mapLayers...)
	a.tracks.Layout(gtx, &a.mapView.View)
	a.layoutLegend(gtx)
	return dims
}

func (a *App) layoutToolbar(gtx layout.Context) layout.Dimensions {
	st := a.style
	return st.BarInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return st.Button(&a.openBtn, "Open").Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: st.GroupSpacing}.Layout),
			layout.Rigid(st.Label("Map").Layout),
			layout.Rigid(layout.Spacer{Width: st.Spacing}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layerSelect.Layout(gtx, st, a.layerNames)
			}),
			layout.Rigid(layout.Spacer{Width: st.GroupSpacing}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.overlaySel.Layout(gtx, st, a.overlayNames)
			}),
			layout.Rigid(layout.Spacer{Width: st.GroupSpacing}.Layout),
			layout.Rigid(a.layoutColoring),
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

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

	split     Split
	mapView   *mapview.Map
	mapLayers []mapview.Layer
	tracks    mapview.Lines

	openBtn   widget.Clickable
	fileList  widget.List
	files     []*openFile
	panelRows []panelRow
	// tracksChanged requests updating the lines on the map.
	tracksChanged bool
	dialogOpen    atomic.Bool

	// loadMu guards the fields for files read in the background.
	loadMu    sync.Mutex
	nextBatch int
	pending   map[int][]settings.File
	loaded    []loadBatch
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
		split:      Split{Width: unit.Dp(opts.State.PanelWidth)},
		layers:     opts.Layers,
		overlaySel: NewMultiSelect("Overlays", len(opts.Overlays)),
		mapView:    mapview.New(geo.LonLat{}, 0),
		tracks:     mapview.Lines{Color: trackColor, Width: 3, DotRadius: 4},
		fileList:   widget.List{List: layout.List{Axis: layout.Vertical}},
		pending:    map[int][]settings.File{},
	}
	if opts.State.Window.Valid() {
		a.win = *opts.State.Window
	} else {
		a.win = defaultWindow
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
	a.openFiles(slices.Clone(opts.State.Files), false)
	files := make([]settings.File, len(opts.Files))
	for i, p := range opts.Files {
		files[i].Path = p
	}
	a.openFiles(files, true)
	return a
}

// State returns the selected map and overlays and the current view, for saving them.
func (a *App) State() settings.Settings {
	ll := geo.ToLonLat(a.mapView.View.Center)
	s := settings.Settings{
		TileCache:  a.tileCache,
		PanelWidth: float32(a.split.Width),
		Map:        a.layerNames[a.layerSelect.Selected()],
		View:       &settings.View{Lon: ll.Lon, Lat: ll.Lat, Zoom: a.mapView.View.Zoom},
		Window:     new(a.win),
		Files:      a.fileState(),
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
			return a.split.Layout(gtx, a.style, a.layoutPanel, a.layoutMap)
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

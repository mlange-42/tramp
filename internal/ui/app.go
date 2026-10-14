// Package ui contains the TRAMP application window and its layout.
package ui

import (
	"fmt"
	"image"
	"log"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/geocode"
	"github.com/mlange-42/tramp/internal/httpx"
	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/routing"
	"github.com/mlange-42/tramp/internal/settings"
	"github.com/mlange-42/tramp/internal/tiles"
	"github.com/mlange-42/tramp/internal/wms"
)

// UserAgent is sent with all map, search and routing requests.
// It names the website and the contact address, as the usage policies of the services ask for.
const UserAgent = "TRAMP (+https://github.com/mlange-42/tramp; tramp@mlange.dev)"

// worldBounds is the area shown if no initial view is given.
var worldBounds = geo.Rect{
	Min: geo.ToMercator(geo.LonLat{Lon: -180, Lat: -60}),
	Max: geo.ToMercator(geo.LonLat{Lon: 180, Lat: 75}),
}

// Options configure the application.
type Options struct {
	// Maps and Overlays are the background maps and overlays, with section headings.
	Maps     []Map
	Overlays []Overlay
	// Gradients are the color gradients for coloring tracks. If empty, the built-in ones are used.
	Gradients []Gradient
	// Presets are the stored combinations of map and overlays.
	Presets []Preset
	// SavePresets stores the presets after they were changed. If nil, changes are not stored.
	SavePresets func([]Preset) error
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
	// tiles are the tiles shown, for the time timeValue if the overlay changes over time.
	tiles     *tiles.Manager
	fetcher   *tiles.WMSFetcher
	timeValue string
	// others are the tiles of other times, kept or loaded in the background, see overlaytiles.go.
	others []*timeTiles
	// fallback are the tiles of the previous time, drawn until the shown ones are loaded.
	fallback *tiles.Manager
	// prefetch are the TIME values to load in the background, one step before and after the shown time.
	prefetch []string
	// times are the valid times of overlays that change over time, see overlaytime.go.
	times overlayTimes
	// legend is the legend image from the capabilities, see overlaylegend.go.
	legend overlayLegend
}

// App is the main application state.
type App struct {
	window *app.Window
	// current is the window that invalidate redraws. It changes when the window is recreated, see [App.recreateWindow].
	current *atomic.Pointer[app.Window]
	// deviceLost is the time the GPU device was last lost, see [App.recreateWindow].
	deviceLost time.Time
	// win is the window size, position and state, for saving them.
	win    settings.Window
	metric unit.Metric
	plat   platformWindow
	// invalidate requests a redraw. Safe for concurrent use.
	invalidate func()
	style      *Style
	client     *wms.Client

	// tileCache is the memory in MB for the tiles kept in memory, shared by all visible layers.
	tileCache    int
	diskCache    diskCacheView
	layers       []wms.Layer
	layerNames   []string
	layerSelect  Select
	tiles        *tiles.Manager
	overlays     []overlayState
	overlayNames []string
	overlaySel   *MultiSelect
	// presets are the stored combinations of map and overlays, see presets.go.
	presets     []Preset
	savePresets func([]Preset) error
	presetMenu  presetMenu

	split      Split
	chartSplit Split
	mapView    *mapview.Map
	mapLayers  []mapview.Layer
	tracks     mapview.Lines
	// Sizes of tracks, routes and waypoints on the map.
	trackWidth, routeWidth, waypointSize, symbolSize unit.Dp

	// colorBy selects the metric to color tracks by, gradientSel the gradient.
	colorBy     Select
	gradientSel Select
	// gradients are the available gradients, gradientNames their names.
	gradients     []Gradient
	gradientNames []string
	// metricGradients are the selected gradients per metric, as indices in gradients.
	metricGradients []int
	legend          *legend
	// selected is the item shown in the chart, or nil.
	selected *fileItem
	chart    chart
	// highlight shows the part of the selected item that is visible in the zoomed chart.
	highlight    mapview.Lines
	highlightKey highlightKey
	// hover is the position on the selected item under the pointer in the chart, or near it on the map.
	hover trackHover

	openBtn   iconButton
	newBtn    iconButton
	fileList  widget.List
	files     []*openFile
	panelRows []panelRow
	// tracksChanged requests updating the lines on the map.
	tracksChanged bool
	dialogOpen    atomic.Bool
	// editing is the file in edit mode, or nil.
	editing *openFile
	editor  editor
	props   props
	// closeOK is set when unsaved changes were saved or discarded on closing the window.
	closeOK bool

	// gps shows the live position from a GPS receiver.
	gps gpsView
	// search is the search field for places and coordinates, using geocoder for place names.
	// searchCfg is the configured search service, kept for saving the settings.
	search    searchBox
	geocoder  geocode.Searcher
	searchCfg settings.Search
	// nav shows a route found by router between two points.
	// routingCfg is the configured routing service and the selected profile, kept for saving the settings.
	nav        navigator
	router     routing.Router
	routingCfg settings.Routing
	// elev shows the elevation under the pointer.
	elev elevationView
	// sources is the list of data sources, opened from the status bar.
	sources sourcesView
	// menu is the menu bar at the left of the toolbar.
	menu menuBar
	// about is the About window, opened from the Help menu.
	about aboutView

	// nextColor is the index of the default color for the next opened file.
	nextColor int

	// pending are the files being read, by batch, for saving the settings.
	nextBatch int
	pending   map[int][]settings.File
	// limiter limits the simultaneous tile downloads per server, for all layers.
	limiter *tiles.Limiter

	// timeCtl selects the time of overlays that change over time.
	timeCtl timeControl

	// uiTasks are functions to run on the UI goroutine, see [App.runOnUI], guarded by bgMu.
	bgMu    sync.Mutex
	uiTasks []func()
}

// New creates the application for the given window.
func New(w *app.Window, opts Options) *App {
	current := new(atomic.Pointer[app.Window])
	current.Store(w)
	a := newApp(func() { current.Load().Invalidate() }, opts)
	a.window, a.current = w, current
	return a
}

func newApp(invalidate func(), opts Options) *App {
	layers, layerSections := splitSections(opts.Maps, func(m Map) (string, wms.Layer) { return m.Section, m.Layer })
	overlays, overlaySections := splitSections(opts.Overlays, func(o Overlay) (string, Overlay) { return o.Section, o })
	a := &App{
		invalidate:   invalidate,
		style:        DefaultStyle(),
		limiter:      tiles.NewLimiter(),
		client:       &wms.Client{Client: httpx.Client{HTTP: &http.Client{Timeout: 30 * time.Second}, UserAgent: UserAgent}},
		tileCache:    opts.State.TileCache,
		diskCache:    newDiskCache(opts.State.DiskCache),
		layers:       layers,
		presets:      opts.Presets,
		savePresets:  opts.SavePresets,
		overlaySel:   NewMultiSelect("Overlays", len(overlays)),
		mapView:      mapview.New(geo.LonLat{}, 0),
		trackWidth:   dpOr(opts.State.Style.TrackWidth, settings.DefaultTrackWidth),
		routeWidth:   dpOr(opts.State.Style.RouteWidth, settings.DefaultRouteWidth),
		waypointSize: dpOr(opts.State.Style.WaypointSize, settings.DefaultWaypointSize),
		symbolSize:   dpOr(opts.State.Style.SymbolSize, settings.DefaultSymbolSize),
		fileList:     widget.List{List: layout.List{Axis: layout.Vertical}},
		pending:      map[int][]settings.File{},
		gps:          gpsView{cfg: opts.State.GPS},
		searchCfg:    opts.State.Search,
		routingCfg:   opts.State.Routing,
		elev: elevationView{cfg: opts.State.Elevation, cacheSize: opts.State.ElevationTileCache,
			service: newElevationService(opts.State.Elevation, opts.State.ElevationTileCache)},
	}
	a.menu = newMenuBar(menuTitles...)
	if a.elev.service != nil && a.diskCache.elevation != nil {
		a.elev.service.SetDiskCache(a.diskCache.elevation, a.diskCache.maxAge())
	}
	if opts.State.Window.Valid() {
		a.win = *opts.State.Window
	} else {
		a.win = defaultWindow
	}
	a.split.Size = dpOr(a.win.PanelWidth, settings.DefaultPanelWidth)
	a.chartSplit = Split{
		Axis: layout.Vertical, End: true, Collapsible: true, MinSize: a.style.MinChartHeight,
		Size:      dpOr(a.win.ChartHeight, settings.DefaultChartHeight),
		Collapsed: a.win.ChartClosed,
	}
	a.chart.axis = chartAxisByKey(a.win.ChartAxis)
	a.geocoder = newGeocoder(opts.State.Search)
	a.router = newRouter(opts.State.Routing)
	a.nav.profile = profileIndex(opts.State.Routing.Profile)
	a.loadNav(opts.State.Routing)
	if c := opts.State.Style.MutedColor; c != "" {
		if col, err := parseColorAlpha(c); err == nil {
			a.style.MutedTrack = col
		} else {
			log.Printf("muted_color: %v", err)
		}
	}
	var mapName string
	var enabled []string
	if v := opts.State.View; v != nil {
		mapName, enabled = v.Map, v.Overlays
	}
	for _, l := range a.layers {
		a.layerNames = append(a.layerNames, l.Name)
		a.layerSelect.tips = append(a.layerSelect.tips, l.Description)
	}
	for _, o := range overlays {
		a.overlays = append(a.overlays, overlayState{Overlay: o})
		a.overlayNames = append(a.overlayNames, o.Layer.Name)
		a.overlaySel.tips = append(a.overlaySel.tips, o.Layer.Description)
	}
	a.layerSelect.help, a.layerSelect.icon = "Background map", iconMap
	a.overlaySel.help = "Overlays on the background map, like hillshading"
	a.overlaySel.icon = iconLayers
	a.presetMenu.help, a.presetMenu.icon = "Presets of map and overlays", iconPresets
	a.layerSelect.sections, a.overlaySel.sections = layerSections, overlaySections
	a.colorBy.help, a.colorBy.icon = "Color tracks and routes by speed, elevation or slope", iconPalette
	for _, m := range metrics {
		a.colorBy.icons = append(a.colorBy.icons, m.icon)
	}
	a.gradientSel.help, a.gradientSel.icon = "Color gradient", iconGradient
	a.setLayer(max(0, slices.Index(a.layerNames, mapName)))
	for i, name := range a.overlayNames {
		if slices.Contains(enabled, name) {
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

// newGeocoder returns the searcher for the configured search service.
// An unknown service is logged, and the default is used.
func newGeocoder(cfg settings.Search) geocode.Searcher {
	client := &http.Client{Timeout: 15 * time.Second}
	g, err := geocode.New(cfg.Service, cfg.URL, client, UserAgent)
	if err != nil {
		log.Printf("search: %v", err)
		g, _ = geocode.New("", "", client, UserAgent)
	}
	return g
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
		TileCache:          a.tileCache,
		ElevationTileCache: a.elev.cacheSize,
		DiskCache:          a.diskCache.cfg,
		Style: settings.Style{
			TrackWidth:   float32(a.trackWidth),
			RouteWidth:   float32(a.routeWidth),
			WaypointSize: float32(a.waypointSize),
			SymbolSize:   float32(a.symbolSize),
			MutedColor:   formatColorAlpha(a.style.MutedTrack),
		},
		View: &settings.View{
			Lon:  ll.Lon,
			Lat:  ll.Lat,
			Zoom: a.mapView.View.Zoom,
			Map:  a.layerNames[a.layerSelect.Selected()],
		},
		GPS:       a.gps.cfg,
		Search:    a.searchCfg,
		Routing:   a.navState(),
		Elevation: a.elev.cfg,
		Window:    new(a.win),
		Files:     a.fileState(),
	}
	s.Window.PanelWidth = float32(a.split.Size)
	s.Window.ChartHeight = float32(a.chartSplit.Size)
	s.Window.ChartClosed = a.chartSplit.Collapsed
	s.Window.ChartAxis = chartAxes[a.chart.axis].key
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
	defer a.stopGPS()

	var ops op.Ops
	for {
		switch e := a.window.Event().(type) {
		case app.DestroyEvent:
			if a.recreateWindow(e.Err) {
				continue
			}
			return e.Err
		case *app.ClosingEvent:
			if !a.confirmClose() {
				e.Abort()
			}
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

// closeWindow closes the window.
func (a *App) closeWindow() {
	if a.window != nil {
		a.window.Perform(system.ActionClose)
	}
}

func (a *App) closeTiles() {
	if a.tiles != nil {
		a.tiles.Close()
	}
	for _, o := range a.overlays {
		for _, m := range o.allTiles() {
			m.Close()
		}
	}
}

// newManager creates the tile manager of a layer, and returns it with its fetcher.
// Layers that change over time (refresh) are not stored on disk,
// so that old images are never shown as current, e.g. when offline.
func (a *App) newManager(layer *wms.Layer, shade, refresh bool) (*tiles.Manager, *tiles.WMSFetcher) {
	opts := tiles.DefaultOptions()
	opts.OnUpdate = a.invalidate
	opts.Limiter, opts.Host, opts.Connections = a.limiter, layer.Host(), layer.Connections
	opts.Name = layer.Name
	if layer.Time != "" {
		opts.Name += " at " + layer.Time
	}
	f := &tiles.WMSFetcher{Client: a.client, Layer: layer, Shade: shade}
	if !refresh {
		f.Cache, f.MaxAge = a.diskCache.tiles, a.diskCache.maxAge()
	}
	return tiles.NewManager(f, opts), f
}

func (a *App) setLayer(i int) {
	if a.tiles != nil {
		// Closing waits for running downloads to abort, so don't block the UI.
		go a.tiles.Close()
	}
	a.layerSelect.SetSelected(i)
	a.tiles = nil
	if !a.layers[i].IsNone() {
		a.tiles, _ = a.newManager(&a.layers[i], false, false)
	}
	a.updateCapacity()
}

func (a *App) setOverlay(i int, enabled bool) {
	o := &a.overlays[i]
	if enabled && o.tiles == nil {
		layer := o.Layer
		layer.Transparent = true
		o.tiles, o.fetcher = a.newManager(&layer, o.Shade, o.Refresh > 0)
		o.enabled(a, i, time.Now())
	} else if !enabled && o.tiles != nil {
		o.closeOverlayTiles()
	}
	a.updateCapacity()
}

// updateCapacity splits the tile cache between the visible layers. The kept times of overlays that change
// over time get room for about the visible tiles each, at most half of the cache together,
// and the rest is split evenly between the shown tiles of the layers.
func (a *App) updateCapacity() {
	if a.tileCache <= 0 {
		// Use the tile manager's default.
		return
	}
	var shown, kept []*tiles.Manager
	if a.tiles != nil {
		shown = append(shown, a.tiles)
	}
	perTime := minTimeCapacity
	for _, o := range a.overlays {
		if o.tiles == nil {
			continue
		}
		shown = append(shown, o.tiles)
		for _, t := range o.others {
			kept = append(kept, t.tiles)
		}
		perTime = max(perTime, 2*len(o.tiles.Wanted()))
	}
	capacity := (a.tileCache << 20) / tiles.TileBytes
	if len(kept) > 0 {
		perTime = min(perTime, capacity/2/len(kept))
		capacity -= perTime * len(kept)
	}
	for _, m := range kept {
		m.SetCapacity(perTime)
	}
	for _, m := range shown {
		m.SetCapacity(capacity / len(shown))
	}
}

// minTimeCapacity is the least number of tiles kept for each kept time of an overlay, see [App.updateCapacity].
const minTimeCapacity = 64

func (a *App) update(gtx layout.Context) {
	if a.layerSelect.Update(gtx) {
		a.setLayer(a.layerSelect.Selected())
	}
	for _, i := range a.overlaySel.Update(gtx) {
		a.setOverlay(i, a.overlaySel.Checked(i))
	}
	switch act := a.presetMenu.update(gtx, len(a.presets)); {
	case act.apply >= 0:
		a.applyPreset(a.presets[act.apply])
	case act.remove >= 0:
		a.deletePreset(act.remove)
	case act.save:
		a.savePresetAs()
	}
	a.refreshOverlays(gtx)
	a.updateTimeControl(gtx)

	a.runUITasks()
	a.updateAbout(gtx)
	a.menu.update(gtx, a.menuItems())
	if a.openBtn.Clicked(gtx) {
		a.showOpenDialog()
	}
	if a.newBtn.Clicked(gtx) {
		a.newFile()
	}
	a.updateShortcuts(gtx)
	a.updateSources(gtx)
	a.updateSearch(gtx)

	a.updateColoring(gtx)
	a.updateFiles(gtx)
	a.updateProfiles()
	a.updatePointElevations()
	// The map's hover position is needed before the map is drawn.
	a.mapView.Update(gtx)
	// The route takes the map clicks on its ends, and while picking them, before the editor.
	events := a.updateNav(gtx, a.mapView.Primary())
	a.updateEditor(gtx, events)
	if c, ok := a.navCursor(gtx); ok {
		a.mapView.Cursor = c
	}
	a.updateProps(gtx)
	// The selected item is muted on the map while the chart is zoomed in.
	zoomed := a.chart.zoomed()
	a.chart.update(gtx)
	if a.chart.updateAxis(gtx) {
		a.updateChart()
	}
	if a.chartSplit.Collapsed {
		// A closed chart shows the whole item when opened again, and doesn't affect the map.
		a.chart.setRange(0, a.chart.total)
		a.chart.hovering = false
	}
	if a.chart.zoomed() != zoomed {
		a.tracksChanged = true
	}
	if a.tracksChanged {
		a.tracksChanged = false
		a.updateTracks()
	}
	a.updateHover(gtx)
	// Double-clicking the chart moves the map to the clicked position, without zooming.
	if a.chart.doubleClicked && a.hover.valid {
		a.mapView.View.Center = a.hover.pos
		a.gps.following = false
	}
	// Double-clicking the selected item on the map centers the zoomed chart on the position.
	if a.mapView.DoubleClicked() && a.hover.valid && a.chart.zoomedOn(a.selected) {
		c := &a.chart
		span := c.to - c.from
		c.setRange(a.hover.x-span/2, span)
	}
	a.updateHighlight()
	a.updateGPS(gtx)
}

// updateShortcuts handles the global keyboard shortcuts.
// Key events go to the first handler asking for them, so while typing in a property field,
// undo and redo are left to the field, also in the search field.
func (a *App) updateShortcuts(gtx layout.Context) {
	filters := []event.Filter{
		key.Filter{Name: "O", Required: key.ModShortcut},
		key.Filter{Name: "N", Required: key.ModShortcut},
		key.Filter{Name: "S", Required: key.ModShortcut},
		key.Filter{Name: "F", Required: key.ModShortcut},
		key.Filter{Name: "D", Required: key.ModShortcut},
	}
	if !a.propsFocused(gtx) && !a.searchFocused(gtx) {
		filters = append(filters,
			key.Filter{Name: "Z", Required: key.ModShortcut, Optional: key.ModShift},
			key.Filter{Name: "Y", Required: key.ModShortcut},
		)
	}
	for {
		ev, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}
		switch {
		case e.Name == "O":
			a.showOpenDialog()
		case e.Name == "N":
			a.newFile()
		case e.Name == "S":
			a.save()
		case e.Name == "F":
			a.focusSearch(gtx)
		case e.Name == "D":
			a.toggleNav()
		case e.Name == "Y", e.Name == "Z" && e.Modifiers.Contain(key.ModShift):
			a.redo()
		case e.Name == "Z":
			a.undo()
		}
	}
}

func (a *App) layout(gtx layout.Context) layout.Dimensions {
	a.update(gtx)
	paint.Fill(gtx.Ops, a.style.Theme.Bg)

	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(a.layoutToolbar),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.split.Layout(gtx, a.style, a.layoutPanel, func(gtx layout.Context) layout.Dimensions {
				return a.chartSplit.Layout(gtx, a.style, a.layoutChart, a.layoutMap)
			})
		}),
		layout.Rigid(a.layoutStatus),
	)
	a.layoutAbout(gtx)
	return dims
}

func (a *App) layoutMap(gtx layout.Context) layout.Dimensions {
	base := &a.layers[a.layerSelect.Selected()]
	a.mapLayers = append(a.mapLayers[:0], mapview.Layer{Tiles: a.tiles, MaxTileLevel: base.MaxZoom})
	for _, o := range a.overlays {
		// Overlays waiting for their time are drawn once it is known, so that they are loaded only once.
		if o.tiles != nil && !o.times.hidden() {
			a.mapLayers = append(a.mapLayers, mapview.Layer{Tiles: o.tiles, Fallback: o.fallback, Opacity: o.Opacity, MaxTileLevel: o.Layer.MaxZoom})
		}
	}
	dims := a.mapView.Layout(gtx, a.mapLayers...)
	// After the map requested the visible tiles.
	a.prefetchTimes()
	a.tracks.Layout(gtx, &a.mapView.View)
	// The highlight of the found route is drawn with the route, above the tracks.
	if a.highlightKey.item == nil || a.highlightKey.item != a.nav.item {
		a.highlight.Layout(gtx, &a.mapView.View)
	}
	a.layoutNavMap(gtx)
	a.layoutSearchMarker(gtx)
	a.layoutEditMap(gtx)
	a.layoutHoverMap(gtx)
	a.layoutGPSMarker(gtx)
	// The legend and the GPS button are above the time strip, if shown.
	// The directions place them by the minimum constraints, so both shrink.
	above := gtx
	above.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y-a.layoutTimeline(gtx)))
	a.layoutLegend(above)
	a.layoutGPSButton(above)
	a.layoutEditHint(above)
	a.layoutNavPanel(gtx)
	return dims
}

func (a *App) layoutToolbar(gtx layout.Context) layout.Dimensions {
	st := a.style
	return st.BarInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.menu.layout(gtx, st, a.menuItems())
			}),
			layout.Rigid(layout.Spacer{Width: st.GroupSpacing}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.newBtn.Layout(gtx, st, iconNew, "New GPX file for routes and waypoints (Ctrl+N)", true, false)
			}),
			layout.Rigid(layout.Spacer{Width: 1}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.openBtn.Layout(gtx, st, iconOpen, "Open GPX files (Ctrl+O)", true, false)
			}),
			layout.Rigid(layout.Spacer{Width: st.GroupSpacing}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layerSelect.Layout(gtx, st, a.layerNames)
			}),
			layout.Rigid(layout.Spacer{Width: 1}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.overlaySel.Layout(gtx, st, a.overlayNames)
			}),
			layout.Rigid(layout.Spacer{Width: 1}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.presetMenu.layout(gtx, st, a.presets, a.presetActive)
			}),
			layout.Rigid(layout.Spacer{Width: st.GroupSpacing}.Layout),
			layout.Rigid(a.layoutColoring),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if a.editing == nil {
					return layout.Dimensions{}
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(layout.Spacer{Width: st.GroupSpacing}.Layout),
					layout.Rigid(a.layoutTools),
					layout.Rigid(layout.Spacer{Width: st.GroupSpacing}.Layout),
					layout.Rigid(a.layoutUndoRedo),
				)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, 0)}
			}),
			layout.Rigid(layout.Spacer{Width: st.GroupSpacing}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.nav.btn.Layout(gtx, st, iconDirections, "Find a route between two points on the map (Ctrl+D)", true, a.nav.open)
			}),
			layout.Rigid(layout.Spacer{Width: st.Spacing}.Layout),
			layout.Rigid(a.layoutSearch),
		)
	})
}

// hoverPlaceholder is shown in the status bar instead of the position while the pointer is not on the map,
// as wide as a position.
var hoverPlaceholder = fmt.Sprintf("%-*s", utf8.RuneCountInString(geo.LonLat{}.String()), "-")

func (a *App) layoutStatus(gtx layout.Context) layout.Dimensions {
	// The parts have a fixed width where possible, and the changing ones come last,
	// so that the others don't move. The font is monospaced.
	pos := hoverPlaceholder
	if a.mapView.HoverValid {
		pos = geo.ToLonLat(a.mapView.Hover).String()
	}
	if a.elev.service != nil {
		pos += formatHoverElevation(a.hoverElevation())
	}
	status := fmt.Sprintf("%s   zoom %4.1f (tiles %2d)", pos, a.mapView.View.Zoom, a.mapView.TileLevel())
	if s := a.gpsStatus(); s != "" {
		status += "   " + s
	}

	var loading int
	if a.tiles != nil {
		loading = a.tiles.Loading()
	}
	for _, o := range a.overlays {
		if o.tiles != nil {
			loading += o.tiles.Loading()
		}
	}
	if loading > 0 {
		status += fmt.Sprintf("   loading %d", loading)
	}

	st := a.style
	macro := op.Record(gtx.Ops)
	dims := st.BarInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		// Flex lays out rigid children first, so the sources link always gets its space,
		// and the status and credits are truncated to the rest instead of wrapping.
		return layout.Flex{}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := st.SmallLabel(status)
						l.MaxLines = 1
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: st.GroupSpacing}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						l := st.SmallLabel(shortCredits(a.mapCredits()))
						l.Alignment = text.End
						l.MaxLines = 1
						return l.Layout(gtx)
					}),
				)
			}),
			layout.Rigid(a.layoutSourcesLink),
		)
	})
	call := macro.Stop()

	paint.FillShape(gtx.Ops, st.StatusBg, clip.Rect(image.Rectangle{Max: dims.Size}).Op())
	call.Add(gtx.Ops)
	a.layoutSources(gtx, dims.Size)
	return dims
}

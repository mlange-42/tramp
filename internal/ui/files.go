package ui

import (
	"errors"
	"fmt"
	"image/color"
	"log"
	"math"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/mapview"
	"github.com/mlange-42/tramp/internal/settings"
	"github.com/mlange-42/tramp/internal/track"
	"github.com/ncruces/zenity"
)

// minFitSize is the minimum extent shown when zooming to a file or item, in meters.
const minFitSize = 500

// openFile is an opened track file in the side panel.
type openFile struct {
	path string
	// data is the content of the file, for editing and saving it.
	data    *track.File
	name    string
	summary string
	items   []*fileItem
	bounds  geo.Rect

	visible widget.Bool
	// click is the name of the file: a click selects its first track or route, a double click zooms to it.
	click    widget.Clickable
	close    iconButton
	expand   iconButton
	colorBtn widget.Clickable
	editBtn  iconButton
	tips     rowTips
	expanded bool
	drag     rowDrag
	// defColor is the color for new items of a file without items.
	defColor color.NRGBA

	edit editState
}

// itemKind is the kind of a file item.
type itemKind int

const (
	trackItem itemKind = iota
	routeItem
	waypointItem
)

// fileItem is a track, a route, or the waypoints of a file.
type fileItem struct {
	kind    itemKind
	name    string
	summary string
	lines   []mapview.Polyline
	// points are the original points of the lines, for computing values to color them by.
	points [][]track.Point
	// values caches the values of the lines per metric, see [mapview.LineGroup.Values].
	values map[track.Metric][][]float64
	// dist caches the distance along the item for each point, continuing over all lines, in meters.
	dist [][]float64
	// timeCache caches the result of [fileItem.times], if timesDone.
	timeCache [][]float64
	start     time.Time
	timesDone bool
	// gapCache caches the result of [fileItem.gaps], if gapsDone.
	gapCache [][]bool
	gapsDone bool
	dots     []geo.Point
	bounds   geo.Rect
	color    color.NRGBA
	// index is the position of the item in the file, independent of the panel order.
	index int
	// nth is the position of the item among the items of its kind in the file.
	nth int

	visible widget.Bool
	// click is the name of the item: a click selects it, a double click zooms to it.
	click    widget.Clickable
	colorBtn widget.Clickable
	tips     rowTips
	drag     rowDrag
}

// newOpenFile prepares a read file for the panel and the map.
func newOpenFile(path string, f *track.File) *openFile {
	of := &openFile{path: path, data: f, name: filepath.Base(path)}
	of.visible.Value = true
	of.items = buildItems(f)
	for _, it := range of.items {
		it.visible.Value = true
	}
	of.update()
	return of
}

// buildItems returns the items for the content of the file, in file order, without panel state.
func buildItems(f *track.File) []*fileItem {
	var items []*fileItem
	for i := range f.Tracks {
		t := &f.Tracks[i]
		it := &fileItem{kind: trackItem, nth: i, name: itemName(t.Name, "Track", i, len(f.Tracks)), summary: trackSummary(t)}
		for j := range t.Segments {
			pts := make([]geo.Point, t.Segments[j].Len())
			for k, p := range t.Segments[j].Points {
				pts[k] = geo.ToMercator(p.Pos)
			}
			it.lines = append(it.lines, mapview.NewPolyline(pts))
			it.points = append(it.points, t.Segments[j].Points)
		}
		items = append(items, it)
	}
	for i := range f.Routes {
		r := &f.Routes[i]
		it := &fileItem{kind: routeItem, nth: i, name: itemName(r.Name, "Route", i, len(f.Routes)), summary: "route · " + formatDistance(r.Length())}
		pts := make([]geo.Point, len(r.Points))
		tps := make([]track.Point, len(r.Points))
		for k, p := range r.Points {
			pts[k] = geo.ToMercator(p.Pos)
			tps[k] = track.Point{Pos: p.Pos, Ele: p.Ele, Time: p.Time}
		}
		it.lines = append(it.lines, mapview.NewPolyline(pts))
		it.points = append(it.points, tps)
		items = append(items, it)
	}
	if len(f.Waypoints) > 0 {
		it := &fileItem{kind: waypointItem, name: "Waypoints", summary: count(len(f.Waypoints), "waypoint")}
		for _, w := range f.Waypoints {
			it.dots = append(it.dots, geo.ToMercator(w.Pos))
		}
		items = append(items, it)
	}
	for _, it := range items {
		it.bounds = geo.EmptyRect()
		for _, l := range it.lines {
			it.bounds = it.bounds.Union(l.Bounds)
		}
		for _, p := range it.dots {
			it.bounds = it.bounds.Extend(p)
		}
	}
	return items
}

// update sets the file indices of the items, which are in file order, and the bounds and summary of the file.
func (f *openFile) update() {
	f.bounds = geo.EmptyRect()
	for i, it := range f.items {
		it.index = i
		f.bounds = f.bounds.Union(it.bounds)
	}
	switch len(f.items) {
	case 0:
		f.summary = "empty"
	case 1:
		f.summary = f.items[0].summary
	default:
		var parts []string
		if n := len(f.data.Tracks); n > 0 {
			parts = append(parts, count(n, "track"))
		}
		if n := len(f.data.Routes); n > 0 {
			parts = append(parts, count(n, "route"))
		}
		if n := len(f.data.Waypoints); n > 0 {
			parts = append(parts, count(n, "waypoint"))
		}
		f.summary = strings.Join(parts, " · ")
	}
}

// rebuild updates the items after the file data changed.
// An item that still exists, by kind and position among the items of its kind, keeps its color,
// visibility and place in the panel. New items are visible, in the file's first color, and listed last.
func (f *openFile) rebuild() {
	old := f.items
	built := buildItems(f.data)
	f.items = make([]*fileItem, len(built))
	var added []*fileItem
	for i, n := range built {
		j := slices.IndexFunc(old, func(o *fileItem) bool { return o.kind == n.kind && o.nth == n.nth })
		if j < 0 {
			n.visible.Value = true
			n.color = f.defColor
			if len(old) > 0 {
				n.color = old[0].color
			}
			f.items[i] = n
			added = append(added, n)
			continue
		}
		f.items[i] = old[j]
		old[j].setContent(n)
	}
	f.update()

	// Restore the panel order.
	var order []*fileItem
	for _, o := range old {
		if slices.Contains(f.items, o) {
			order = append(order, o)
		}
	}
	f.items = append(order, added...)
}

// setContent replaces the content of the item with that of n, and clears the caches.
func (it *fileItem) setContent(n *fileItem) {
	it.name, it.summary = n.name, n.summary
	it.lines, it.points, it.dots, it.bounds = n.lines, n.points, n.dots, n.bounds
	it.values, it.dist = nil, nil
	it.timeCache, it.start, it.timesDone = nil, time.Time{}, false
	it.gapCache, it.gapsDone = nil, false
}

// order returns the file indices of the items in panel order, or nil if that is the file order.
func (f *openFile) order() []int {
	order := make([]int, len(f.items))
	sorted := true
	for i, it := range f.items {
		order[i] = it.index
		sorted = sorted && it.index == i
	}
	if sorted {
		return nil
	}
	return order
}

// setOrder puts the items in the given order of file indices.
// It is ignored unless it is a permutation of all items, e.g. if the file changed.
func (f *openFile) setOrder(order []int) {
	if len(order) != len(f.items) {
		return
	}
	items := make([]*fileItem, len(order))
	for i, idx := range order {
		if idx < 0 || idx >= len(items) || slices.Contains(items, f.items[idx]) {
			return
		}
		items[i] = f.items[idx]
	}
	f.items = items
}

// blockHeight returns the height of the file row and its item rows, if expanded, at the last layout.
func (f *openFile) blockHeight() int {
	h := f.drag.height
	if f.expanded && f.expandable() {
		for _, it := range f.items {
			h += it.drag.height
		}
	}
	return h
}

// lineValues returns the values of a metric for the lines of the item,
// or nil if no line has values. Gaps in the recording have no values, see [track.Gaps].
func (it *fileItem) lineValues(m track.Metric) [][]float64 {
	if m == track.NoMetric {
		return nil
	}
	if vals, ok := it.values[m]; ok {
		return vals
	}
	vals := make([][]float64, len(it.points))
	gaps := it.gaps()
	found := false
	for i, pts := range it.points {
		vals[i] = track.SegmentValues(pts, m)
		found = found || vals[i] != nil
		if vals[i] != nil && gaps != nil {
			for j, gap := range gaps[i] {
				if gap {
					vals[i][j] = math.NaN()
				}
			}
		}
	}
	if !found {
		vals = nil
	}
	if it.values == nil {
		it.values = map[track.Metric][][]float64{}
	}
	it.values[m] = vals
	return vals
}

// gaps returns the gaps in the recording, see [track.Gaps].
func (it *fileItem) gaps() [][]bool {
	if !it.gapsDone {
		it.gapsDone = true
		it.gapCache = track.Gaps(it.points)
	}
	return it.gapCache
}

// distances returns the distance along the item for each point, continuing over all lines, in meters.
func (it *fileItem) distances() [][]float64 {
	if it.dist != nil || len(it.points) == 0 {
		return it.dist
	}
	it.dist = make([][]float64, len(it.points))
	var start float64
	for i, pts := range it.points {
		d := track.CumulativeDistance(pts)
		for j := range d {
			d[j] += start
		}
		if len(d) > 0 {
			start = d[len(d)-1]
		}
		it.dist[i] = d
	}
	return it.dist
}

// times returns the time since the first point for each point of the item, in seconds,
// and the time of the first point. It returns nil if any point has no time.
// Times are made non-decreasing, so that they can be searched like distances.
func (it *fileItem) times() ([][]float64, time.Time) {
	if it.timesDone {
		return it.timeCache, it.start
	}
	it.timesDone = true
	var start time.Time
	var last float64
	times := make([][]float64, len(it.points))
	for i, pts := range it.points {
		times[i] = make([]float64, len(pts))
		for j, p := range pts {
			if p.Time.IsZero() {
				return nil, time.Time{}
			}
			if start.IsZero() {
				start = p.Time
			}
			last = math.Max(last, p.Time.Sub(start).Seconds())
			times[i][j] = last
		}
	}
	if start.IsZero() {
		return nil, time.Time{}
	}
	it.timeCache, it.start = times, start
	return times, start
}

// chartable reports whether the item has lines to show in the chart.
func (it *fileItem) chartable() bool {
	return len(it.lines) > 0
}

// firstChartable returns the first item of the file that can be shown in the chart, or nil.
func (f *openFile) firstChartable() *fileItem {
	for _, it := range f.items {
		if it.chartable() {
			return it
		}
	}
	return nil
}

// colors returns the distinct colors of the items, in item order.
func (f *openFile) colors() []color.NRGBA {
	var cols []color.NRGBA
	for _, it := range f.items {
		if !slices.Contains(cols, it.color) {
			cols = append(cols, it.color)
		}
	}
	return cols
}

// setColors sets the item colors from saved colors, if there is one per item,
// or otherwise all to the given default.
func (f *openFile) setColors(saved []string, def color.NRGBA) {
	f.defColor = def
	ok := len(saved) == len(f.items)
	cols := make([]color.NRGBA, len(f.items))
	for i := range cols {
		if !ok {
			break
		}
		c, err := parseColor(saved[i])
		if err != nil {
			log.Printf("%s: %v", f.path, err)
			ok = false
		}
		cols[i] = c
	}
	for i, it := range f.items {
		if ok {
			it.color = cols[i]
		} else {
			it.color = def
		}
	}
}

// expandable reports whether the file has items to show below it.
func (f *openFile) expandable() bool {
	return len(f.items) > 1
}

func itemName(name, kind string, i, n int) string {
	if name != "" {
		return name
	}
	if n == 1 {
		return kind
	}
	return fmt.Sprintf("%s %d", kind, i+1)
}

// trackSummary returns length, duration and sampling interval, like "48.7 km · 4:50 h · 1 s".
func trackSummary(t *track.Track) string {
	parts := []string{formatDistance(t.Length())}
	if start, end := t.TimeSpan(); !start.IsZero() {
		parts = append(parts, formatDuration(end.Sub(start)))
	}
	if dt := t.Interval(); dt > 0 {
		parts = append(parts, fmt.Sprintf("%g s", dt.Round(100*time.Millisecond).Seconds()))
	}
	return strings.Join(parts, " · ")
}

func formatDistance(m float64) string {
	if m < 1000 {
		return fmt.Sprintf("%.0f m", m)
	}
	return fmt.Sprintf("%.1f km", m/1000)
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	if d < time.Hour {
		return fmt.Sprintf("%d min", int(d.Minutes()))
	}
	return fmt.Sprintf("%d:%02d h", int(d.Hours()), int(d.Minutes())%60)
}

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// fitRect pads r for showing it on the map, with a minimum size for tiny or single-point extents.
func fitRect(r geo.Rect) geo.Rect {
	pad := math.Max(math.Max(r.Max.X-r.Min.X, r.Max.Y-r.Min.Y)*0.05, minFitSize/2)
	return geo.Rect{
		Min: geo.Point{X: r.Min.X - pad, Y: r.Min.Y - pad},
		Max: geo.Point{X: r.Max.X + pad, Y: r.Max.Y + pad},
	}
}

// loadBatch is a set of files read in the background.
type loadBatch struct {
	id      int
	fit     bool
	results []loadResult
}

type loadResult struct {
	settings.File
	file *track.File
	err  error
}

// openFiles reads the files in the background and adds them to the panel when done,
// in the given order. If fit is set, the map then shows them.
// Safe for concurrent use.
func (a *App) openFiles(files []settings.File, fit bool) {
	if len(files) == 0 {
		return
	}
	for i := range files {
		if abs, err := filepath.Abs(files[i].Path); err == nil {
			files[i].Path = abs
		}
	}
	a.bgMu.Lock()
	id := a.nextBatch
	a.nextBatch++
	a.pending[id] = files
	a.bgMu.Unlock()

	go func() {
		b := loadBatch{id: id, fit: fit}
		var failed []string
		for _, f := range files {
			data, err := track.ReadFile(f.Path)
			if err != nil {
				log.Printf("opening track: %v", err)
				failed = append(failed, err.Error())
			}
			b.results = append(b.results, loadResult{File: f, file: data, err: err})
		}
		a.bgMu.Lock()
		a.loaded = append(a.loaded, b)
		a.bgMu.Unlock()
		a.invalidate()

		// Missing files on restore are only logged.
		if fit && len(failed) > 0 {
			_ = zenity.Error(strings.Join(failed, "\n"), zenity.Title("Opening files failed"))
		}
	}()
}

// addLoaded adds files read in the background to the panel.
func (a *App) addLoaded() {
	a.bgMu.Lock()
	batches := a.loaded
	a.loaded = nil
	for _, b := range batches {
		delete(a.pending, b.id)
	}
	a.bgMu.Unlock()

	for _, b := range batches {
		fit := geo.EmptyRect()
		for _, r := range b.results {
			if r.err != nil {
				continue
			}
			i := slices.IndexFunc(a.files, func(f *openFile) bool { return f.path == r.Path })
			if i < 0 {
				f := newOpenFile(r.Path, r.file)
				f.visible.Value = !r.Hidden
				f.setColors(r.Colors, trackColors[a.nextColor%len(trackColors)])
				f.setOrder(r.Order)
				if len(r.Colors) == 0 {
					a.nextColor++
				}
				a.files = append(a.files, f)
				i = len(a.files) - 1
				if a.selected == nil {
					a.selected = f.firstChartable()
				}
			}
			fit = fit.Union(a.files[i].bounds)
		}
		if b.fit && !fit.Empty() {
			a.mapView.Fit(fitRect(fit))
		}
		a.tracksChanged = true
	}
}

// showOpenDialog lets the user choose files to open, without blocking the UI.
func (a *App) showOpenDialog() {
	if !a.dialogOpen.CompareAndSwap(false, true) {
		return
	}
	opts := append([]zenity.Option{
		zenity.Title("Open tracks"),
		zenity.FileFilters{
			{Name: "GPX files", Patterns: []string{"*.gpx"}, CaseFold: true},
			{Name: "All files", Patterns: []string{"*"}},
		},
	}, a.plat.dialogOptions()...)
	go func() {
		defer a.dialogOpen.Store(false)
		paths, err := zenity.SelectFileMultiple(opts...)
		if err != nil {
			if !errors.Is(err, zenity.ErrCanceled) {
				log.Printf("open dialog: %v", err)
			}
			return
		}
		files := make([]settings.File, len(paths))
		for i, p := range paths {
			files[i].Path = p
		}
		a.openFiles(files, true)
	}()
}

// fileState returns the opened files for saving, including those still being read.
func (a *App) fileState() []settings.File {
	var files []settings.File
	for _, f := range a.files {
		cols := make([]string, len(f.items))
		for _, it := range f.items {
			cols[it.index] = formatColor(it.color)
		}
		files = append(files, settings.File{Path: f.path, Hidden: !f.visible.Value, Colors: cols, Order: f.order()})
	}
	a.bgMu.Lock()
	defer a.bgMu.Unlock()
	ids := make([]int, 0, len(a.pending))
	for id := range a.pending {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		for _, f := range a.pending[id] {
			if !slices.ContainsFunc(files, func(o settings.File) bool { return o.Path == f.Path }) {
				files = append(files, f)
			}
		}
	}
	return files
}

// updateTracks shows the visible items on the map, colored by the selected metric,
// and the selected item in the chart.
func (a *App) updateTracks() {
	items := a.shownItems()
	groups := a.lineGroups(items)
	a.legend = a.newLegend(groups)
	// While the chart is zoomed in, the selected item is muted, and the part visible in the chart
	// is drawn on top, see [App.updateHighlight]. Its values still count for the color range.
	for i, it := range items {
		if a.chart.zoomedOn(it) {
			groups[i].Color, groups[i].Values = a.style.MutedTrack, nil
		}
	}
	var c *mapview.Coloring
	if a.legend != nil {
		c = a.legend.coloring()
	}
	a.tracks.Set(groups, c)
	a.updateChart()
}

// lineWidth returns the width of the item's lines on the map.
func (a *App) lineWidth(it *fileItem) unit.Dp {
	if it.kind == routeItem {
		return a.routeWidth
	}
	return a.trackWidth
}

// shown reports whether the item is visible on the map.
func (a *App) shown(it *fileItem) bool {
	return slices.Contains(a.shownItems(), it)
}

// highlightKey identifies the content of the map highlight.
type highlightKey struct {
	item         *fileItem
	chartVersion int
	from, to     float64
}

// updateHighlight shows the part of the selected item that is visible in the zoomed chart on the map,
// colored like the map. It is only rebuilt when the shown range or the chart content changes.
func (a *App) updateHighlight() {
	c := &a.chart
	var key highlightKey
	if it := a.selected; c.zoomedOn(it) && a.shown(it) {
		key = highlightKey{item: it, chartVersion: c.version, from: c.from, to: c.to}
	}
	if key == a.highlightKey {
		return
	}
	a.highlightKey = key
	it := key.item
	if it == nil {
		a.highlight.Set(nil, nil)
		return
	}
	var coloring *mapview.Coloring
	if a.legend != nil {
		coloring = a.legend.coloring()
	}
	lines, vals := rangeLines(it.lines, c.xs, it.lineValues(a.colorMetric()), key.from, key.to)
	a.highlight.Set([]mapview.LineGroup{{
		Color:  it.color,
		Lines:  lines,
		Width:  a.lineWidth(it),
		Values: vals,
	}}, coloring)
}

// rangeLines returns the parts of the lines within a range of the chart's horizontal axis,
// given the non-decreasing position of each point on it, see [chartData.xs],
// and the values of their segments if vals is not nil. The parts start and end exactly at the range limits.
func rangeLines(lines []mapview.Polyline, dist, vals [][]float64, from, to float64) ([]mapview.Polyline, [][]float64) {
	var outLines []mapview.Polyline
	var outVals [][]float64
	for i, l := range lines {
		d := dist[i]
		if len(d) < 2 || d[len(d)-1] <= from || d[0] >= to {
			continue
		}
		// Segments j0 to j1-1 overlap the range.
		j0 := max(0, sort.SearchFloat64s(d, from)-1)
		j1 := min(len(d)-1, sort.SearchFloat64s(d, to))
		at := func(j int, x float64) geo.Point {
			a, b := l.Points[j], l.Points[j+1]
			t := 0.0
			if d[j+1] > d[j] {
				t = math.Min(math.Max((x-d[j])/(d[j+1]-d[j]), 0), 1)
			}
			return geo.Point{X: a.X + (b.X-a.X)*t, Y: a.Y + (b.Y-a.Y)*t}
		}
		pts := make([]geo.Point, 0, j1-j0+1)
		pts = append(pts, at(j0, from))
		pts = append(pts, l.Points[j0+1:j1]...)
		pts = append(pts, at(j1-1, to))
		outLines = append(outLines, mapview.NewPolyline(pts))
		if vals != nil {
			var v []float64
			if vals[i] != nil {
				v = vals[i][j0:j1]
			}
			outVals = append(outVals, v)
		}
	}
	return outLines, outVals
}

// trackGroups returns the visible items in drawing order:
// from the bottom of the panel to the top, so that the top-most entry is on top.
func (a *App) trackGroups() []mapview.LineGroup {
	return a.lineGroups(a.shownItems())
}

// shownItems returns the visible items in drawing order, see [App.trackGroups].
func (a *App) shownItems() []*fileItem {
	var items []*fileItem
	for _, f := range slices.Backward(a.files) {
		if !f.visible.Value {
			continue
		}
		for _, it := range slices.Backward(f.items) {
			if it.visible.Value {
				items = append(items, it)
			}
		}
	}
	return items
}

// lineGroups returns the map drawing of the items, colored by the selected metric.
func (a *App) lineGroups(items []*fileItem) []mapview.LineGroup {
	groups := make([]mapview.LineGroup, len(items))
	for i, it := range items {
		groups[i] = mapview.LineGroup{
			Color:   it.color,
			Lines:   it.lines,
			Dots:    it.dots,
			Width:   a.lineWidth(it),
			DotSize: a.waypointSize,
			Values:  it.lineValues(a.colorMetric()),
		}
	}
	return groups
}

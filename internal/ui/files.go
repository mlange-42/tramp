package ui

import (
	"errors"
	"fmt"
	"image/color"
	"log"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"time"

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
	path    string
	name    string
	summary string
	items   []*fileItem
	bounds  geo.Rect

	visible  widget.Bool
	zoom     widget.Clickable
	close    widget.Clickable
	expand   widget.Clickable
	colorBtn widget.Clickable
	expanded bool
	drag     rowDrag
}

// fileItem is a track, a route, or the waypoints of a file.
type fileItem struct {
	name    string
	summary string
	lines   []mapview.Polyline
	dots    []geo.Point
	bounds  geo.Rect
	color   color.NRGBA
	// index is the position of the item in the file, independent of the panel order.
	index int

	visible  widget.Bool
	zoom     widget.Clickable
	colorBtn widget.Clickable
	drag     rowDrag
}

// newOpenFile prepares a read file for the panel and the map.
func newOpenFile(path string, f *track.File) *openFile {
	of := &openFile{path: path, name: filepath.Base(path), bounds: geo.EmptyRect()}
	of.visible.Value = true

	for i := range f.Tracks {
		t := &f.Tracks[i]
		it := &fileItem{name: itemName(t.Name, "Track", i, len(f.Tracks)), summary: trackSummary(t)}
		for j := range t.Segments {
			pts := make([]geo.Point, t.Segments[j].Len())
			for k, p := range t.Segments[j].Points {
				pts[k] = geo.ToMercator(p.Pos)
			}
			it.lines = append(it.lines, mapview.NewPolyline(pts))
		}
		of.add(it)
	}
	for i := range f.Routes {
		r := &f.Routes[i]
		it := &fileItem{name: itemName(r.Name, "Route", i, len(f.Routes)), summary: "route · " + formatDistance(r.Length())}
		pts := make([]geo.Point, len(r.Points))
		for k, p := range r.Points {
			pts[k] = geo.ToMercator(p.Pos)
		}
		it.lines = append(it.lines, mapview.NewPolyline(pts))
		of.add(it)
	}
	if len(f.Waypoints) > 0 {
		it := &fileItem{name: "Waypoints", summary: count(len(f.Waypoints), "waypoint")}
		for _, w := range f.Waypoints {
			it.dots = append(it.dots, geo.ToMercator(w.Pos))
		}
		of.add(it)
	}

	switch len(of.items) {
	case 0:
		of.summary = "empty"
	case 1:
		of.summary = of.items[0].summary
	default:
		var parts []string
		if n := len(f.Tracks); n > 0 {
			parts = append(parts, count(n, "track"))
		}
		if n := len(f.Routes); n > 0 {
			parts = append(parts, count(n, "route"))
		}
		if n := len(f.Waypoints); n > 0 {
			parts = append(parts, count(n, "waypoint"))
		}
		of.summary = strings.Join(parts, " · ")
	}
	return of
}

func (f *openFile) add(it *fileItem) {
	it.visible.Value = true
	it.bounds = geo.EmptyRect()
	for _, l := range it.lines {
		it.bounds = it.bounds.Union(l.Bounds)
	}
	for _, p := range it.dots {
		it.bounds = it.bounds.Extend(p)
	}
	f.bounds = f.bounds.Union(it.bounds)
	it.index = len(f.items)
	f.items = append(f.items, it)
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

// updateTracks shows the visible items on the map.
func (a *App) updateTracks() {
	a.tracks.Set(a.trackGroups())
}

// trackGroups returns the visible items in drawing order:
// from the bottom of the panel to the top, so that the top-most entry is on top.
func (a *App) trackGroups() []mapview.LineGroup {
	var groups []mapview.LineGroup
	for _, f := range slices.Backward(a.files) {
		if !f.visible.Value {
			continue
		}
		for _, it := range slices.Backward(f.items) {
			if it.visible.Value {
				groups = append(groups, mapview.LineGroup{Color: it.color, Lines: it.lines, Dots: it.dots})
			}
		}
	}
	return groups
}

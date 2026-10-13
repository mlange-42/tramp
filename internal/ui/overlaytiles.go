package ui

import (
	"slices"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/tiles"
	"github.com/mlange-42/tramp/internal/wms"
)

// timeKeep is the number of times of an overlay whose tiles are kept in memory, besides the shown one.
// The times loaded in the background are kept in any case. See [App.updateCapacity] for their memory.
const timeKeep = 12

// timeTiles are the tiles of an overlay for a time that is not shown, kept for showing it again,
// or loaded in the background for the next step of the time control.
type timeTiles struct {
	// value is the TIME parameter, "" for the server's default.
	value   string
	tiles   *tiles.Manager
	fetcher *tiles.WMSFetcher
	// used is when the time was last shown.
	used time.Time
}

// showTime shows the tiles of an overlay for a TIME value, kept ones if available.
// The tiles of the previous time are drawn where the new ones are not loaded yet.
// With keep, they are kept for showing them again, otherwise dropped.
func (a *App) showTime(o *overlayState, value string, keep bool) {
	if value == o.timeValue {
		return
	}
	old := o.tiles
	if keep {
		o.others = append(o.others, &timeTiles{value: o.timeValue, tiles: old, fetcher: o.fetcher, used: time.Now()})
		o.fallback = old
	} else {
		go old.Close()
		o.fallback = nil
	}
	o.timeValue = value
	if k := slices.IndexFunc(o.others, func(t *timeTiles) bool { return t.value == value }); k >= 0 {
		o.tiles, o.fetcher = o.others[k].tiles, o.others[k].fetcher
		o.others = slices.Delete(o.others, k, k+1)
	} else {
		t := a.newOverlayTiles(o, value)
		o.tiles, o.fetcher = t.tiles, t.fetcher
	}
	a.pruneTimes(o)
	a.updateCapacity()
}

// newOverlayTiles creates the tiles of an overlay for a TIME value.
func (a *App) newOverlayTiles(o *overlayState, value string) *timeTiles {
	layer := o.Layer
	layer.Transparent = true
	layer.Time = value
	m, f := a.newManager(&layer, o.Shade, o.Refresh > 0)
	f.SetReferences(o.references(value))
	return &timeTiles{value: value, tiles: m, fetcher: f}
}

// referenceTries is the number of reference times tried for a time, see [overlayState.references].
const referenceTries = 4

// references returns the reference times to request the data of a TIME value for: the latest ones
// not after the time, the latest first. A model run covers only times after its start, and some data,
// like sums over hours, only from later. The server's default is the latest run, which fails for earlier times.
func (o *overlayState) references(value string) []string {
	refs := o.times.refs
	if refs == nil || value == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	var values []string
	for _, r := range refs.Latest(t, referenceTries) {
		values = append(values, wms.FormatTime(r))
	}
	return values
}

// pruneTimes drops the least recently shown tiles of other times beyond [timeKeep],
// except for those loaded in the background.
func (a *App) pruneTimes(o *overlayState) {
	var kept []*timeTiles
	for _, t := range o.others {
		if !slices.Contains(o.prefetch, t.value) {
			kept = append(kept, t)
		}
	}
	slices.SortFunc(kept, func(x, y *timeTiles) int { return x.used.Compare(y.used) })
	for len(kept) > timeKeep {
		a.dropTime(o, kept[0])
		kept = kept[1:]
	}
}

// dropTime closes the kept tiles of another time.
func (a *App) dropTime(o *overlayState, t *timeTiles) {
	o.others = slices.DeleteFunc(o.others, func(x *timeTiles) bool { return x == t })
	if o.fallback == t.tiles {
		o.fallback = nil
	}
	go t.tiles.Close()
}

// dropOtherTimes closes the tiles of all times not shown, like when the data changed.
func (a *App) dropOtherTimes(o *overlayState) {
	for len(o.others) > 0 {
		a.dropTime(o, o.others[0])
	}
	a.updateCapacity()
}

// closeOverlayTiles closes all tiles of an overlay, for disabling it.
func (o *overlayState) closeOverlayTiles() {
	for _, m := range o.allTiles() {
		go m.Close()
	}
	o.tiles, o.fetcher, o.others, o.fallback, o.prefetch, o.timeValue = nil, nil, nil, nil, nil, ""
}

// prefetchTimes loads the tiles of the neighboring times of the overlays in the background,
// for the tiles shown, once these are loaded. Meanwhile, the background loading pauses.
// The tiles of the previous time are no longer drawn below once the shown ones are loaded.
// The tiles of other times stop loading, as do those of hidden overlays.
func (a *App) prefetchTimes() {
	for i := range a.overlays {
		o := &a.overlays[i]
		if o.tiles == nil {
			continue
		}
		hidden := o.times.hidden()
		o.tiles.SetBackground(false)
		if hidden {
			// Not drawn, so the map doesn't request its tiles.
			o.tiles.Request(nil)
		}
		busy := o.tiles.Loading() > 0
		var wanted []geo.TileKey
		if !busy && !hidden {
			o.fallback = nil
			wanted = o.tiles.Wanted()
			for _, value := range o.prefetch {
				if !slices.ContainsFunc(o.others, func(t *timeTiles) bool { return t.value == value }) {
					o.others = append(o.others, a.newOverlayTiles(o, value))
					a.updateCapacity()
				}
			}
		}
		for _, t := range o.others {
			if hidden || !slices.Contains(o.prefetch, t.value) {
				t.tiles.Request(nil)
				continue
			}
			t.tiles.SetBackground(true)
			t.tiles.Request(wanted)
		}
	}
}

// allTiles returns the tile managers of an overlay: of the shown time and of the kept times.
func (o *overlayState) allTiles() []*tiles.Manager {
	var ms []*tiles.Manager
	if o.tiles != nil {
		ms = append(ms, o.tiles)
	}
	for _, t := range o.others {
		ms = append(ms, t.tiles)
	}
	return ms
}

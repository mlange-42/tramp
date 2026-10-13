package ui

import (
	"slices"
	"time"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/tiles"
)

// timeKeep is the number of times of an overlay whose tiles are kept in memory, besides the shown one.
// The times loaded in the background are kept in any case.
const timeKeep = 4

// timeTiles are the tiles of an overlay for a time that is not shown, kept for showing it again,
// or loaded in the background for the next step of the time control.
type timeTiles struct {
	// value is the TIME parameter, "" for the server's default.
	value string
	tiles *tiles.Manager
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
		o.others = append(o.others, &timeTiles{value: o.timeValue, tiles: old, used: time.Now()})
		o.fallback = old
	} else {
		go old.Close()
		o.fallback = nil
	}
	o.tiles, o.timeValue = nil, value
	if k := slices.IndexFunc(o.others, func(t *timeTiles) bool { return t.value == value }); k >= 0 {
		o.tiles = o.others[k].tiles
		o.others = slices.Delete(o.others, k, k+1)
	} else {
		o.tiles = a.newOverlayTiles(o, value)
	}
	a.pruneTimes(o)
	a.updateCapacity()
}

// newOverlayTiles creates the tile manager of an overlay for a TIME value.
func (a *App) newOverlayTiles(o *overlayState, value string) *tiles.Manager {
	layer := o.Layer
	layer.Transparent = true
	layer.Time = value
	m, _ := a.newManager(&layer, o.Shade, o.Refresh > 0)
	return m
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
	o.tiles, o.others, o.fallback, o.prefetch, o.timeValue = nil, nil, nil, nil, ""
}

// prefetchTimes loads the tiles of the neighboring times of the overlays in the background,
// for the tiles shown, once these are loaded. Meanwhile, the background loading pauses.
// The tiles of the previous time are no longer drawn below once the shown ones are loaded.
func (a *App) prefetchTimes() {
	for i := range a.overlays {
		o := &a.overlays[i]
		if o.tiles == nil || o.times.hidden() {
			continue
		}
		busy := o.tiles.Loading() > 0
		var wanted []geo.TileKey
		if !busy {
			o.fallback = nil
			wanted = o.tiles.Wanted()
		}
		for _, value := range o.prefetch {
			k := slices.IndexFunc(o.others, func(t *timeTiles) bool { return t.value == value })
			if k < 0 {
				if busy {
					continue
				}
				o.others = append(o.others, &timeTiles{value: value, tiles: a.newOverlayTiles(o, value)})
				k = len(o.others) - 1
				a.updateCapacity()
			}
			o.others[k].tiles.Request(wanted)
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

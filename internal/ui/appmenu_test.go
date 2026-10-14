package ui

import (
	"path/filepath"
	"testing"

	"github.com/mlange-42/tramp/internal/settings"
)

// clearCacheItem is the index of "Clear tile cache" in the Map menu, after the view options and a separator.
const clearCacheItem = int(numViewOptions) + 1

func TestMenuItems(t *testing.T) {
	a := newApp(func() {}, Options{Maps: DefaultMaps()})
	items := a.menuItems()
	if len(items) != len(menuTitles) || len(a.menu.menus) != len(menuTitles) {
		t.Fatalf("%d item lists and %d menus for %d titles", len(items), len(a.menu.menus), len(menuTitles))
	}
	for i, menu := range items {
		for _, it := range menu {
			if it.label != "" && it.tip == "" {
				t.Errorf("%s › %s has no tooltip", menuTitles[i], it.label)
			}
		}
	}
	// Without disk cache, as in tests, its items are disabled.
	for _, it := range items[1][clearCacheItem:] {
		if !it.disabled {
			t.Errorf("%q enabled without disk cache", it.label)
		}
	}

	dir := t.TempDir()
	a = newApp(func() {}, Options{Maps: DefaultMaps(), State: settings.Settings{
		DiskCache: settings.DiskCache{Dir: dir, MaxSize: 1, ElevationMaxSize: 1},
	}})
	if a.diskCache.dir != dir {
		t.Errorf("cache directory %q, want %q", a.diskCache.dir, dir)
	}
	if err := a.diskCache.tiles.Put("a/1.png", make([]byte, 1000)); err != nil {
		t.Fatal(err)
	}
	if err := a.diskCache.elevation.Put("b/1.webp", make([]byte, 500)); err != nil {
		t.Fatal(err)
	}
	a.diskCache.tiles.Wait()
	a.diskCache.elevation.Wait()
	if got := a.diskCache.size(); got != 1500 {
		t.Errorf("size %d, want 1500", got)
	}
	if it := a.menuItems()[1][clearCacheItem]; it.disabled || it.label != "Clear tile cache (1 KB)…" {
		t.Errorf("got %q, disabled %v", it.label, it.disabled)
	}
	if err := a.diskCache.clear(); err != nil {
		t.Fatal(err)
	}
	a.diskCache.tiles.Wait()
	a.diskCache.elevation.Wait()
	if a.diskCache.size() != 0 {
		t.Error("cache not empty after clearing")
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "*", "*")); len(matches) != 0 {
		t.Errorf("files left after clearing: %v", matches)
	}
}

func TestFormatBytes(t *testing.T) {
	for n, want := range map[int64]string{0: "0 KB", 358400: "350 KB", 4404019: "4.2 MB", 851443712: "812 MB", 5 << 30: "5120 MB"} {
		if got := formatBytes(n); got != want {
			t.Errorf("formatBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

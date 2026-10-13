package ui

import (
	"errors"
	"log"
	"path/filepath"
	"time"

	"github.com/mlange-42/tramp/internal/diskcache"
	"github.com/mlange-42/tramp/internal/settings"
)

// Directories of the map and elevation tiles in the cache directory.
const (
	tilesDir     = "tiles"
	elevationDir = "elevation"
)

// diskCacheView keeps downloaded map and elevation tiles on disk.
type diskCacheView struct {
	cfg settings.DiskCache
	// dir is the directory of the caches.
	dir string
	// tiles is the cache of the map tiles, and elevation that of the elevation tiles. Nil if turned off.
	tiles, elevation *diskcache.Cache
}

// newDiskCache opens the caches of map and elevation tiles. A size limit of 0 or less turns a cache off,
// as in tests, unlike in the settings file where 0 is the default.
func newDiskCache(cfg settings.DiskCache) diskCacheView {
	v := diskCacheView{cfg: cfg}
	if cfg.MaxSize <= 0 && cfg.ElevationMaxSize <= 0 {
		return v
	}
	dir := cfg.Dir
	if dir == "" {
		var err error
		if dir, err = settings.CacheDir(); err != nil {
			log.Printf("tile cache: %v", err)
			return v
		}
	}
	v.dir = dir
	v.tiles = diskcache.New(filepath.Join(dir, tilesDir), int64(cfg.MaxSize)<<20)
	v.elevation = diskcache.New(filepath.Join(dir, elevationDir), int64(cfg.ElevationMaxSize)<<20)
	return v
}

// maxAge returns the age after which a cached tile is checked with the server.
func (v *diskCacheView) maxAge() time.Duration {
	days := v.cfg.MaxAge
	if days <= 0 {
		days = settings.DefaultDiskCacheAge
	}
	return time.Duration(days) * 24 * time.Hour
}

// size returns the total size of the stored map and elevation tiles in bytes.
func (v *diskCacheView) size() int64 {
	return v.tiles.Size() + v.elevation.Size()
}

// clear deletes the stored map and elevation tiles.
func (v *diskCacheView) clear() error {
	return errors.Join(v.tiles.Clear(), v.elevation.Clear())
}

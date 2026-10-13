package ui

import (
	"log"
	"path/filepath"
	"time"

	"github.com/mlange-42/tramp/internal/diskcache"
	"github.com/mlange-42/tramp/internal/settings"
)

// tilesDir is the directory of the map tiles in the cache directory.
const tilesDir = "tiles"

// diskCacheView keeps downloaded map tiles on disk.
type diskCacheView struct {
	cfg settings.DiskCache
	// tiles is the cache of the map tiles. Nil if turned off.
	tiles *diskcache.Cache
}

// newDiskCache opens the cache of map tiles. Its size limit 0 or less turns it off,
// as in tests, unlike in the settings file where 0 is the default.
func newDiskCache(cfg settings.DiskCache) diskCacheView {
	v := diskCacheView{cfg: cfg}
	if cfg.MaxSize <= 0 {
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
	v.tiles = diskcache.New(filepath.Join(dir, tilesDir), int64(cfg.MaxSize)<<20)
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

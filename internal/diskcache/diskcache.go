// Package diskcache keeps downloaded files, like map tiles, in a directory with a size limit.
//
// Entries are files addressed by keys like "osm-standard-3f2a91c0/14/8712/5520.png".
// The modification time of a file is when it was downloaded or last confirmed by the server.
// When the files exceed the size limit, the oldest ones are deleted.
package diskcache

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// cleanTarget is the share of the size limit the cache is reduced to when it exceeds the limit,
// so that cleaning up doesn't start again with the next file.
const cleanTarget = 0.9

// tempPattern names the files being written. They are deleted by the scan if left over from a crash.
const tempPattern = "*.tmp"

// oldInfix is in the names of directories removed by [Cache.Clear] while they are deleted.
const oldInfix = ".old-"

// Cache stores files in a directory, and deletes the oldest ones when they exceed a size limit.
// A nil Cache stores nothing. It is safe for concurrent use.
type Cache struct {
	dir     string
	maxSize int64

	mu sync.Mutex
	// size is the total size of the files.
	size int64
	// scanning is whether a scan of the directory is running. Only one runs at a time.
	scanning bool
	// added is the size added since the running scan started.
	added int64
	// gen counts the calls of Clear, so that a scan started before doesn't overwrite the size.
	gen int
	wg  sync.WaitGroup
}

// New returns a cache in dir with a size limit in bytes. It returns nil if maxSize is 0 or less.
// It scans the directory in the background for the size of the files, and deletes files
// left over from a crash or from an interrupted [Cache.Clear].
func New(dir string, maxSize int64) *Cache {
	if maxSize <= 0 {
		return nil
	}
	c := &Cache{dir: dir, maxSize: maxSize}
	c.removeOld()
	c.startScan()
	return c
}

// Dir returns the directory of the cache.
func (c *Cache) Dir() string {
	if c == nil {
		return ""
	}
	return c.dir
}

// Size returns the total size of the files in bytes. It may be too low while the first scan is running.
func (c *Cache) Size() int64 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.size
}

// Get returns the content of the file with the key, and when it was stored or last touched.
// ok is false if there is no such file.
func (c *Cache) Get(key string) (data []byte, modTime time.Time, ok bool) {
	path, err := c.path(key)
	if err != nil {
		return nil, time.Time{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, time.Time{}, false
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, time.Time{}, false
	}
	data, err = io.ReadAll(f)
	if err != nil {
		return nil, time.Time{}, false
	}
	return data, info.ModTime(), true
}

// Put stores the data under the key, replacing an existing file.
// Empty data is stored as an empty file, e.g. to remember that a server has no such tile.
func (c *Cache) Put(key string, data []byte) error {
	path, err := c.path(key)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// Written to a temporary file first, so that readers never see half a file.
	tmp, err := os.CreateTemp(dir, tempPattern)
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	var old int64
	if info, err := os.Stat(path); err == nil {
		old = info.Size()
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	c.add(int64(len(data)) - old)
	return nil
}

// Touch sets the modification time of the file with the key to now,
// e.g. when the server confirmed that it is unchanged.
func (c *Cache) Touch(key string) error {
	path, err := c.path(key)
	if err != nil {
		return err
	}
	now := time.Now()
	return os.Chtimes(path, now, now)
}

// Clear deletes all files. The directory is renamed first and deleted in the background,
// so that the cache is empty at once, and files stored meanwhile go to a new directory.
func (c *Cache) Clear() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	old := c.dir + oldInfix + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.Rename(c.dir, old); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	c.size = 0
	c.added = 0
	c.gen++
	c.wg.Go(func() { _ = os.RemoveAll(old) })
	return nil
}

// Wait waits for running scans and deletions in the background.
func (c *Cache) Wait() {
	if c != nil {
		c.wg.Wait()
	}
}

// path returns the file path of a key. Keys use slashes, and must not leave the directory.
func (c *Cache) path(key string) (string, error) {
	if c == nil {
		return "", errors.New("diskcache: no cache")
	}
	if !filepath.IsLocal(key) || strings.HasSuffix(key, ".tmp") {
		return "", fmt.Errorf("diskcache: invalid key %q", key)
	}
	return filepath.Join(c.dir, filepath.FromSlash(key)), nil
}

// add adds to the total size, and starts cleaning up if the limit is exceeded.
func (c *Cache) add(n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.size += n
	if c.scanning {
		c.added += n
		return
	}
	if c.size > c.maxSize {
		c.startScanLocked()
	}
}

func (c *Cache) startScan() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.startScanLocked()
}

// startScanLocked starts a scan in the background unless one is running. The caller must hold the lock.
func (c *Cache) startScanLocked() {
	if c.scanning {
		return
	}
	c.scanning = true
	c.added = 0
	gen := c.gen
	c.wg.Go(func() { c.scan(gen) })
}

// file is a file found by a scan.
type file struct {
	path    string
	size    int64
	modTime time.Time
}

// scan determines the total size of the files, and deletes the oldest ones if it exceeds the limit.
func (c *Cache) scan(gen int) {
	var files []file
	var total int64
	_ = filepath.WalkDir(c.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if ok, _ := filepath.Match(tempPattern, d.Name()); ok {
			// Left over from a crash, unless it is being written right now.
			if time.Since(info.ModTime()) > time.Hour {
				_ = os.Remove(path)
			}
			return nil
		}
		files = append(files, file{path: path, size: info.Size(), modTime: info.ModTime()})
		total += info.Size()
		return nil
	})

	if total > c.maxSize {
		slices.SortFunc(files, func(a, b file) int { return a.modTime.Compare(b.modTime) })
		target := int64(float64(c.maxSize) * cleanTarget)
		for _, f := range files {
			if total <= target {
				break
			}
			if err := os.Remove(f.path); err == nil || errors.Is(err, fs.ErrNotExist) {
				total -= f.size
			}
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.scanning = false
	if c.gen != gen {
		// Cleared meanwhile. The size counts the files stored since.
		return
	}
	// Files stored during the scan may be counted twice, which errs on the safe side.
	c.size = total + c.added
	if c.size > c.maxSize && c.added > 0 {
		c.startScanLocked()
	}
}

// removeOld deletes directories left over from an interrupted [Cache.Clear].
func (c *Cache) removeOld() {
	old, _ := filepath.Glob(c.dir + oldInfix + "*")
	for _, dir := range old {
		c.wg.Go(func() { _ = os.RemoveAll(dir) })
	}
}

// Name returns a directory name for a data source, like "osm-standard-3f2a91c0":
// a readable part from name, and a hash of identity, the properties that determine the content,
// like the URL. When these change, the name changes, and old files are not used any more.
func Name(name string, identity ...string) string {
	h := sha256.Sum256([]byte(strings.Join(identity, "\x00")))
	hash := hex.EncodeToString(h[:4])
	if slug := slugify(name); slug != "" {
		return slug + "-" + hash
	}
	return hash
}

// maxSlug is the maximum length of the readable part of a [Name].
const maxSlug = 40

// slugify returns the lower-case ASCII letters and digits of s, with dashes for other characters between them.
func slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
			if b.Len() >= maxSlug {
				break
			}
		} else {
			dash = true
		}
	}
	return b.String()
}

// ImageExt returns the file extension of the images of a URL template like "https://a/{z}/{x}/{y}.png",
// or "" if it doesn't end with that of a common image format.
func ImageExt(urlTemplate string) string {
	u, _, _ := strings.Cut(urlTemplate, "?")
	switch ext := strings.ToLower(path.Ext(u)); ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		return ext
	}
	return ""
}

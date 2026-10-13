package diskcache

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPutGet(t *testing.T) {
	c := New(t.TempDir(), 1000)
	c.Wait()

	if _, _, ok := c.Get("a/1/2/3.png"); ok {
		t.Fatal("found a file in an empty cache")
	}
	before := time.Now().Add(-time.Second)
	if err := c.Put("a/1/2/3.png", []byte("tile")); err != nil {
		t.Fatal(err)
	}
	data, mod, ok := c.Get("a/1/2/3.png")
	if !ok || string(data) != "tile" {
		t.Fatalf("got %q, %v", data, ok)
	}
	if mod.Before(before) {
		t.Errorf("modification time %v before %v", mod, before)
	}
	if got := c.Size(); got != 4 {
		t.Errorf("size %d, want 4", got)
	}

	// Replacing counts the difference.
	if err := c.Put("a/1/2/3.png", []byte("bigger tile")); err != nil {
		t.Fatal(err)
	}
	if got := c.Size(); got != 11 {
		t.Errorf("size %d after replacing, want 11", got)
	}

	// Empty files are entries too.
	if err := c.Put("a/1/2/4.png", nil); err != nil {
		t.Fatal(err)
	}
	if data, _, ok := c.Get("a/1/2/4.png"); !ok || len(data) != 0 {
		t.Errorf("got %q, %v for an empty file", data, ok)
	}
}

func TestInvalidKeys(t *testing.T) {
	c := New(t.TempDir(), 1000)
	c.Wait()
	for _, key := range []string{"", "../x", "a/../../x", "/abs", "a/b.tmp"} {
		if err := c.Put(key, []byte("x")); err == nil {
			t.Errorf("Put(%q) succeeded", key)
		}
	}
}

func TestNil(t *testing.T) {
	c := New(t.TempDir(), 0)
	if c != nil {
		t.Fatal("cache without size limit")
	}
	if err := c.Put("a/b", []byte("x")); err == nil {
		t.Error("Put on nil cache succeeded")
	}
	if _, _, ok := c.Get("a/b"); ok {
		t.Error("Get on nil cache found a file")
	}
	if c.Size() != 0 || c.Clear() != nil {
		t.Error("nil cache not empty")
	}
	c.Wait()
}

func TestTouch(t *testing.T) {
	c := New(t.TempDir(), 1000)
	c.Wait()
	if err := c.Put("a/b", []byte("x")); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(filepath.Join(c.Dir(), "a", "b"), old, old); err != nil {
		t.Fatal(err)
	}
	if err := c.Touch("a/b"); err != nil {
		t.Fatal(err)
	}
	if _, mod, _ := c.Get("a/b"); time.Since(mod) > time.Minute {
		t.Errorf("modification time %v not updated", mod)
	}
}

// fill writes files of 100 bytes with increasing modification times into dir, the first one the oldest.
func fill(t *testing.T, dir string, names ...string) {
	t.Helper()
	start := time.Now().Add(-time.Duration(len(names)) * time.Hour)
	for i, name := range names {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, bytes.Repeat([]byte{'x'}, 100), 0o644); err != nil {
			t.Fatal(err)
		}
		mod := start.Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
}

func exists(c *Cache, key string) bool {
	_, _, ok := c.Get(key)
	return ok
}

func TestScan(t *testing.T) {
	dir := t.TempDir()
	fill(t, dir, "a/1", "a/2", "b/1")
	// A temporary file left over from a crash.
	fill(t, dir, "a/x.tmp")
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "a", "x.tmp"), old, old); err != nil {
		t.Fatal(err)
	}

	c := New(dir, 1000)
	c.Wait()
	if got := c.Size(); got != 300 {
		t.Errorf("size %d, want 300", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "a", "x.tmp")); err == nil {
		t.Error("temporary file not deleted")
	}
}

func TestScanEvicts(t *testing.T) {
	dir := t.TempDir()
	fill(t, dir, "a/1", "a/2", "b/1", "b/2", "b/3")

	// Over the limit at the start: reduced to 90% of it, the oldest files first.
	c := New(dir, 350)
	c.Wait()
	if got := c.Size(); got != 300 {
		t.Errorf("size %d, want 300", got)
	}
	if exists(c, "a/1") || exists(c, "a/2") || !exists(c, "b/1") || !exists(c, "b/3") {
		t.Error("wrong files deleted")
	}
}

func TestPutEvicts(t *testing.T) {
	dir := t.TempDir()
	fill(t, dir, "a/1", "a/2", "a/3")
	c := New(dir, 450)
	c.Wait()

	// Exceeds the limit: reduced to at most 405 bytes, so 4 files are kept.
	if err := c.Put("b/1", bytes.Repeat([]byte{'x'}, 100)); err != nil {
		t.Fatal(err)
	}
	if err := c.Put("b/2", bytes.Repeat([]byte{'x'}, 100)); err != nil {
		t.Fatal(err)
	}
	c.Wait()
	if got := c.Size(); got > 405 {
		t.Errorf("size %d after cleaning up, want at most 405", got)
	}
	if exists(c, "a/1") || !exists(c, "b/1") || !exists(c, "b/2") {
		t.Error("wrong files deleted")
	}
}

func TestClear(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "tiles")
	fill(t, dir, "a/1", "a/2")
	// Left over from an interrupted clear.
	fill(t, dir+".old-123", "a/1")

	c := New(dir, 1000)
	c.Wait()
	if _, err := os.Stat(dir + ".old-123"); err == nil {
		t.Error("old directory not deleted")
	}

	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if c.Size() != 0 || exists(c, "a/1") {
		t.Error("cache not empty after clearing")
	}
	if err := c.Put("a/3", []byte("x")); err != nil {
		t.Fatal(err)
	}
	c.Wait()
	if !exists(c, "a/3") || c.Size() != 1 {
		t.Error("file stored after clearing not found")
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("%d entries in the parent directory, want only the cache", len(entries))
	}

	// Clearing a cache without directory works.
	empty := New(filepath.Join(parent, "none"), 1000)
	if err := empty.Clear(); err != nil {
		t.Error(err)
	}
	empty.Wait()
}

func TestName(t *testing.T) {
	a := Name("OSM Standard", "https://tile.openstreetmap.org/{z}/{x}/{y}.png")
	if len(a) != len("osm-standard-")+8 || a[:13] != "osm-standard-" {
		t.Errorf("got %q", a)
	}
	if b := Name("OSM Standard", "https://other/{z}/{x}/{y}.png"); b == a {
		t.Error("same name for another URL")
	}
	if b := Name("OSM Standard", "a", "b"); b == Name("OSM Standard", "ab") {
		t.Error("identity parts not separated")
	}
	if got := Name("  Labels & roads (bright) "); got[:len("labels-roads-bright-")] != "labels-roads-bright-" {
		t.Errorf("got %q", got)
	}
	if got := Name("Äöü"); len(got) != 8 {
		t.Errorf("got %q, want only the hash", got)
	}
}

func TestImageExt(t *testing.T) {
	for url, want := range map[string]string{
		"https://a/{z}/{x}/{y}.png":          ".png",
		"https://a/{z}/{x}/{y}.WebP?key=abc": ".webp",
		"https://a/{z}/{x}/{y}.jpeg":         ".jpeg",
		"https://a/{z}/{x}/{y}":              "",
		"https://a/tile.php?z={z}":           "",
	} {
		if got := ImageExt(url); got != want {
			t.Errorf("ImageExt(%q) = %q, want %q", url, got, want)
		}
	}
}

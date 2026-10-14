package ui

import (
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/mlange-42/tramp/internal/settings"
	"github.com/ncruces/zenity"
)

// uiTester lays out an app in frames, and sends input to it.
type uiTester struct {
	a    *App
	r    input.Router
	ops  op.Ops
	size image.Point
}

// newUITester returns a tester for an app without map tiles and elevations, so that it doesn't download anything.
// disk is the disk cache.
func newUITester(t *testing.T, disk settings.DiskCache) *uiTester {
	a := newApp(func() {}, Options{Maps: DefaultMaps(), State: settings.Settings{
		View:      &settings.View{Map: "None", Zoom: 5},
		Elevation: settings.Elevation{Source: "none"},
		DiskCache: disk,
	}})
	t.Cleanup(a.closeTiles)
	u := &uiTester{a: a, size: image.Pt(1000, 700)}
	u.frame()
	return u
}

func (u *uiTester) frame() {
	u.ops.Reset()
	gtx := layout.Context{Ops: &u.ops, Source: u.r.Source(), Now: time.Now(), Constraints: layout.Exact(u.size)}
	gtx.Metric.PxPerDp, gtx.Metric.PxPerSp = 1, 1
	u.a.layout(gtx)
	u.r.Frame(&u.ops)
}

func (u *uiTester) click(at image.Point) {
	p := f32.Pt(float32(at.X), float32(at.Y))
	u.r.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p},
	)
	u.frame()
	u.frame()
}

// clickItem chooses item j of menu i, by a programmatic click.
func (u *uiTester) clickItem(i, j int) {
	u.a.menu.menus[i].btns[j].Click()
	u.frame()
	u.frame()
}

// header returns the center of the header of menu i in the window.
func (u *uiTester) header(i int) image.Point {
	b := u.a.menu.menus[i].bounds
	// The menu bar is the first thing in the toolbar.
	inset := image.Pt(int(u.a.style.BarInset.Left), int(u.a.style.BarInset.Top))
	return b.Min.Add(b.Size().Div(2)).Add(inset)
}

// stubCommands replaces running commands, and returns the arguments of the commands that would have run.
func stubCommands(t *testing.T) *[][]string {
	var started [][]string
	orig := startCommand
	startCommand = func(cmd *exec.Cmd, _ bool) { started = append(started, cmd.Args) }
	t.Cleanup(func() { startCommand = orig })
	return &started
}

// lastArg returns the last argument of the last command, or "".
func lastArg(started [][]string) string {
	if len(started) == 0 {
		return ""
	}
	args := started[len(started)-1]
	return args[len(args)-1]
}

func TestMenuBar(t *testing.T) {
	started := stubCommands(t)
	u := newUITester(t, settings.DiskCache{})
	m := u.a.menu.menus
	isOpen := func() []bool { return []bool{m[0].open, m[1].open, m[2].open} }

	u.click(u.header(0))
	if !slices.Equal(isOpen(), []bool{true, false, false}) {
		t.Fatalf("expected File open, got %v", isOpen())
	}
	// Clicking another header while a menu is open opens that one.
	u.click(u.header(1))
	if !slices.Equal(isOpen(), []bool{false, true, false}) {
		t.Fatalf("expected Map open, got %v", isOpen())
	}
	// Clicking the header of the open menu closes it.
	u.click(u.header(1))
	if !slices.Equal(isOpen(), []bool{false, false, false}) {
		t.Fatalf("expected all closed, got %v", isOpen())
	}

	// Disabled items do nothing, and the menu stays open.
	u.click(u.header(1))
	u.clickItem(1, clearCacheItem)
	if !m[1].open || u.a.dialogOpen.Load() {
		t.Error("disabled item was chosen")
	}
	// A click outside closes the menu, and doesn't reach the map.
	center := u.a.mapView.View.Center
	u.click(image.Pt(500, 400))
	if m[1].open || u.a.mapView.View.Center != center {
		t.Error("expected the menu closed, and the map unchanged")
	}

	// Choosing an item runs it, and closes the menu.
	u.click(u.header(2))
	u.clickItem(2, 0)
	if m[2].open || lastArg(*started) != docsURL {
		t.Errorf("expected the documentation opened and the menu closed, got %v, %v", m[2].open, *started)
	}
	u.click(u.header(2))
	u.clickItem(2, 1)
	if lastArg(*started) != issuesURL {
		t.Errorf("expected the issues opened, got %v", *started)
	}
	u.click(u.header(2))
	u.clickItem(2, 3)
	if !u.a.sources.open {
		t.Error("expected the data sources open")
	}
	u.click(image.Pt(500, 400))

	// Without a selected route, it can't be saved as track.
	u.click(u.header(0))
	u.clickItem(0, 3)
	if !m[0].open {
		t.Error("disabled item was chosen")
	}
	u.click(image.Pt(500, 400))

	u.click(u.header(0))
	u.clickItem(0, 5)
	dir, err := settings.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if lastArg(*started) != dir {
		t.Errorf("expected the settings folder %s opened, got %v", dir, *started)
	}
	// Quit does nothing without window.
	u.click(u.header(0))
	u.clickItem(0, 7)
}

func TestMenuCache(t *testing.T) {
	started := stubCommands(t)
	dir := t.TempDir()
	u := newUITester(t, settings.DiskCache{Dir: dir, MaxSize: 1, ElevationMaxSize: 1})
	u.a.diskCache.tiles.Wait()
	u.a.diskCache.elevation.Wait()
	if err := u.a.diskCache.tiles.Put("a/1.png", make([]byte, 2000)); err != nil {
		t.Fatal(err)
	}

	u.click(u.header(1))
	u.clickItem(1, clearCacheItem+1)
	if lastArg(*started) != dir {
		t.Errorf("expected the cache folder %s opened, got %v", dir, *started)
	}

	answer := func(err error) {
		question = func(string, ...zenity.Option) error { return err }
		t.Cleanup(func() { question = zenity.Question })
	}
	waitDialog := func() {
		t.Helper()
		for start := time.Now(); u.a.dialogOpen.Load(); {
			if time.Since(start) > 5*time.Second {
				t.Fatal("dialog not finished")
			}
			time.Sleep(time.Millisecond)
		}
	}

	// Canceled: the tiles are kept.
	answer(zenity.ErrCanceled)
	u.click(u.header(1))
	u.clickItem(1, clearCacheItem)
	waitDialog()
	if u.a.diskCache.size() != 2000 {
		t.Errorf("size %d after canceling, want 2000", u.a.diskCache.size())
	}

	// Confirmed: the tiles are deleted.
	answer(nil)
	u.click(u.header(1))
	u.clickItem(1, clearCacheItem)
	waitDialog()
	u.a.diskCache.tiles.Wait()
	if u.a.diskCache.size() != 0 {
		t.Errorf("size %d after clearing, want 0", u.a.diskCache.size())
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "tiles", "*")); len(matches) != 0 {
		t.Errorf("files left after clearing: %v", matches)
	}
}

func TestAbout(t *testing.T) {
	started := stubCommands(t)
	u := newUITester(t, settings.DiskCache{})
	v := &u.a.about
	openAbout := func() {
		t.Helper()
		u.click(u.header(2))
		u.clickItem(2, 5)
		if !v.open {
			t.Fatal("expected the About window open")
		}
	}

	openAbout()
	if v.img.Size() != image.Pt(960, 480) {
		t.Errorf("image size %v", v.img.Size())
	}
	// Clicks in the window keep it open.
	u.click(image.Pt(u.size.X/2, u.size.Y/2))
	if !v.open {
		t.Error("expected the About window open after a click in it")
	}

	v.repo.Click()
	u.frame()
	if lastArg(*started) != repoURL {
		t.Errorf("expected the repository opened, got %v", *started)
	}
	v.mail.Click()
	u.frame()
	if lastArg(*started) != "mailto:"+contactEmail {
		t.Errorf("expected a mail opened, got %v", *started)
	}
	v.sources.Click()
	u.frame()
	if v.open || !u.a.sources.open {
		t.Error("expected the data sources instead of the About window")
	}
	u.click(image.Pt(500, 400))

	// Closed by Esc, the close button and clicks outside.
	openAbout()
	u.r.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	u.frame()
	if v.open {
		t.Error("expected the About window closed by Esc")
	}
	openAbout()
	v.close.btn.Click()
	u.frame()
	if v.open {
		t.Error("expected the About window closed by its button")
	}
	openAbout()
	center := u.a.mapView.View.Center
	u.click(image.Pt(5, u.size.Y/2))
	if v.open || u.a.mapView.View.Center != center {
		t.Error("expected the About window closed by a click outside, and the map unchanged")
	}
}

func TestVersion(t *testing.T) {
	// Tests have no version of the main module.
	if v := Version(); v != "development build" && v[0] != 'v' {
		t.Errorf("unexpected version %q", v)
	}
}

func TestURLCommand(t *testing.T) {
	for goos, want := range map[string][]string{
		"windows": {"rundll32", "url.dll,FileProtocolHandler", "https://x"},
		"darwin":  {"open", "https://x"},
		"linux":   {"xdg-open", "https://x"},
	} {
		if got := urlCommand(goos, "https://x").Args; !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", goos, got, want)
		}
	}
}

func TestOpenFolder(t *testing.T) {
	started := stubCommands(t)
	dir := filepath.Join(t.TempDir(), "a", "b")
	openFolder(dir)
	if lastArg(*started) != dir {
		t.Errorf("expected %s opened, got %v", dir, *started)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("directory not created: %v", err)
	}
}

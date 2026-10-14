package settings

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", FileName)

	def, exists, err := load(path)
	if err != nil || exists {
		t.Fatalf("missing file: exists %v, err %v", exists, err)
	}
	if def.TileCache != DefaultTileCache || def.ElevationTileCache != DefaultElevationTileCache {
		t.Errorf("tile cache defaults not set: %d, %d", def.TileCache, def.ElevationTileCache)
	}
	if def.DiskCache.MaxSize != DefaultDiskCacheSize || def.DiskCache.MaxAge != DefaultDiskCacheAge ||
		def.DiskCache.ElevationMaxSize != DefaultDiskCacheElevationSize {
		t.Errorf("disk cache defaults not set: %+v", def.DiskCache)
	}
	if st := def.Style; st.TrackWidth != DefaultTrackWidth || st.RouteWidth != DefaultRouteWidth || st.WaypointSize != DefaultWaypointSize ||
		st.SymbolSize != DefaultSymbolSize || st.MutedColor != DefaultMutedColor {
		t.Errorf("style defaults not set: %+v", st)
	}

	s := Settings{
		TileCache:          100,
		ElevationTileCache: 50,
		DiskCache:          DiskCache{Dir: "/cache", MaxSize: -1, ElevationMaxSize: 500, MaxAge: 7},
		Style: Style{
			TrackWidth:   4,
			RouteWidth:   2.5,
			WaypointSize: 10,
			SymbolSize:   20,
			ColorBy:      "slope",
			Gradients:    map[string]string{"speed": "Turbo", "slope": "Blue–Red"},
			MutedColor:   "#a0a0a0",
		},
		View: &View{
			Lon: 12.5, Lat: -51.25, Zoom: 10.5,
			Map:      "OpenStreetMap",
			Overlays: []string{"Hillshade (SRTM)", "Labels & roads"},
		},
		Routing:   Routing{Tolerance: 50, SlightAngle: 30},
		Elevation: Elevation{Source: "mapterhorn", URL: "https://dem.example.com/{z}/{x}/{y}.png", Encoding: "mapbox", TileSize: 512, MaxZoom: 14},
		Window:    &Window{Width: 1100, Height: 700, X: new(-8), Y: new(20), Maximized: true, PanelWidth: 300, ChartHeight: 150, ChartClosed: true, ChartAxis: "clock_time"},
	}
	if err := save(path, &s); err != nil {
		t.Fatal(err)
	}
	got, exists, err := load(path)
	if err != nil || !exists {
		t.Fatalf("exists %v, err %v", exists, err)
	}
	if !reflect.DeepEqual(got, s) {
		t.Errorf("got %+v, want %+v", got, s)
	}
}

func TestReadWriteYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "test.yaml")

	type config struct {
		Names []string `yaml:"names"`
	}
	var c config
	exists, err := readYAML(path, &c)
	if err != nil || exists {
		t.Fatalf("missing file: exists %v, err %v", exists, err)
	}

	want := config{Names: []string{"a", "b"}}
	if err := writeYAML(path, &want); err != nil {
		t.Fatal(err)
	}
	exists, err = readYAML(path, &c)
	if err != nil || !exists {
		t.Fatalf("exists %v, err %v", exists, err)
	}
	if !reflect.DeepEqual(c, want) {
		t.Errorf("got %+v, want %+v", c, want)
	}
}

func TestPaneSizeDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := save(path, &Settings{Window: &Window{Width: 800, Height: 600}}); err != nil {
		t.Fatal(err)
	}
	s, _, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Window.PanelWidth != DefaultPanelWidth {
		t.Errorf("panel width default not set: %v", s.Window.PanelWidth)
	}
}

func TestRoutePointDefaults(t *testing.T) {
	for _, tc := range []struct{ val, wantTol, wantAngle float64 }{
		{0, DefaultRouteTolerance, DefaultSlightAngle},
		{50, 50, 50},
		{-1, -1, -1},
	} {
		path := filepath.Join(t.TempDir(), FileName)
		if err := save(path, &Settings{Routing: Routing{Tolerance: tc.val, SlightAngle: tc.val}}); err != nil {
			t.Fatal(err)
		}
		s, _, err := load(path)
		if err != nil {
			t.Fatal(err)
		}
		if s.Routing.Tolerance != tc.wantTol || s.Routing.SlightAngle != tc.wantAngle {
			t.Errorf("value %v: expected %v and %v, got %+v", tc.val, tc.wantTol, tc.wantAngle, s.Routing)
		}
	}
}

type versionedFile struct {
	Name string `yaml:"name"`
}

func (versionedFile) FormatVersion() int { return 3 }

func TestFileVersion(t *testing.T) {
	type file struct {
		Name string `yaml:"name,omitempty"`
	}
	dir := t.TempDir()
	WrittenBy = "TRAMP v1.2.3"
	t.Cleanup(func() { WrittenBy = "" })

	path := filepath.Join(dir, "file.yaml")
	if err := writeYAML(path, &file{Name: "a"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "# Written by TRAMP v1.2.3\nversion: 1\nname: a\n"; string(data) != want {
		t.Errorf("got %q, want %q", data, want)
	}
	var got file
	if _, err := readYAML(path, &got); err != nil || got.Name != "a" {
		t.Errorf("read back %+v, err %v", got, err)
	}

	// An empty struct must still give valid YAML.
	if err := writeYAML(path, &file{}); err != nil {
		t.Fatal(err)
	}
	if _, err := readYAML(path, &got); err != nil {
		t.Errorf("empty file: %v", err)
	}

	for _, tc := range []struct {
		content string
		newer   bool
	}{
		{"name: b\n", false},
		{"version: 1\nname: b\n", false},
		{"version: 2\nname: b\n", true},
	} {
		if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
			t.Fatal(err)
		}
		got := file{Name: "unchanged"}
		exists, err := readYAML(path, &got)
		if !exists {
			t.Errorf("%q: file not found", tc.content)
		}
		if tc.newer {
			if !errors.Is(err, ErrNewerVersion) || got.Name != "unchanged" {
				t.Errorf("%q: expected ErrNewerVersion and no change, got %+v, err %v", tc.content, got, err)
			}
		} else if err != nil || got.Name != "b" {
			t.Errorf("%q: got %+v, err %v", tc.content, got, err)
		}
	}

	// A type with its own version.
	if err := writeYAML(path, &versionedFile{Name: "c"}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "\nversion: 3\n") {
		t.Errorf("own version not written: %q", data)
	}
	var v versionedFile
	if _, err := readYAML(path, &v); err != nil || v.Name != "c" {
		t.Errorf("own version: got %+v, err %v", v, err)
	}
	if _, err := readYAML(path, &got); !errors.Is(err, ErrNewerVersion) {
		t.Errorf("version 3 read as version 1: %v", err)
	}
}

func TestNewerSettingsNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte("version: 99\ntile_cache: 5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, exists, err := load(path)
	if !exists || !errors.Is(err, ErrNewerVersion) {
		t.Fatalf("exists %v, err %v", exists, err)
	}
	if s.TileCache != DefaultTileCache {
		t.Errorf("expected defaults, got tile cache %d", s.TileCache)
	}
}

func TestUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	for _, tc := range []struct {
		content string
		keys    []string
	}{
		{"version: 1\ntile_cache: 5\n", nil},
		{"", nil},
		{"tile_cahce: 5\n", []string{"tile_cahce (line 1)"}},
		{"tile_cache: 5\nstyle:\n  trak_width: 2\n  route_width: 2\nfoo: 1\n", []string{"trak_width (line 3)", "foo (line 5)"}},
	} {
		if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
			t.Fatal(err)
		}
		s := Settings{TileCache: 1}
		_, err := readYAML(path, &s)
		var ue *UnknownKeysError
		if tc.keys == nil {
			if err != nil {
				t.Errorf("%q: %v", tc.content, err)
			}
			continue
		}
		if !errors.As(err, &ue) || !reflect.DeepEqual(ue.Keys, tc.keys) {
			t.Errorf("%q: expected unknown keys %v, got %v", tc.content, tc.keys, err)
		}
		if s.TileCache != 1 {
			t.Errorf("%q: settings changed despite the error: %+v", tc.content, s)
		}
	}

	// Other errors are not reported as unknown keys.
	if err := os.WriteFile(path, []byte("tile_cache: lots\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var s Settings
	_, err := readYAML(path, &s)
	var ue *UnknownKeysError
	if err == nil || errors.As(err, &ue) {
		t.Errorf("expected a type error, got %v", err)
	}
}

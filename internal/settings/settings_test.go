package settings

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", fileName)

	def, exists, err := load(path)
	if err != nil || exists {
		t.Fatalf("missing file: exists %v, err %v", exists, err)
	}
	if def.TileCache != DefaultTileCache {
		t.Errorf("tile cache default not set: %d", def.TileCache)
	}
	if st := def.Style; st.TrackWidth != DefaultTrackWidth || st.RouteWidth != DefaultRouteWidth || st.WaypointSize != DefaultWaypointSize ||
		st.SymbolSize != DefaultSymbolSize || st.MutedColor != DefaultMutedColor {
		t.Errorf("style defaults not set: %+v", st)
	}

	s := Settings{
		TileCache: 100,
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
		Routing: Routing{Tolerance: 50},
		Window: &Window{Width: 1100, Height: 700, X: new(-8), Y: new(20), Maximized: true, PanelWidth: 300, ChartHeight: 150, ChartClosed: true, ChartAxis: "clock_time"},
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
	path := filepath.Join(t.TempDir(), fileName)
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

func TestRouteToleranceDefault(t *testing.T) {
	for _, tc := range []struct{ tol, want float64 }{{0, DefaultRouteTolerance}, {50, 50}, {-1, -1}} {
		path := filepath.Join(t.TempDir(), fileName)
		if err := save(path, &Settings{Routing: Routing{Tolerance: tc.tol}}); err != nil {
			t.Fatal(err)
		}
		s, _, err := load(path)
		if err != nil {
			t.Fatal(err)
		}
		if s.Routing.Tolerance != tc.want {
			t.Errorf("tolerance %v: expected %v, got %v", tc.tol, tc.want, s.Routing.Tolerance)
		}
	}
}

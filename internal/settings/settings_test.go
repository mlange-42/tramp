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
	if st := def.Style; st.TrackWidth != DefaultTrackWidth || st.RouteWidth != DefaultRouteWidth || st.WaypointSize != DefaultWaypointSize {
		t.Errorf("style defaults not set: %+v", st)
	}

	s := Settings{
		TileCache: 100,
		Style: Style{
			TrackWidth:   4,
			RouteWidth:   2.5,
			WaypointSize: 10,
			ColorBy:      "slope",
			Gradients:    map[string]string{"speed": "Turbo", "slope": "Blue–Red"},
		},
		View: &View{
			Lon: 12.5, Lat: -51.25, Zoom: 10.5,
			Map:      "OpenStreetMap",
			Overlays: []string{"Hillshade (SRTM)", "Labels & roads"},
		},
		Window: &Window{Width: 1100, Height: 700, X: new(-8), Y: new(20), Maximized: true, PanelWidth: 300},
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

func TestPanelWidthDefault(t *testing.T) {
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

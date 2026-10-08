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
	if def.PanelWidth != DefaultPanelWidth {
		t.Errorf("panel width default not set: %v", def.PanelWidth)
	}

	s := Settings{
		TileCache:  100,
		PanelWidth: 300,
		Map:        "OpenStreetMap",
		Overlays:   []string{"Hillshade (SRTM)", "Labels & roads"},
		View:       &View{Lon: 12.5, Lat: -51.25, Zoom: 10.5},
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

package settings

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", fileName)

	_, exists, err := load(path)
	if err != nil || exists {
		t.Fatalf("missing file: exists %v, err %v", exists, err)
	}

	s := Settings{
		Map:      "OpenStreetMap",
		Overlays: []string{"Hillshade (SRTM)", "Labels & roads"},
		View:     &View{Lon: 12.5, Lat: -51.25, Zoom: 10.5},
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

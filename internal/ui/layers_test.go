package ui

import (
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestWMSConfigYAML(t *testing.T) {
	cfg := DefaultWMS()
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Overlay layers are inlined, and options only written where set.
	if strings.Contains(string(data), "layer:") || strings.Contains(string(data), "transparent") {
		t.Errorf("unexpected keys in:\n%s", data)
	}

	var got WMSConfig
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Errorf("got %+v, want %+v", got, cfg)
	}
}

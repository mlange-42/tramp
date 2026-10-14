package ui

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mlange-42/tramp/internal/settings"
)

var updateKeys = flag.Bool("update-keys", false, "write the missing key snapshots of TestConfigKeys")

// TestConfigKeys fails if the keys of a configuration file change without a new version of the file.
// The keys of each version are kept in testdata/keys. After raising a version, run
//
//	go test ./internal/ui -run TestConfigKeys -update-keys
//
// to write the snapshot of the new version. Existing snapshots are never overwritten.
func TestConfigKeys(t *testing.T) {
	for _, file := range []struct {
		name string
		v    interface{ FormatVersion() int }
	}{
		{settings.FileName, settings.Settings{}},
		{WMSFile, WMSConfig{}},
		{GradientsFile, GradientConfig{}},
		{PresetsFile, PresetConfig{}},
	} {
		version := file.v.FormatVersion()
		keys := settings.KeyPaths(file.v)
		base := strings.TrimSuffix(file.name, filepath.Ext(file.name))
		path := filepath.Join("testdata", "keys", fmt.Sprintf("%s.v%d.txt", base, version))
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			if !*updateKeys {
				t.Errorf("%s: no key snapshot for version %d. Run the test with -update-keys to write it", file.name, version)
				continue
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(strings.Join(keys, "\n")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		want := strings.Fields(string(data))
		if slices.Equal(keys, want) {
			continue
		}
		var diff []string
		for _, k := range keys {
			if !slices.Contains(want, k) {
				diff = append(diff, "+"+k)
			}
		}
		for _, k := range want {
			if !slices.Contains(keys, k) {
				diff = append(diff, "-"+k)
			}
		}
		t.Errorf("%s: keys changed in version %d: %s\n"+
			"Raise the version of the file and run the test with -update-keys, see settings.FormatVersion",
			file.name, version, strings.Join(diff, " "))
	}
}

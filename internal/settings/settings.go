// Package settings stores user preferences in a YAML file in the user's config directory.
package settings

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

const (
	dirName  = "tramp"
	fileName = "settings.yaml"
)

// Settings are the persisted user preferences.
type Settings struct {
	// Map is the name of the selected background map.
	Map string `yaml:"map"`
	// Overlays are the names of the enabled overlays.
	Overlays []string `yaml:"overlays"`
	// View is the last map view. Nil if not known yet.
	View *View `yaml:"view,omitempty"`
}

// View is a map position and zoom level.
type View struct {
	Lon  float64 `yaml:"lon"`
	Lat  float64 `yaml:"lat"`
	Zoom float64 `yaml:"zoom"`
}

// Dir returns the settings directory, e.g. %AppData%\tramp on Windows,
// ~/.config/tramp on Linux or ~/Library/Application Support/tramp on macOS.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, dirName), nil
}

// Path returns the path of the settings file.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

// Load reads the settings file.
// If it does not exist, it returns empty settings and exists is false.
func Load() (s Settings, exists bool, err error) {
	path, err := Path()
	if err != nil {
		return s, false, err
	}
	return load(path)
}

// Save writes the settings file, creating the settings directory if necessary.
func Save(s *Settings) error {
	path, err := Path()
	if err != nil {
		return err
	}
	return save(path, s)
}

func load(path string) (s Settings, exists bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, false, nil
	}
	if err != nil {
		return s, false, err
	}
	if err := yaml.Unmarshal(data, &s); err != nil {
		return s, true, fmt.Errorf("parsing %s: %w", path, err)
	}
	return s, true, nil
}

func save(path string, s *Settings) error {
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Write to a temporary file first, so that a crash doesn't leave a truncated file.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

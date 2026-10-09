// Package settings stores user preferences and configuration in YAML files in the user's config directory.
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

// DefaultTileCache is the default for [Settings.TileCache].
// A tile takes 256 KB of memory, so this is about 512 MB.
const DefaultTileCache = 2048

// DefaultPanelWidth is the default for [Settings.PanelWidth].
const DefaultPanelWidth = 250

// Settings are the persisted user preferences.
type Settings struct {
	// TileCache is the maximum number of map tiles kept in memory, shared by all visible layers.
	// A tile takes 256 KB.
	TileCache int `yaml:"tile_cache"`
	// PanelWidth is the width of the side panel, in device-independent pixels.
	PanelWidth float32 `yaml:"panel_width"`
	// Map is the name of the selected background map.
	Map string `yaml:"map"`
	// Overlays are the names of the enabled overlays.
	Overlays []string `yaml:"overlays"`
	// View is the last map view. Nil if not known yet.
	View *View `yaml:"view,omitempty"`
	// Window is the last main window size, position and state. Nil if not known yet.
	Window *Window `yaml:"window,omitempty"`
	// Files are the opened track files, in panel order.
	Files []File `yaml:"files,omitempty"`
}

// File is an opened track file.
type File struct {
	// Path is the absolute path of the file.
	Path string `yaml:"path"`
	// Hidden is whether the file is hidden on the map.
	Hidden bool `yaml:"hidden,omitempty"`
	// Colors are the colors of the tracks, routes and waypoints of the file, in file order, like "#e01010".
	Colors []string `yaml:"colors,omitempty"`
	// Order is the order of the tracks, routes and waypoints in the panel, as indices in file order.
	// Empty for file order.
	Order []int `yaml:"order,omitempty"`
}

// Window is the size, position and state of the main window.
type Window struct {
	// Width and Height of the window when not maximized, in device-independent pixels.
	// On Windows, this is the outer size including the frame, otherwise the content size.
	Width  float32 `yaml:"width"`
	Height float32 `yaml:"height"`
	// X and Y (left and top) are the top-left corner of the window when not maximized, in physical screen pixels.
	// Only used on Windows. Nil if not known.
	X *int `yaml:"left,omitempty"`
	Y *int `yaml:"top,omitempty"`
	// Maximized is whether the window is maximized.
	Maximized bool `yaml:"maximized"`
}

// Valid reports whether the window has a usable size.
func (w *Window) Valid() bool {
	return w != nil && w.Width > 0 && w.Height > 0
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
	return FilePath(fileName)
}

// FilePath returns the path of the file with the given name in the settings directory.
func FilePath(name string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// LoadFile reads the YAML file with the given name in the settings directory into v.
// If the file does not exist, v is left unchanged and exists is false.
func LoadFile(name string, v any) (exists bool, err error) {
	path, err := FilePath(name)
	if err != nil {
		return false, err
	}
	return readYAML(path, v)
}

// SaveFile writes v to the YAML file with the given name in the settings directory,
// creating the directory if necessary.
func SaveFile(name string, v any) error {
	path, err := FilePath(name)
	if err != nil {
		return err
	}
	return writeYAML(path, v)
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
	exists, err = readYAML(path, &s)
	s.setDefaults()
	return s, exists, err
}

// setDefaults fills in missing or invalid values.
func (s *Settings) setDefaults() {
	if s.TileCache <= 0 {
		s.TileCache = DefaultTileCache
	}
	if s.PanelWidth <= 0 {
		s.PanelWidth = DefaultPanelWidth
	}
}

func save(path string, s *Settings) error {
	return writeYAML(path, s)
}

func readYAML(path string, v any) (exists bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := yaml.Unmarshal(data, v); err != nil {
		return true, fmt.Errorf("parsing %s: %w", path, err)
	}
	return true, nil
}

func writeYAML(path string, v any) error {
	data, err := yaml.Marshal(v)
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

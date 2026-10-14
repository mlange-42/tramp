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

// Defaults for [Settings.TileCache] and [Settings.ElevationTileCache], in MB.
const (
	DefaultTileCache          = 1024
	DefaultElevationTileCache = 256
)

// Defaults for [DiskCache].
const (
	DefaultDiskCacheSize          = 2048
	DefaultDiskCacheElevationSize = 1024
	DefaultDiskCacheAge           = 30
)

// DefaultPanelWidth is the default for [Window.PanelWidth].
const DefaultPanelWidth = 280

// DefaultChartHeight is the default for [Window.ChartHeight].
const DefaultChartHeight = 120

// Defaults for the sizes of tracks, routes and waypoints on the map, in device-independent pixels.
const (
	DefaultTrackWidth   = 3
	DefaultRouteWidth   = 3
	DefaultWaypointSize = 8
	DefaultSymbolSize   = 18
)

// DefaultMutedColor is the default for [Style.MutedColor].
const DefaultMutedColor = "#808080c0"

// Settings are the persisted user preferences.
type Settings struct {
	// TileCache is the maximum memory in MB for the map tiles kept in memory, shared by all visible layers.
	TileCache int `yaml:"tile_cache"`
	// ElevationTileCache is the maximum memory in MB for the elevation tiles kept in memory.
	ElevationTileCache int `yaml:"elevation_tile_cache"`
	// DiskCache keeps downloaded map and elevation tiles on disk, for offline use.
	DiskCache DiskCache `yaml:"disk_cache"`
	// Style is how tracks, routes and waypoints are drawn on the map.
	Style Style `yaml:"style"`
	// View is the last map view, with the selected map and overlays. Nil if not known yet.
	View *View `yaml:"view,omitempty"`
	// GPS selects the GPS receiver for showing the live position.
	GPS GPS `yaml:"gps"`
	// Search selects the service for searching places by name.
	Search Search `yaml:"search"`
	// Routing selects the service for finding routes.
	Routing Routing `yaml:"routing"`
	// Elevation selects the source of terrain elevations.
	Elevation Elevation `yaml:"elevation"`
	// Window is the last main window size, position and state. Nil if not known yet.
	Window *Window `yaml:"window,omitempty"`
	// Files are the opened track files, in panel order.
	Files []File `yaml:"files,omitempty"`
	// Show are the features of the map view switched on or off in the Map menu, by name.
	// Missing ones have their default.
	Show map[string]bool `yaml:"show,omitempty"`
}

// Style is how tracks, routes and waypoints are drawn on the map.
type Style struct {
	// TrackWidth and RouteWidth are the line widths of tracks and routes,
	// in device-independent pixels.
	TrackWidth float32 `yaml:"track_width"`
	RouteWidth float32 `yaml:"route_width"`
	// WaypointSize is the diameter of waypoint dots, in device-independent pixels.
	WaypointSize float32 `yaml:"waypoint_size"`
	// SymbolSize is the diameter of the markers of waypoints with a symbol, in device-independent pixels.
	SymbolSize float32 `yaml:"symbol_size"`
	// ColorBy is the metric tracks are colored by: none, speed, elevation or slope.
	ColorBy string `yaml:"color_by,omitempty"`
	// Gradients are the names of the color gradients per metric.
	Gradients map[string]string `yaml:"gradients,omitempty"`
	// MutedColor is the color of the selected track or route on the map outside the range shown
	// in the zoomed chart, and of segments without value in tracks colored by value, like gaps in the recording.
	// Like "#808080", or with opacity like "#808080c0".
	MutedColor string `yaml:"muted_color"`
}

// DiskCache keeps downloaded map and elevation tiles on disk.
type DiskCache struct {
	// Dir is the directory of the cache. If empty, the tramp directory in the user's cache directory is used,
	// see [CacheDir].
	Dir string `yaml:"dir"`
	// MaxSize is the maximum size of the map tiles in MB. Negative turns the cache off, 0 for the default.
	MaxSize int `yaml:"max_size"`
	// ElevationMaxSize is the maximum size of the elevation tiles in MB. Negative turns the cache off,
	// 0 for the default.
	ElevationMaxSize int `yaml:"elevation_max_size"`
	// MaxAge is the age in days after which a tile is checked with the server again.
	// Older tiles are still used if the server can't be reached. 0 for the default.
	MaxAge int `yaml:"max_age"`
}

// GPS selects the serial port of the GPS receiver for showing the live position.
type GPS struct {
	// Port is the serial port, like COM5 or /dev/ttyUSB0. If empty, the receiver is searched on all ports.
	// It can also be a network source: tcp://host:port, udp://:port or gpsd://host[:port].
	Port string `yaml:"port"`
	// Baud is the baud rate of a serial port, like 4800. If 0, common baud rates are tried.
	Baud int `yaml:"baud"`
}

// Search selects the service for searching places by name.
type Search struct {
	// Service is photon or nominatim. If empty, photon is used.
	Service string `yaml:"service"`
	// URL is the search endpoint of the service, for using another server.
	// If empty, the public server of the service is used.
	URL string `yaml:"url"`
	// Limit is the maximum number of places listed for a search, up to [MaxSearchLimit].
	// If 0, [DefaultSearchLimit] is used.
	Limit int `yaml:"limit"`
}

// DefaultSearchLimit is the default for [Search.Limit].
const DefaultSearchLimit = 10

// MaxSearchLimit is the highest [Search.Limit], the most results Nominatim returns.
const MaxSearchLimit = 40

// SearchLimit returns the limit, or the default if not set, at most [MaxSearchLimit].
func (s Search) SearchLimit() int {
	if s.Limit <= 0 {
		return DefaultSearchLimit
	}
	return min(s.Limit, MaxSearchLimit)
}

// DefaultRouteTolerance is the default for [Routing.Tolerance], in meters.
const DefaultRouteTolerance = 100

// DefaultSlightAngle is the default for [Routing.SlightAngle], in degrees.
const DefaultSlightAngle = 22.5

// Routing selects the service for finding routes.
type Routing struct {
	// Service is valhalla or osrm. If empty, valhalla is used.
	Service string `yaml:"service"`
	// URL is the endpoint of the service, for using another server.
	// For OSRM, {profile} in it is replaced by foot, bike or car.
	// If empty, the public FOSSGIS server of the service is used.
	URL string `yaml:"url"`
	// Profile is the last selected means of travel: foot, bike or car. If empty, foot is used.
	Profile string `yaml:"profile"`
	// Tolerance is the largest distance in meters between a saved route and the straight lines
	// between its points. Points are added where needed. Negative to add none, 0 for the default.
	Tolerance float64 `yaml:"tolerance"`
	// SlightAngle is the smallest change of direction in degrees for which a slight turn gets a point
	// in a saved route. Negative for none, 0 for the default.
	SlightAngle float64 `yaml:"slight_angle"`
	// Points are the start, the via points and the target of the last route.
	Points []RoutePoint `yaml:"points,omitempty"`
}

// Elevation selects the source of terrain elevations: a built-in one, or another service of Terrain-RGB tiles.
type Elevation struct {
	// Source is aws or mapterhorn. If empty, the default is used.
	Source string `yaml:"source"`
	// URL is a tile URL template like "https://tiles.example.com/{z}/{x}/{y}.png", for another service.
	// The other fields override those of the source if set.
	URL string `yaml:"url"`
	// Encoding is terrarium or mapbox.
	Encoding string `yaml:"encoding"`
	// TileSize is the edge length of the tiles in pixels.
	TileSize int `yaml:"tile_size"`
	// MaxZoom is the highest tile level of the service.
	MaxZoom int `yaml:"max_zoom"`
}

// RoutePoint is a point of a route: a position, and the name of the place if it was searched.
type RoutePoint struct {
	Lon  float64 `yaml:"lon"`
	Lat  float64 `yaml:"lat"`
	Name string  `yaml:"name,omitempty"`
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
	// PanelWidth is the width of the side panel, in device-independent pixels.
	PanelWidth float32 `yaml:"panel_width"`
	// ChartHeight is the height of the chart below the map, in device-independent pixels.
	ChartHeight float32 `yaml:"chart_height"`
	// ChartClosed is whether the chart below the map is closed.
	ChartClosed bool `yaml:"chart_closed,omitempty"`
	// ChartAxis is the horizontal axis of the chart: distance, trip_time or clock_time.
	ChartAxis string `yaml:"chart_axis,omitempty"`
	// EditHintClosed is whether the hint on the map while editing is collapsed to an info icon.
	EditHintClosed bool `yaml:"edit_hint_closed,omitempty"`
}

// Valid reports whether the window has a usable size.
func (w *Window) Valid() bool {
	return w != nil && w.Width > 0 && w.Height > 0
}

// View is a map position and zoom level, with the selected map and overlays.
type View struct {
	Lon  float64 `yaml:"lon"`
	Lat  float64 `yaml:"lat"`
	Zoom float64 `yaml:"zoom"`
	// Map is the name of the selected background map.
	Map string `yaml:"map"`
	// Overlays are the names of the enabled overlays.
	Overlays []string `yaml:"overlays"`
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

// CacheDir returns the directory for cached data, e.g. %LocalAppData%\tramp on Windows,
// ~/.cache/tramp on Linux or ~/Library/Caches/tramp on macOS.
func CacheDir() (string, error) {
	base, err := os.UserCacheDir()
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
	if s.ElevationTileCache <= 0 {
		s.ElevationTileCache = DefaultElevationTileCache
	}
	if s.DiskCache.MaxSize == 0 {
		s.DiskCache.MaxSize = DefaultDiskCacheSize
	}
	if s.DiskCache.ElevationMaxSize == 0 {
		s.DiskCache.ElevationMaxSize = DefaultDiskCacheElevationSize
	}
	if s.DiskCache.MaxAge <= 0 {
		s.DiskCache.MaxAge = DefaultDiskCacheAge
	}
	if s.Window != nil && s.Window.PanelWidth <= 0 {
		s.Window.PanelWidth = DefaultPanelWidth
	}
	if s.Window != nil && s.Window.ChartHeight <= 0 {
		s.Window.ChartHeight = DefaultChartHeight
	}
	if s.Style.TrackWidth <= 0 {
		s.Style.TrackWidth = DefaultTrackWidth
	}
	if s.Style.RouteWidth <= 0 {
		s.Style.RouteWidth = DefaultRouteWidth
	}
	if s.Style.WaypointSize <= 0 {
		s.Style.WaypointSize = DefaultWaypointSize
	}
	if s.Style.SymbolSize <= 0 {
		s.Style.SymbolSize = DefaultSymbolSize
	}
	if s.Routing.Tolerance == 0 {
		s.Routing.Tolerance = DefaultRouteTolerance
	}
	if s.Routing.SlightAngle == 0 {
		s.Routing.SlightAngle = DefaultSlightAngle
	}
	if s.Style.MutedColor == "" {
		s.Style.MutedColor = DefaultMutedColor
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

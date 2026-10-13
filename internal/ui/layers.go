package ui

import (
	"fmt"
	"time"

	"github.com/mlange-42/tramp/internal/wms"
	"go.yaml.in/yaml/v3"
)

// WMSFile is the name of the WMS configuration file in the settings directory.
const WMSFile = "wms.yaml"

// WMSConfig are the available background maps and overlays.
type WMSConfig struct {
	// Maps are the background maps. There must be at least one.
	Maps []Map `yaml:"maps"`
	// Overlays are drawn on top of the background map, in this order.
	Overlays []Overlay `yaml:"overlays"`
}

// DefaultWMS returns the built-in maps and overlays.
func DefaultWMS() WMSConfig {
	return WMSConfig{Maps: DefaultMaps(), Overlays: DefaultOverlays()}
}

// WithoutDisabled returns the configuration without the disabled maps and overlays, see [wms.Layer.Enabled].
// Sections whose layers are all disabled are dropped too.
func (c WMSConfig) WithoutDisabled() WMSConfig {
	return WMSConfig{
		Maps:     withoutDisabled(c.Maps, func(m Map) (string, bool) { return m.Section, m.Layer.IsEnabled() }),
		Overlays: withoutDisabled(c.Overlays, func(o Overlay) (string, bool) { return o.Section, o.Layer.IsEnabled() }),
	}
}

// withoutDisabled returns the entries without the disabled layers, and without sections that had layers
// that are all disabled. entry returns the section heading of an entry, and whether it is enabled.
func withoutDisabled[E any](entries []E, entry func(E) (string, bool)) []E {
	var out []E
	section := -1 // index in out of the current section heading
	hasLayers, kept := false, false
	dropSection := func() {
		if section >= 0 && hasLayers && !kept {
			out = append(out[:section], out[section+1:]...)
		}
	}
	for _, e := range entries {
		name, enabled := entry(e)
		if name != "" {
			dropSection()
			section, hasLayers, kept = len(out), false, false
			out = append(out, e)
			continue
		}
		hasLayers = true
		if enabled {
			kept = true
			out = append(out, e)
		}
	}
	dropSection()
	return out
}

// Map is a background map, or a section heading in the list of maps.
type Map struct {
	// Section starts a section with this heading. If set, the layer must be empty.
	Section string    `yaml:"section,omitempty"`
	Layer   wms.Layer `yaml:",inline"`
}

// Overlay is a layer drawn on top of the background map, or a section heading in the list of overlays.
type Overlay struct {
	// Section starts a section with this heading. If set, the other fields must be empty.
	Section string    `yaml:"section,omitempty"`
	Layer   wms.Layer `yaml:",inline"`
	// Opacity of the overlay. Zero is treated as fully opaque.
	Opacity float32 `yaml:"opacity,omitempty"`
	// Shade converts a grayscale relief image into a transparent shading.
	Shade bool `yaml:"shade,omitempty"`
	// Refresh is the interval for loading the tiles again, for layers that change over time like
	// weather radar. Zero means never. Such layers are not stored in the disk cache.
	Refresh time.Duration `yaml:"refresh,omitempty"`
	// TimeStep restricts the times requested to multiples of it, for services that have data less often
	// than their capabilities tell. Zero means the times of the capabilities.
	TimeStep time.Duration `yaml:"time_step,omitempty"`
}

// MinRefresh is the shortest interval for loading an overlay again, see [Overlay.Refresh].
const MinRefresh = time.Minute

// sectionEntry is how section headings are written, without the empty layer fields.
type sectionEntry struct {
	Section string `yaml:"section"`
}

// MarshalYAML writes section headings without the empty layer fields.
func (m Map) MarshalYAML() (any, error) {
	if m.Section != "" {
		return sectionEntry{m.Section}, nil
	}
	type plain Map
	return plain(m), nil
}

// UnmarshalYAML reads a map or section heading, and checks that it is not both or neither.
func (m *Map) UnmarshalYAML(value *yaml.Node) error {
	type plain Map
	if err := value.Decode((*plain)(m)); err != nil {
		return err
	}
	return checkEntry(value, m.Section, m.Layer != wms.Layer{})
}

// MarshalYAML writes section headings without the empty layer fields.
func (o Overlay) MarshalYAML() (any, error) {
	if o.Section != "" {
		return sectionEntry{o.Section}, nil
	}
	type plain Overlay
	return plain(o), nil
}

// UnmarshalYAML reads an overlay or section heading, and checks that it is not both or neither.
func (o *Overlay) UnmarshalYAML(value *yaml.Node) error {
	type plain Overlay
	if err := value.Decode((*plain)(o)); err != nil {
		return err
	}
	rest := *o
	rest.Section = ""
	return checkEntry(value, o.Section, rest != Overlay{})
}

// checkEntry reports an error if an entry is both a section heading and a layer, or neither.
func checkEntry(value *yaml.Node, section string, isLayer bool) error {
	switch {
	case section != "" && isLayer:
		return fmt.Errorf("line %d: section %q must be a separate entry", value.Line, section)
	case section == "" && !isLayer:
		return fmt.Errorf("line %d: empty entry", value.Line)
	}
	return nil
}

// Section is a heading in a drop-down list, shown before the option at Index.
type Section struct {
	Index int
	Name  string
}

// splitSections separates list entries into options and section headings.
// split returns the section heading of an entry, or its option if the heading is empty.
// Sections without options after them are dropped.
func splitSections[E, T any](entries []E, split func(E) (string, T)) ([]T, []Section) {
	var options []T
	var sections []Section
	for _, e := range entries {
		if name, opt := split(e); name != "" {
			sections = append(sections, Section{Index: len(options), Name: name})
		} else {
			options = append(options, opt)
		}
	}
	for len(sections) > 0 && sections[len(sections)-1].Index == len(options) {
		sections = sections[:len(sections)-1]
	}
	return options, sections
}

const (
	eoxURL       = "https://tiles.maps.eox.at/wms"
	bkgURL       = "https://sgx.geodatenzentrum.de"
	cyclosmURL   = "https://a.tile-cyclosm.openstreetmap.fr"
	waymarkedURL = "https://tile.waymarkedtrails.org"
	dwdURL       = "https://maps.dwd.de/geoserver/ows"
	// dwdWorkspaceURL is the start of the addresses of the services for single layers.
	dwdWorkspaceURL = "https://maps.dwd.de/geoserver/dwd/"

	osmAttribution       = "© OpenStreetMap contributors"
	eoxOSMAttribution    = "Data © OpenStreetMap contributors and others, Rendering © EOX"
	bkgAttribution       = "© Bundesamt für Kartographie und Geodäsie"
	basemapAttribution   = "© basemap.de / BKG, Datenquellen: © GeoBasis-DE"
	cyclosmAttribution   = osmAttribution + ", Style CyclOSM, Tiles OpenStreetMap France"
	waymarkedAttribution = osmAttribution + ", Overlay © waymarkedtrails.org"
	dwdAttribution       = "Data © Deutscher Wetterdienst (DWD), CC BY 4.0"

	// osmShort is the short credit of OpenStreetMap data, shared by many layers.
	osmShort = "© OpenStreetMap"
	// osmCopyrightURL is the page with the license and the credits of OpenStreetMap.
	osmCopyrightURL = "https://www.openstreetmap.org/copyright"
	// dwdCopyrightURL is the page with the terms of use of the Deutscher Wetterdienst, linked from its WMS.
	dwdCopyrightURL = "https://www.dwd.de/copyright"
)

// DefaultOverlays are the built-in overlays, in drawing order, with section headings.
// All of them must support EPSG:3857.
func DefaultOverlays() []Overlay {
	return []Overlay{
		{Section: "Hillshade"},
		{
			// The free terrestris service adds advertising watermarks to some tiles.
			Layer: wms.Layer{
				Name:             "Hillshade (SRTM)",
				Description:      "Relief shading from SRTM elevation data. Worldwide, coarse. Some tiles have watermarks of the terrestris service.",
				URL:              "https://ows.terrestris.de/osm/service",
				Layers:           "SRTM30-Hillshade",
				Format:           "image/png",
				Version:          "1.3.0",
				Attribution:      "SRTM, terrestris",
				AttributionShort: "SRTM · terrestris",
				AttributionURL:   "https://www.terrestris.de",
			},
			Opacity: 0.6,
			Shade:   true,
		},
		{
			// Germany only, high resolution. White on flat terrain, unlike the plain "hillshade" layer.
			Layer: wms.Layer{
				Name:             "Hillshade Germany (basemap.de)",
				Description:      "High-resolution relief shading by the BKG. Germany only.",
				Type:             wms.TypeXYZ,
				URL:              bkgURL + "/wmts_basemapde_schummerung/tile/1.0.0/de_basemapde_web_raster_combshade/default/GLOBAL_WEBMERCATOR/{z}/{y}/{x}.png",
				Attribution:      basemapAttribution,
				AttributionShort: "© basemap.de / BKG",
				AttributionURL:   "https://basemap.de",
			},
			Opacity: 0.6,
			Shade:   true,
		},
		{Section: "Routes"},
		{
			Layer: wms.Layer{
				Name:             "Hiking routes",
				Description:      "Marked hiking trails, from waymarkedtrails.org. Worldwide.",
				Type:             wms.TypeXYZ,
				URL:              waymarkedURL + "/hiking/{z}/{x}/{y}.png",
				MaxZoom:          18,
				Attribution:      waymarkedAttribution,
				AttributionShort: osmShort + " · Waymarked Trails",
				AttributionURL:   "https://waymarkedtrails.org",
			},
		},
		{
			Layer: wms.Layer{
				Name:             "Cycling routes",
				Description:      "Signed cycling routes, from waymarkedtrails.org. Worldwide.",
				Type:             wms.TypeXYZ,
				URL:              waymarkedURL + "/cycling/{z}/{x}/{y}.png",
				MaxZoom:          18,
				Attribution:      waymarkedAttribution,
				AttributionShort: osmShort + " · Waymarked Trails",
				AttributionURL:   "https://waymarkedtrails.org",
			},
		},
		{
			Layer: wms.Layer{
				Name:             "MTB routes",
				Description:      "Signed mountain bike routes, from waymarkedtrails.org. Worldwide.",
				Type:             wms.TypeXYZ,
				URL:              waymarkedURL + "/mtb/{z}/{x}/{y}.png",
				MaxZoom:          18,
				Attribution:      waymarkedAttribution,
				AttributionShort: osmShort + " · Waymarked Trails",
				AttributionURL:   "https://waymarkedtrails.org",
			},
		},
		{
			Layer: wms.Layer{
				Name:             "Cycle infrastructure",
				Description:      "Bike lanes, cycle paths and bike parking, from CyclOSM. Worldwide.",
				Type:             wms.TypeXYZ,
				URL:              cyclosmURL + "/cyclosm-lite/{z}/{x}/{y}.png",
				MaxZoom:          20,
				Attribution:      cyclosmAttribution,
				AttributionShort: osmShort + " · CyclOSM",
				AttributionURL:   "https://www.cyclosm.org",
			},
		},
		{Section: "Weather"},
		{
			// Gray outside the radar coverage.
			Layer: dwdLayer("Precipitation radar", "Niederschlagsradar", 11,
				"Rain and snow from the weather radar, every 5 minutes. Germany and surroundings."),
			Opacity: 0.7,
			Refresh: 5 * time.Minute,
		},
		{
			Layer: dwdLayer("Weather warnings", "Warnungen_Gemeinden_vereinigt", 12,
				"Current official weather warnings for municipalities. Germany only."),
			Refresh: 10 * time.Minute,
		},
		{
			Layer: dwdLayer("Wind 10m", "Icon_reg025_fd_sl_UV10M", 0,
				"Forecast wind barbs at 10m above ground, from the ICON model. Worldwide."),
			Refresh: 30 * time.Minute,
		},
		{
			Layer: dwdLayer("Precipitation 1h", "Icon-eu_reg00625_fd_sl_TOTPREC01H", 9,
				"Forecast precipitation in the hour up to the shown time, from the ICON-EU model. Europe."),
			Opacity: 0.6,
			Refresh: 30 * time.Minute,
		},
		{
			Layer: dwdLayer("Precipitation 6h", "Icon_reg025_fd_sl_TOTPREC06H", 8,
				"Forecast precipitation in the 6 hours up to the shown time, from the ICON model. Worldwide."),
			Opacity: 0.6,
			Refresh: 30 * time.Minute,
			// The capabilities tell every 3 hours, but there is data only every 6 hours.
			TimeStep: 6 * time.Hour,
		},
		{Section: "Labels"},
		{
			Layer: wms.Layer{
				Name:             "Borders & places",
				Description:      "Coastlines, borders and place names, without roads. Worldwide.",
				URL:              eoxURL,
				Layers:           "coastline_3857,overlay_base_3857",
				Format:           "image/png",
				Version:          "1.3.0",
				Attribution:      eoxOSMAttribution,
				AttributionShort: osmShort + " · EOX",
				AttributionURL:   "https://maps.eox.at",
			},
		},
		{
			// Dark labels, for bright maps.
			Layer: wms.Layer{
				Name:             "Labels & roads",
				Description:      "Place names and roads in dark text, for bright maps. Worldwide.",
				URL:              eoxURL,
				Layers:           "overlay_3857",
				Format:           "image/png",
				Version:          "1.3.0",
				Attribution:      eoxOSMAttribution,
				AttributionShort: osmShort + " · EOX",
				AttributionURL:   "https://maps.eox.at",
			},
		},
		{
			// Bright labels, for the satellite map.
			Layer: wms.Layer{
				Name:             "Labels & roads (bright)",
				Description:      "Place names and roads in bright text, for the satellite map. Worldwide.",
				URL:              eoxURL,
				Layers:           "overlay_bright_3857",
				Format:           "image/png",
				Version:          "1.3.0",
				Attribution:      eoxOSMAttribution,
				AttributionShort: osmShort + " · EOX",
				AttributionURL:   "https://maps.eox.at",
			},
		},
	}
}

// dwdLayer returns a layer of the WMS of the Deutscher Wetterdienst.
func dwdLayer(name, layer string, maxZoom int, description string) wms.Layer {
	return wms.Layer{
		Name:        name,
		Description: description,
		URL:         dwdURL,
		Layers:      "dwd:" + layer,
		// The capabilities of the single layer are much smaller than those of the whole service.
		Capabilities: dwdWorkspaceURL + layer + "/ows",
		Format:       "image/png",
		Version:      "1.3.0",
		MaxZoom:      maxZoom,
		// The DWD needs about as long for 4×4 tiles in one image as for a single tile.
		MetaTiles:        4,
		Attribution:      dwdAttribution,
		AttributionShort: "© DWD",
		AttributionURL:   dwdCopyrightURL,
	}
}

// DefaultMaps are the built-in background maps, with section headings.
// All of them must support EPSG:3857 or be XYZ tile services in Web Mercator.
// The first map is the default.
func DefaultMaps() []Map {
	return []Map{
		{Section: "World"},
		{
			Layer: wms.Layer{
				Name:             "OSM Standard",
				Description:      "Standard OpenStreetMap style. Worldwide, detailed up to zoom 19.",
				Type:             wms.TypeXYZ,
				URL:              "https://tile.openstreetmap.org/{z}/{x}/{y}.png",
				MaxZoom:          19,
				Attribution:      osmAttribution,
				AttributionShort: osmShort,
				AttributionURL:   osmCopyrightURL,
			},
		},
		{
			Layer: wms.Layer{
				Name:             "CyclOSM",
				Description:      "OpenStreetMap style for cycling, with bike lanes, surfaces and bike shops. Worldwide.",
				Type:             wms.TypeXYZ,
				URL:              cyclosmURL + "/cyclosm/{z}/{x}/{y}.png",
				MaxZoom:          20,
				Attribution:      cyclosmAttribution,
				AttributionShort: osmShort + " · CyclOSM",
				AttributionURL:   "https://www.cyclosm.org",
			},
		},
		{
			Layer: wms.Layer{
				Name:        "OpenTopoMap",
				Description: "Topographic map with contour lines and hillshading, from OpenStreetMap and SRTM data. Worldwide, up to zoom 17.",
				Type:        wms.TypeXYZ,
				URL:         "https://tile.opentopomap.org/{z}/{x}/{y}.png",
				// Served up to level 17.
				MaxZoom:          17,
				Attribution:      "Map data © OpenStreetMap contributors, SRTM | Map style © OpenTopoMap (CC-BY-SA)",
				AttributionShort: osmShort + " · OpenTopoMap",
				AttributionURL:   "https://opentopomap.org/about",
			},
		},
		{
			Layer: wms.Layer{
				Name:             "Terrain Light",
				Description:      "Light terrain map from OpenStreetMap data with relief shading. Worldwide, works well under overlays.",
				URL:              eoxURL,
				Layers:           "terrain-light_3857",
				Format:           "image/png",
				Version:          "1.3.0",
				Attribution:      eoxOSMAttribution,
				AttributionShort: osmShort + " · EOX",
				AttributionURL:   "https://maps.eox.at",
			},
		},
		{
			Layer: wms.Layer{
				Name:        "Natural Earth",
				Description: "Small-scale world map with land cover tints, relief shading and water, from Natural Earth. Worldwide, coarse, so best for an overview, e.g. under weather overlays.",
				URL:         dwdURL,
				Layers:      "dwd:Natural_Earth_Map",
				Format:      "image/png",
				Version:     "1.3.0",
				// The raster has about 1-2 km per pixel, so closer levels show nothing new.
				MaxZoom: 8,
				// The DWD forbids storing all its tiles, but this map doesn't change.
				Cache:            wms.CacheAlways,
				Attribution:      "Made with Natural Earth, rendered by the Deutscher Wetterdienst (DWD)",
				AttributionShort: "Natural Earth · DWD",
				AttributionURL:   "https://www.naturalearthdata.com",
			},
		},
		{
			Layer: wms.Layer{
				Name:             "Satellite",
				Description:      "Cloud-free Sentinel-2 satellite imagery from 2025. Worldwide, at about 10 m resolution, so blurry when zoomed in close.",
				URL:              eoxURL,
				Layers:           "s2cloudless-2025_3857",
				Format:           "image/jpeg",
				Version:          "1.3.0",
				Attribution:      "Sentinel-2 cloudless by EOX IT Services GmbH (contains modified Copernicus Sentinel data 2025)",
				AttributionShort: "Sentinel-2 cloudless by EOX",
				AttributionURL:   "https://s2maps.eu",
			},
		},
		{Section: "Europe"},
		{
			Layer: wms.Layer{
				Name:             "TopPlusOpen",
				Description:      "Official topographic map by the BKG. Worldwide at small scales, Europe at medium scales, full detail only in Germany.",
				URL:              bkgURL + "/wms_topplus_open",
				Layers:           "web",
				Format:           "image/png",
				Version:          "1.3.0",
				Attribution:      bkgAttribution,
				AttributionShort: "© BKG",
				AttributionURL:   "https://gdz.bkg.bund.de/index.php/default/wms-topplusopen-wms-topplus-open.html",
			},
		},
		{
			Layer: wms.Layer{
				Name:             "TopPlusOpen Grey",
				Description:      "Greyscale version of TopPlusOpen, so that tracks and overlays stand out. Same coverage.",
				URL:              bkgURL + "/wms_topplus_open",
				Layers:           "web_grau",
				Format:           "image/png",
				Version:          "1.3.0",
				Attribution:      bkgAttribution,
				AttributionShort: "© BKG",
				AttributionURL:   "https://gdz.bkg.bund.de/index.php/default/wms-topplusopen-wms-topplus-open.html",
			},
		},
		{Section: "Germany"},
		{
			Layer: wms.Layer{
				Name:             "basemap.de",
				Description:      "Official German topographic base map by the BKG. Germany only, very detailed.",
				Type:             wms.TypeXYZ,
				URL:              bkgURL + "/wmts_basemapde/tile/1.0.0/de_basemapde_web_raster_farbe/default/GLOBAL_WEBMERCATOR/{z}/{y}/{x}.png",
				Attribution:      basemapAttribution,
				AttributionShort: "© basemap.de / BKG",
				AttributionURL:   "https://basemap.de",
			},
		},
		{Section: "Other"},
		{
			// No background map, for showing only the overlays.
			Layer: wms.Layer{
				Name:        "None",
				Description: "No background map, shows only the overlays.",
				Type:        wms.TypeNone,
			},
		},
	}
}

package ui

import (
	"fmt"

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
}

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

	osmAttribution       = "© OpenStreetMap contributors"
	eoxOSMAttribution    = "Data © OpenStreetMap contributors and others, Rendering © EOX"
	bkgAttribution       = "© Bundesamt für Kartographie und Geodäsie"
	basemapAttribution   = "© basemap.de / BKG, Datenquellen: © GeoBasis-DE"
	cyclosmAttribution   = osmAttribution + ", Style CyclOSM, Tiles OpenStreetMap France"
	waymarkedAttribution = osmAttribution + ", Overlay © waymarkedtrails.org"
)

// DefaultOverlays are the built-in overlays, in drawing order, with section headings.
// All of them must support EPSG:3857.
func DefaultOverlays() []Overlay {
	return []Overlay{
		{Section: "Hillshade"},
		{
			// The free terrestris service adds advertising watermarks to some tiles.
			Layer: wms.Layer{
				Name:        "Hillshade (SRTM)",
				URL:         "https://ows.terrestris.de/osm/service",
				Layers:      "SRTM30-Hillshade",
				Format:      "image/png",
				Version:     "1.3.0",
				Attribution: "SRTM, terrestris",
			},
			Opacity: 0.6,
			Shade:   true,
		},
		{
			// Germany only, high resolution. White on flat terrain, unlike the plain "hillshade" layer.
			Layer: wms.Layer{
				Name:        "Hillshade Germany (basemap.de)",
				Type:        wms.TypeXYZ,
				URL:         bkgURL + "/wmts_basemapde_schummerung/tile/1.0.0/de_basemapde_web_raster_combshade/default/GLOBAL_WEBMERCATOR/{z}/{y}/{x}.png",
				Attribution: basemapAttribution,
			},
			Opacity: 0.6,
			Shade:   true,
		},
		{Section: "Routes"},
		{
			Layer: wms.Layer{
				Name:        "Hiking routes",
				Type:        wms.TypeXYZ,
				URL:         waymarkedURL + "/hiking/{z}/{x}/{y}.png",
				MaxZoom:     18,
				Attribution: waymarkedAttribution,
			},
		},
		{
			Layer: wms.Layer{
				Name:        "Cycling routes",
				Type:        wms.TypeXYZ,
				URL:         waymarkedURL + "/cycling/{z}/{x}/{y}.png",
				MaxZoom:     18,
				Attribution: waymarkedAttribution,
			},
		},
		{
			Layer: wms.Layer{
				Name:        "MTB routes",
				Type:        wms.TypeXYZ,
				URL:         waymarkedURL + "/mtb/{z}/{x}/{y}.png",
				MaxZoom:     18,
				Attribution: waymarkedAttribution,
			},
		},
		{
			Layer: wms.Layer{
				Name:        "Cycle infrastructure",
				Type:        wms.TypeXYZ,
				URL:         cyclosmURL + "/cyclosm-lite/{z}/{x}/{y}.png",
				MaxZoom:     20,
				Attribution: cyclosmAttribution,
			},
		},
		{Section: "Labels"},
		{
			// Dark labels, for bright maps.
			Layer: wms.Layer{
				Name:        "Labels & roads",
				URL:         eoxURL,
				Layers:      "overlay_3857",
				Format:      "image/png",
				Version:     "1.3.0",
				Attribution: eoxOSMAttribution,
			},
		},
		{
			// Bright labels, for the satellite map.
			Layer: wms.Layer{
				Name:        "Labels & roads (bright)",
				URL:         eoxURL,
				Layers:      "overlay_bright_3857",
				Format:      "image/png",
				Version:     "1.3.0",
				Attribution: eoxOSMAttribution,
			},
		},
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
				Name:        "OSM Standard",
				Type:        wms.TypeXYZ,
				URL:         "https://tile.openstreetmap.org/{z}/{x}/{y}.png",
				MaxZoom:     19,
				Attribution: osmAttribution,
			},
		},
		{
			Layer: wms.Layer{
				Name:        "CyclOSM",
				Type:        wms.TypeXYZ,
				URL:         cyclosmURL + "/cyclosm/{z}/{x}/{y}.png",
				MaxZoom:     20,
				Attribution: cyclosmAttribution,
			},
		},
		{
			Layer: wms.Layer{
				Name: "OpenTopoMap",
				Type: wms.TypeXYZ,
				URL:  "https://tile.opentopomap.org/{z}/{x}/{y}.png",
				// Served up to level 17.
				MaxZoom:     17,
				Attribution: "Map data © OpenStreetMap contributors, SRTM | Map style © OpenTopoMap (CC-BY-SA)",
			},
		},
		{
			Layer: wms.Layer{
				Name:        "Terrain Light",
				URL:         eoxURL,
				Layers:      "terrain-light_3857",
				Format:      "image/png",
				Version:     "1.3.0",
				Attribution: eoxOSMAttribution,
			},
		},
		{
			Layer: wms.Layer{
				Name:        "Satellite",
				URL:         eoxURL,
				Layers:      "s2cloudless-2025_3857",
				Format:      "image/jpeg",
				Version:     "1.3.0",
				Attribution: "Sentinel-2 cloudless by EOX IT Services GmbH (contains modified Copernicus Sentinel data 2025)",
			},
		},
		{Section: "Europe"},
		{
			Layer: wms.Layer{
				Name:        "TopPlusOpen",
				URL:         bkgURL + "/wms_topplus_open",
				Layers:      "web",
				Format:      "image/png",
				Version:     "1.3.0",
				Attribution: bkgAttribution,
			},
		},
		{
			Layer: wms.Layer{
				Name:        "TopPlusOpen Grey",
				URL:         bkgURL + "/wms_topplus_open",
				Layers:      "web_grau",
				Format:      "image/png",
				Version:     "1.3.0",
				Attribution: bkgAttribution,
			},
		},
		{Section: "Germany"},
		{
			Layer: wms.Layer{
				Name:        "basemap.de",
				Type:        wms.TypeXYZ,
				URL:         bkgURL + "/wmts_basemapde/tile/1.0.0/de_basemapde_web_raster_farbe/default/GLOBAL_WEBMERCATOR/{z}/{y}/{x}.png",
				Attribution: basemapAttribution,
			},
		},
		{Section: "Other"},
		{
			// No background map, for showing only the overlays.
			Layer: wms.Layer{
				Name: "None",
				Type: wms.TypeNone,
			},
		},
	}
}

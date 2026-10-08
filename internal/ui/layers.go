package ui

import "github.com/mlange-42/tramp/internal/wms"

// WMSFile is the name of the WMS configuration file in the settings directory.
const WMSFile = "wms.yaml"

// WMSConfig are the available background maps and overlays.
type WMSConfig struct {
	// Maps are the background maps. There must be at least one.
	Maps []wms.Layer `yaml:"maps"`
	// Overlays are drawn on top of the background map, in this order.
	Overlays []Overlay `yaml:"overlays"`
}

// DefaultWMS returns the built-in maps and overlays.
func DefaultWMS() WMSConfig {
	return WMSConfig{Maps: DefaultLayers(), Overlays: DefaultOverlays()}
}

// Overlay is a layer drawn on top of the background map.
type Overlay struct {
	Layer wms.Layer `yaml:",inline"`
	// Opacity of the overlay. Zero is treated as fully opaque.
	Opacity float32 `yaml:"opacity,omitempty"`
	// Shade converts a grayscale relief image into a transparent shading.
	Shade bool `yaml:"shade,omitempty"`
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

// DefaultOverlays are the built-in overlays, in drawing order.
// All of them must support EPSG:3857.
func DefaultOverlays() []Overlay {
	return []Overlay{
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

// DefaultLayers are the built-in background maps.
// All of them must support EPSG:3857 or be XYZ tile services in Web Mercator.
func DefaultLayers() []wms.Layer {
	return []wms.Layer{
		{
			Name:        "TopPlusOpen",
			URL:         bkgURL + "/wms_topplus_open",
			Layers:      "web",
			Format:      "image/png",
			Version:     "1.3.0",
			Attribution: bkgAttribution,
		},
		{
			Name:        "TopPlusOpen Grey",
			URL:         bkgURL + "/wms_topplus_open",
			Layers:      "web_grau",
			Format:      "image/png",
			Version:     "1.3.0",
			Attribution: bkgAttribution,
		},
		{
			Name:        "basemap.de",
			Type:        wms.TypeXYZ,
			URL:         bkgURL + "/wmts_basemapde/tile/1.0.0/de_basemapde_web_raster_farbe/default/GLOBAL_WEBMERCATOR/{z}/{y}/{x}.png",
			Attribution: basemapAttribution,
		},
		{
			Name:        "Terrain Light",
			URL:         eoxURL,
			Layers:      "terrain-light_3857",
			Format:      "image/png",
			Version:     "1.3.0",
			Attribution: eoxOSMAttribution,
		},
		{
			Name:        "OpenStreetMap",
			URL:         eoxURL,
			Layers:      "osm_3857",
			Format:      "image/png",
			Version:     "1.3.0",
			Attribution: eoxOSMAttribution,
		},
		{
			Name:        "OSM Standard",
			Type:        wms.TypeXYZ,
			URL:         "https://tile.openstreetmap.org/{z}/{x}/{y}.png",
			MaxZoom:     19,
			Attribution: osmAttribution,
		},
		{
			Name:        "CyclOSM",
			Type:        wms.TypeXYZ,
			URL:         cyclosmURL + "/cyclosm/{z}/{x}/{y}.png",
			MaxZoom:     20,
			Attribution: cyclosmAttribution,
		},
		{
			Name: "OpenTopoMap",
			Type: wms.TypeXYZ,
			URL:  "https://tile.opentopomap.org/{z}/{x}/{y}.png",
			// Served up to level 17.
			MaxZoom:     17,
			Attribution: "Map data © OpenStreetMap contributors, SRTM | Map style © OpenTopoMap (CC-BY-SA)",
		},
		{
			Name:        "Satellite",
			URL:         eoxURL,
			Layers:      "s2cloudless-2025_3857",
			Format:      "image/jpeg",
			Version:     "1.3.0",
			Attribution: "Sentinel-2 cloudless by EOX IT Services GmbH (contains modified Copernicus Sentinel data 2025)",
		},
		// The free terrestris service adds advertising watermarks to some tiles.
		// Use them with the -wms flag if needed:
		//   -wms https://ows.terrestris.de/osm/service -layers OSM-WMS
		//   -wms https://ows.terrestris.de/osm/service -layers TOPO-OSM-WMS
	}
}

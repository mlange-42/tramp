package ui

import "github.com/mlange-42/tramp/internal/wms"

// Overlay is a layer drawn on top of the background map.
type Overlay struct {
	Layer wms.Layer
	// Opacity of the overlay. Zero is treated as fully opaque.
	Opacity float32
	// Shade converts a grayscale relief image into a transparent shading.
	Shade bool
}

const (
	eoxURL = "https://tiles.maps.eox.at/wms"
	bkgURL = "https://sgx.geodatenzentrum.de"

	eoxOSMAttribution = "Data © OpenStreetMap contributors and others, Rendering © EOX"
	bkgAttribution    = "© Bundesamt für Kartographie und Geodäsie"
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
			// Germany only, 200 m resolution.
			Layer: wms.Layer{
				Name:        "Hillshade Germany (BKG)",
				URL:         bkgURL + "/wms_dgm200",
				Layers:      "schummerung",
				Format:      "image/png",
				Version:     "1.3.0",
				Attribution: bkgAttribution,
			},
			Opacity: 0.6,
			Shade:   true,
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
// All of them must support EPSG:3857.
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

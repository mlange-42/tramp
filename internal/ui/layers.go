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

// DefaultOverlays are the built-in overlays.
// All of them must support EPSG:3857.
func DefaultOverlays() []Overlay {
	return []Overlay{
		{
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
	}
}

// DefaultLayers are the built-in background maps.
// All of them must support EPSG:3857.
func DefaultLayers() []wms.Layer {
	return []wms.Layer{
		{
			Name:        "OSM",
			URL:         "https://ows.terrestris.de/osm/service",
			Layers:      "OSM-WMS",
			Format:      "image/png",
			Version:     "1.3.0",
			Attribution: "© OpenStreetMap contributors, terrestris",
		},
		{
			Name:        "OSM Topo",
			URL:         "https://ows.terrestris.de/osm/service",
			Layers:      "TOPO-OSM-WMS",
			Format:      "image/png",
			Version:     "1.3.0",
			Attribution: "© OpenStreetMap contributors, SRTM, terrestris",
		},
		{
			Name:        "TopPlusOpen",
			URL:         "https://sgx.geodatenzentrum.de/wms_topplus_open",
			Layers:      "web",
			Format:      "image/png",
			Version:     "1.3.0",
			Attribution: "© Bundesamt für Kartographie und Geodäsie",
		},
		{
			Name:        "TopPlusOpen Grey",
			URL:         "https://sgx.geodatenzentrum.de/wms_topplus_open",
			Layers:      "web_grau",
			Format:      "image/png",
			Version:     "1.3.0",
			Attribution: "© Bundesamt für Kartographie und Geodäsie",
		},
	}
}

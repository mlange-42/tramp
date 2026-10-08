package ui

import "github.com/mlange-42/tramp/internal/wms"

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

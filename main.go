// TRAMP - Track and Route Analysis, Mapping and Planning.
package main

import (
	"flag"
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/unit"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/ui"
	"github.com/mlange-42/tramp/internal/wms"
)

func main() {
	lon := flag.Float64("lon", 12.3731, "longitude of the initial map center")
	lat := flag.Float64("lat", 51.3397, "latitude of the initial map center")
	zoom := flag.Float64("zoom", 12, "initial zoom level")
	wmsURL := flag.String("wms", "", "URL of an additional WMS service, must support EPSG:3857")
	wmsLayers := flag.String("layers", "", "comma-separated layer names for -wms")
	wmsFormat := flag.String("format", "image/png", "image format for -wms")
	wmsVersion := flag.String("version", "1.3.0", "WMS version for -wms")
	flag.Parse()

	layers := ui.DefaultLayers()
	if *wmsURL != "" {
		if *wmsLayers == "" {
			log.Fatal("-layers is required with -wms")
		}
		custom := wms.Layer{
			Name:    "Custom",
			URL:     *wmsURL,
			Layers:  *wmsLayers,
			Format:  *wmsFormat,
			Version: *wmsVersion,
		}
		layers = append([]wms.Layer{custom}, layers...)
	}

	go func() {
		w := new(app.Window)
		w.Option(app.Title("TRAMP"), app.Size(unit.Dp(1100), unit.Dp(700)))
		a := ui.New(w, ui.Options{
			Layers:   layers,
			Overlays: ui.DefaultOverlays(),
			Center:   geo.LonLat{Lon: *lon, Lat: *lat},
			Zoom:     *zoom,
		})
		if err := a.Run(); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

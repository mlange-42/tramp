// TRAMP - Track and Route Analysis, Mapping and Planning.
package main

import (
	"flag"
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/unit"
	"github.com/mlange-42/tramp/internal/settings"
	"github.com/mlange-42/tramp/internal/ui"
	"github.com/mlange-42/tramp/internal/wms"
)

func main() {
	lon := flag.Float64("lon", 0, "longitude of the initial map center (default last position)")
	lat := flag.Float64("lat", 0, "latitude of the initial map center (default last position)")
	zoom := flag.Float64("zoom", 0, "initial zoom level (default last zoom)")
	wmsURL := flag.String("wms", "", "URL of an additional WMS service, must support EPSG:3857")
	wmsLayers := flag.String("layers", "", "comma-separated layer names for -wms")
	wmsFormat := flag.String("format", "image/png", "image format for -wms")
	wmsVersion := flag.String("version", "1.3.0", "WMS version for -wms")
	flag.Parse()

	state, exists, err := settings.Load()
	if err != nil {
		log.Printf("using default settings: %v", err)
	}
	flag.Visit(func(f *flag.Flag) {
		if state.View == nil && (f.Name == "lon" || f.Name == "lat" || f.Name == "zoom") {
			state.View = &settings.View{Zoom: 2}
		}
		switch f.Name {
		case "lon":
			state.View.Lon = *lon
		case "lat":
			state.View.Lat = *lat
		case "zoom":
			state.View.Zoom = *zoom
		}
	})

	wmsConfig := loadWMS()
	layers := wmsConfig.Maps
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
		state.Map = custom.Name
	}

	go func() {
		w := new(app.Window)
		w.Option(app.Title("TRAMP"), app.Size(unit.Dp(1100), unit.Dp(700)))
		a := ui.New(w, ui.Options{
			Layers:   layers,
			Overlays: wmsConfig.Overlays,
			State:    state,
		})
		if !exists && err == nil {
			saveSettings(a.State())
		}
		runErr := a.Run()
		if err == nil {
			// Don't overwrite a file we failed to read.
			saveSettings(a.State())
		}
		if runErr != nil {
			log.Fatal(runErr)
		}
		os.Exit(0)
	}()
	app.Main()
}

// loadWMS reads the WMS configuration file, or creates it with the built-in maps if it doesn't exist.
func loadWMS() ui.WMSConfig {
	var cfg ui.WMSConfig
	exists, err := settings.LoadFile(ui.WMSFile, &cfg)
	if err != nil {
		log.Printf("using default maps: %v", err)
		return ui.DefaultWMS()
	}
	if !exists {
		cfg = ui.DefaultWMS()
		if err := settings.SaveFile(ui.WMSFile, &cfg); err != nil {
			log.Printf("saving %s: %v", ui.WMSFile, err)
		}
		return cfg
	}
	if len(cfg.Maps) == 0 {
		log.Printf("no maps in %s, using default maps", ui.WMSFile)
		cfg.Maps = ui.DefaultLayers()
	}
	return cfg
}

func saveSettings(s settings.Settings) {
	if err := settings.Save(&s); err != nil {
		log.Printf("saving settings: %v", err)
	}
}

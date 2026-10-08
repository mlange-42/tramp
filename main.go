// TRAMP - Track and Route Analysis, Mapping and Planning.
package main

import (
	"log"
	"os"

	"gioui.org/app"
	"github.com/mlange-42/tramp/internal/settings"
	"github.com/mlange-42/tramp/internal/ui"
)

func main() {
	state, exists, err := settings.Load()
	if err != nil {
		log.Printf("using default settings: %v", err)
	}

	wmsConfig := loadWMS()

	go func() {
		w := new(app.Window)
		w.Option(ui.WindowOptions(state.Window)...)
		a := ui.New(w, ui.Options{
			Layers:   wmsConfig.Maps,
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

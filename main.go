// TRAMP - Track and Route Analysis, Mapping and Planning.
package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"

	"gioui.org/app"
	"github.com/mlange-42/tramp/internal/settings"
	"github.com/mlange-42/tramp/internal/ui"
)

// Log files in the settings directory.
const (
	logFile    = "tramp.log"
	oldLogFile = "tramp.old.log"
)

func main() {
	setupLog()

	state, exists, err := settings.Load()
	if err != nil {
		log.Printf("using default settings: %v", err)
	}

	wmsConfig := loadWMS()
	gradients := loadGradients()

	go func() {
		w := new(app.Window)
		w.Option(ui.WindowOptions(state.Window)...)
		a := ui.New(w, ui.Options{
			Layers:    wmsConfig.Maps,
			Overlays:  wmsConfig.Overlays,
			Gradients: gradients,
			State:     state,
			// Track files to open, e.g. from "Open with" or dropping files on the executable.
			Files: os.Args[1:],
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
			// TODO: Gio (v0.10.3) closes the window when D3D11 Present reports a lost device
			// (driver reset, sleep, RDP, ...). If "GPU device lost" recurs, recreate the window
			// on errors.Is(runErr, gpu.ErrDeviceLost) instead of exiting.
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

// loadGradients reads the gradients configuration file, or creates it with the built-in gradients if it doesn't exist.
// Invalid gradients are skipped. If there are no valid ones, the built-in gradients are used.
func loadGradients() []ui.Gradient {
	var cfg ui.GradientConfig
	exists, err := settings.LoadFile(ui.GradientsFile, &cfg)
	if err != nil {
		log.Printf("using default gradients: %v", err)
		return nil
	}
	if !exists {
		cfg = ui.DefaultGradients()
		if err := settings.SaveFile(ui.GradientsFile, &cfg); err != nil {
			log.Printf("saving %s: %v", ui.GradientsFile, err)
		}
	}
	grads, err := cfg.Parse()
	if err != nil {
		log.Printf("invalid gradients in %s: %v", ui.GradientsFile, err)
	}
	if len(grads) == 0 {
		log.Printf("no valid gradients in %s, using default gradients", ui.GradientsFile)
	}
	return grads
}

// setupLog writes the log and crash reports to [logFile] in the settings directory, in addition to stderr.
// Windows release builds have no console, so stderr goes nowhere.
// The log of the previous run is kept as [oldLogFile].
func setupLog() {
	dir, err := settings.Dir()
	if err == nil {
		err = os.MkdirAll(dir, 0o755)
	}
	if err != nil {
		log.Printf("creating log file: %v", err)
		return
	}
	path := filepath.Join(dir, logFile)
	// Fails if another instance has the file open. The log is then appended.
	_ = os.Rename(path, filepath.Join(dir, oldLogFile))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("creating log file: %v", err)
		return
	}
	// The file must come first: MultiWriter stops at the first failing writer,
	// and writing to stderr fails without a console.
	log.SetOutput(io.MultiWriter(f, os.Stderr))
	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		log.Printf("setting crash output: %v", err)
	}
}

func saveSettings(s settings.Settings) {
	if err := settings.Save(&s); err != nil {
		log.Printf("saving settings: %v", err)
	}
}

package ui

import (
	"errors"
	"log"
	"time"

	"gioui.org/app"
	"gioui.org/gpu"
)

// deviceLostRetry is the minimum time between two lost GPU devices for recreating the window.
// If the device is lost again sooner, TRAMP gives up, to not recreate windows in a loop.
const deviceLostRetry = 10 * time.Second

// recreateWindow opens a new window if the old one was closed because the GPU device was lost,
// and reports whether it did.
//
// TODO: remove this workaround when Gio handles a lost device on Present itself.
// In Gio v0.10.3, Window.validateAndProcess recovers from gpu.ErrDeviceLost in Refresh and frame,
// but not in Present, and closes the window. On Windows, D3D11 reports a lost device
// (driver reset, sleep, RDP, ...) mostly on Present. Reported upstream at https://todo.sr.ht/~eliasnaur/gio.
func (a *App) recreateWindow(err error) bool {
	if !errors.Is(err, gpu.ErrDeviceLost) {
		return false
	}
	now := time.Now()
	if !a.deviceLost.IsZero() && now.Sub(a.deviceLost) < deviceLostRetry {
		return false
	}
	a.deviceLost = now
	log.Printf("%v, recreating the window", err)

	// The placement of the old window was read when it was destroyed.
	w := new(app.Window)
	w.Option(WindowOptions(&a.win)...)
	a.window = w
	a.current.Store(w)
	return true
}

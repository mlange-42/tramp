//go:build windows

package ui

import (
	"image"
	"math"
	"syscall"
	"unsafe"

	"gioui.org/app"
	"gioui.org/io/event"
	"github.com/mlange-42/tramp/internal/settings"
)

// hasPlacement reports whether the window position can be saved and restored.
const hasPlacement = true

// platformWindow holds platform-specific window state.
type platformWindow struct {
	hwnd uintptr
}

const (
	swShowNormal    = 1
	swShowMinimized = 2
	swShowMaximized = 3

	wpfRestoreToMaximized = 2

	monitorDefaultToNull    = 0
	monitorDefaultToPrimary = 1
	monitorDefaultToNearest = 2

	swpNoSize     = 0x0001
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010

	mdtEffectiveDPI = 0
	defaultDPI      = 96
)

var (
	user32                 = syscall.NewLazyDLL("user32.dll")
	shcore                 = syscall.NewLazyDLL("shcore.dll")
	procGetWindowPlacement = user32.NewProc("GetWindowPlacement")
	procSetWindowPlacement = user32.NewProc("SetWindowPlacement")
	procSetWindowPos       = user32.NewProc("SetWindowPos")
	procMonitorFromRect    = user32.NewProc("MonitorFromRect")
	procGetMonitorInfo     = user32.NewProc("GetMonitorInfoW")
	procGetDpiForMonitor   = shcore.NewProc("GetDpiForMonitor")
)

type winPoint struct{ X, Y int32 }

type winRect struct{ Left, Top, Right, Bottom int32 }

func (r winRect) rect() image.Rectangle {
	return image.Rect(int(r.Left), int(r.Top), int(r.Right), int(r.Bottom))
}

func toWinRect(r image.Rectangle) winRect {
	return winRect{int32(r.Min.X), int32(r.Min.Y), int32(r.Max.X), int32(r.Max.Y)}
}

// windowPlacement is WINDOWPLACEMENT.
type windowPlacement struct {
	Length         uint32
	Flags          uint32
	ShowCmd        uint32
	MinPosition    winPoint
	MaxPosition    winPoint
	NormalPosition winRect
}

// monitorInfo is MONITORINFO.
type monitorInfo struct {
	Size    uint32
	Monitor winRect
	Work    winRect
	Flags   uint32
}

// event restores the window placement when the window is created,
// and reads it when the window is destroyed.
func (p *platformWindow) event(a *App, e event.Event) {
	ev, ok := e.(app.Win32ViewEvent)
	if !ok {
		return
	}
	if ev.HWND != 0 {
		p.hwnd = ev.HWND
		win := a.win
		if win.X != nil && win.Y != nil && win.Valid() {
			// Moving the window sends messages to the window thread, so it must run there.
			a.window.Run(func() { restorePlacement(ev.HWND, win) })
		} else if win.Maximized {
			a.window.Option(app.Maximized.Option())
		}
		return
	}
	if p.hwnd != 0 {
		// The window is being destroyed, but still exists.
		if w, ok := readPlacement(p.hwnd); ok {
			a.win = w
		}
		p.hwnd = 0
	}
}

// readPlacement returns the position, size and state of a window.
func readPlacement(hwnd uintptr) (settings.Window, bool) {
	p := windowPlacement{Length: uint32(unsafe.Sizeof(windowPlacement{}))}
	if ok, _, _ := procGetWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&p))); ok == 0 {
		return settings.Window{}, false
	}
	r := p.NormalPosition.rect().Add(workspaceOffset())
	scale := defaultDPI / float32(monitorDPI(monitorFromRect(r, monitorDefaultToNearest)))
	return settings.Window{
		Width:     float32(r.Dx()) * scale,
		Height:    float32(r.Dy()) * scale,
		X:         new(r.Min.X),
		Y:         new(r.Min.Y),
		Maximized: p.ShowCmd == swShowMaximized || p.ShowCmd == swShowMinimized && p.Flags&wpfRestoreToMaximized != 0,
	}, true
}

// restorePlacement moves the window to the saved position, size and state.
// If the title bar would not be on a monitor, e.g. after disconnecting one,
// the window is centered on the nearest monitor instead.
// The window is shrunk if it doesn't fit on its monitor.
func restorePlacement(hwnd uintptr, w settings.Window) {
	pos := image.Pt(*w.X, *w.Y)
	// Estimate the size with the primary monitor's DPI to find the monitor of the title bar.
	r := image.Rectangle{Min: pos, Max: pos.Add(dpToPx(w, monitorDPI(primaryMonitor())))}
	visible := false
	mon := monitorFromRect(titleBar(r), monitorDefaultToNull)
	if mon != 0 {
		r.Max = pos.Add(dpToPx(w, monitorDPI(mon)))
		visible = titleVisible(r, monitorWork(mon))
	}
	if !visible {
		mon = monitorFromRect(r, monitorDefaultToNearest)
		r.Max = pos.Add(dpToPx(w, monitorDPI(mon)))
	}
	r = fitWindow(r, monitorWork(mon), visible)

	// Move to the target monitor first, so that its DPI change doesn't scale the final size.
	_, _, _ = procSetWindowPos.Call(hwnd, 0, uintptr(r.Min.X), uintptr(r.Min.Y), 0, 0, swpNoSize|swpNoZOrder|swpNoActivate)

	p := windowPlacement{
		Length:         uint32(unsafe.Sizeof(windowPlacement{})),
		ShowCmd:        swShowNormal,
		NormalPosition: toWinRect(r.Sub(workspaceOffset())),
	}
	if w.Maximized {
		p.ShowCmd = swShowMaximized
	}
	_, _, _ = procSetWindowPlacement.Call(hwnd, uintptr(unsafe.Pointer(&p)))
}

func dpToPx(w settings.Window, dpi uint32) image.Point {
	s := float64(dpi) / defaultDPI
	return image.Pt(int(math.Round(float64(w.Width)*s)), int(math.Round(float64(w.Height)*s)))
}

func monitorFromRect(r image.Rectangle, flags uintptr) uintptr {
	wr := toWinRect(r)
	mon, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&wr)), flags)
	return mon
}

// primaryMonitor returns the primary monitor, which always contains the origin.
func primaryMonitor() uintptr {
	return monitorFromRect(image.Rect(0, 0, 1, 1), monitorDefaultToPrimary)
}

func getMonitorInfo(mon uintptr) (monitorInfo, bool) {
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	ok, _, _ := procGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&info)))
	return info, ok != 0
}

// monitorWork returns the work area of a monitor, i.e. without the taskbar.
func monitorWork(mon uintptr) image.Rectangle {
	info, _ := getMonitorInfo(mon)
	return info.Work.rect()
}

// workspaceOffset converts window placement coordinates to screen coordinates.
// They differ if the taskbar is at the top or left of the primary monitor.
func workspaceOffset() image.Point {
	info, ok := getMonitorInfo(primaryMonitor())
	if !ok {
		return image.Point{}
	}
	return info.Work.rect().Min.Sub(info.Monitor.rect().Min)
}

func monitorDPI(mon uintptr) uint32 {
	if mon == 0 || procGetDpiForMonitor.Find() != nil {
		return defaultDPI
	}
	var x, y uint32
	if hr, _, _ := procGetDpiForMonitor.Call(mon, mdtEffectiveDPI, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y))); hr != 0 || x == 0 {
		return defaultDPI
	}
	return x
}

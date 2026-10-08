package main

import "syscall"

// dpiAwarenessPerMonitorV2 is DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2.
const dpiAwarenessPerMonitorV2 = ^uintptr(3) // -4

// Gio makes only its window thread DPI aware. With display scaling, Windows
// then reports pointer positions in scaled coordinates while the mouse is
// captured, i.e. during drags, and the map jumps when panning starts.
// Making the whole process DPI aware avoids that.
func init() {
	proc := syscall.NewLazyDLL("user32.dll").NewProc("SetProcessDpiAwarenessContext")
	if proc.Find() == nil {
		_, _, _ = proc.Call(dpiAwarenessPerMonitorV2)
	}
}

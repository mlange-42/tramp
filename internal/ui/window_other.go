//go:build !windows

package ui

import "gioui.org/io/event"

// hasPlacement reports whether the window position can be saved and restored.
const hasPlacement = false

// platformWindow holds platform-specific window state.
type platformWindow struct{}

// platformEvent handles platform-specific window events.
func (a *App) platformEvent(event.Event) {}

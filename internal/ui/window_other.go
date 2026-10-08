//go:build !windows

package ui

import "gioui.org/io/event"

// hasPlacement reports whether the window position can be saved and restored.
const hasPlacement = false

// platformWindow holds platform-specific window state.
type platformWindow struct{}

// event handles platform-specific window events.
func (p *platformWindow) event(*App, event.Event) {}

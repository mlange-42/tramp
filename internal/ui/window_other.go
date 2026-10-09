//go:build !windows

package ui

import (
	"image/color"

	"gioui.org/io/event"
	"github.com/ncruces/zenity"
)

// hasPlacement reports whether the window position can be saved and restored.
const hasPlacement = false

// platformWindow holds platform-specific window state.
type platformWindow struct{}

// event handles platform-specific window events.
func (p *platformWindow) event(*App, event.Event) {}

// dialogOptions returns options to attach dialogs to the window.
func (p *platformWindow) dialogOptions() []zenity.Option { return nil }

// colorDialog returns a function that shows a color dialog, to be called from any goroutine.
// The function returns [zenity.ErrCanceled] if the user cancels.
func (p *platformWindow) colorDialog(initial color.NRGBA) func() (color.NRGBA, error) {
	return func() (color.NRGBA, error) {
		c, err := zenity.SelectColor(zenity.Title("Track color"), zenity.Color(initial))
		if err != nil {
			return color.NRGBA{}, err
		}
		nc := color.NRGBAModel.Convert(c).(color.NRGBA)
		nc.A = 0xff
		return nc, nil
	}
}

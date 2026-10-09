//go:build windows

package ui

import (
	"errors"
	"fmt"
	"image/color"
	"sync"
	"syscall"
	"unsafe"

	"github.com/ncruces/zenity"
)

// The color dialog calls ChooseColorW directly instead of using zenity.SelectColor.
// zenity keeps the dialog data on the goroutine stack and runs Go code in a hook during the dialog.
// If that grows and moves the stack, Windows writes the result to the old copy,
// and the dialog returns the initial color.

const (
	ccRGBInit   = 0x01
	ccFullOpen  = 0x02
	ccAnyColor  = 0x100
	actCtxFlags = 0x08 | 0x10 | 0x04 // RESOURCE_NAME_VALID | SET_PROCESS_DEFAULT | ASSEMBLY_DIRECTORY_VALID
	// shell32ManifestID is the resource ID of the manifest in shell32.dll that enables visual styles.
	shell32ManifestID = 124
)

var (
	comdlg32                  = syscall.NewLazyDLL("comdlg32.dll")
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procChooseColor           = comdlg32.NewProc("ChooseColorW")
	procCommDlgExtendedError  = comdlg32.NewProc("CommDlgExtendedError")
	procCreateActCtx          = kernel32.NewProc("CreateActCtxW")
	procReleaseActCtx         = kernel32.NewProc("ReleaseActCtx")
	procGetSystemDirectoryW   = kernel32.NewProc("GetSystemDirectoryW")
	visualStylesOnce          sync.Once
	errColorDialogUnavailable = errors.New("color dialog not available")
)

// chooseColor is the Win32 CHOOSECOLORW structure.
type chooseColor struct {
	StructSize   uint32
	Owner        uintptr
	Instance     uintptr
	RGBResult    uint32
	CustColors   *[16]uint32
	Flags        uint32
	CustData     uintptr
	Hook         uintptr
	TemplateName *uint16
}

// actCtx is the Win32 ACTCTXW structure.
type actCtx struct {
	Size                  uint32
	Flags                 uint32
	Source                *uint16
	ProcessorArchitecture uint16
	LangID                uint16
	AssemblyDirectory     *uint16
	ResourceName          uintptr
	ApplicationName       *uint16
	Module                uintptr
}

// colorDialog holds the dialog data in a global, i.e. on the heap, where it doesn't move.
// The custom colors are kept between dialogs.
var colorDialog struct {
	sync.Mutex
	args   chooseColor
	custom [16]uint32
}

func init() {
	for i := range colorDialog.custom {
		colorDialog.custom[i] = 0xffffff
	}
}

// colorDialog returns a function that shows a color dialog, to be called from any goroutine.
// The function returns [zenity.ErrCanceled] if the user cancels.
func (p *platformWindow) colorDialog(initial color.NRGBA) func() (color.NRGBA, error) {
	owner := p.hwnd
	return func() (color.NRGBA, error) {
		if procChooseColor.Find() != nil {
			return color.NRGBA{}, errColorDialogUnavailable
		}
		visualStylesOnce.Do(enableVisualStyles)

		d := &colorDialog
		d.Lock()
		defer d.Unlock()
		d.args = chooseColor{
			Owner:      owner,
			RGBResult:  uint32(initial.R) | uint32(initial.G)<<8 | uint32(initial.B)<<16,
			CustColors: &d.custom,
			Flags:      ccRGBInit | ccFullOpen | ccAnyColor,
		}
		d.args.StructSize = uint32(unsafe.Sizeof(d.args))

		if ok, _, _ := procChooseColor.Call(uintptr(unsafe.Pointer(&d.args))); ok == 0 {
			if code, _, _ := procCommDlgExtendedError.Call(); code != 0 {
				return color.NRGBA{}, fmt.Errorf("color dialog failed with error %#x", code)
			}
			return color.NRGBA{}, zenity.ErrCanceled
		}
		v := d.args.RGBResult
		return color.NRGBA{R: uint8(v), G: uint8(v >> 8), B: uint8(v >> 16), A: 0xff}, nil
	}
}

// enableVisualStyles makes the common dialogs use the current Windows theme
// instead of the classic look, by activating the manifest of shell32.dll for the process.
// This fails harmlessly if a process default was already set, e.g. by zenity.
func enableVisualStyles() {
	var buf [syscall.MAX_PATH]uint16
	n, _, _ := procGetSystemDirectoryW.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 || n >= uintptr(len(buf)) {
		return
	}
	source, err := syscall.UTF16PtrFromString("shell32.dll")
	if err != nil {
		return
	}
	ctx := actCtx{
		Flags:             actCtxFlags,
		Source:            source,
		AssemblyDirectory: &buf[0],
		ResourceName:      shell32ManifestID,
	}
	ctx.Size = uint32(unsafe.Sizeof(ctx))
	if h, _, _ := procCreateActCtx.Call(uintptr(unsafe.Pointer(&ctx))); h != ^uintptr(0) {
		_, _, _ = procReleaseActCtx.Call(h)
	}
}

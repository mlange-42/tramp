// Package assets holds the app logo and the Windows resource that gives the
// executable, window and taskbar their icon. Importing it links the resource.
//
// go generate renders logo.svg to tramp.ico and social.svg to social.png (needs Inkscape, see gen/main.go),
// and converts the icon to the resource.
package assets

import (
	_ "embed"
	"encoding/binary"
)

// Social is the social preview image with logo and title as PNG, shown in the About window.
//
//go:embed social.png
var Social []byte

// Icon is the app icon in all sizes, as written by gen: a Windows ICO file with a PNG per size.
//
//go:embed tramp.ico
var Icon []byte

// IconPNG returns the PNG of the smallest size in [Icon] that is at least size pixels, or of the largest,
// and its size. It returns nil if the icon can't be read.
func IconPNG(size int) (png []byte, pngSize int) {
	le := binary.LittleEndian
	if len(Icon) < 6 {
		return nil, 0
	}
	n := int(le.Uint16(Icon[4:]))
	for i := range n {
		e := Icon[min(len(Icon), 6+16*i):]
		if len(e) < 16 {
			break
		}
		s := int(e[0])
		if s == 0 {
			s = 256
		}
		start, end := int(le.Uint32(e[12:])), int(le.Uint32(e[12:]))+int(le.Uint32(e[8:]))
		if end > len(Icon) {
			continue
		}
		// The best is the smallest at least size, otherwise the largest.
		better := png == nil ||
			(s >= size && (pngSize < size || s < pngSize)) ||
			(s < size && pngSize < size && s > pngSize)
		if better {
			png, pngSize = Icon[start:end], s
		}
	}
	return png, pngSize
}

//go:generate go run ./gen
//go:generate go run github.com/akavel/rsrc@v0.10.2 -ico tramp.ico -o rsrc_windows_amd64.syso

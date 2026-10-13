// Package assets holds the app logo and the Windows resource that gives the
// executable, window and taskbar their icon. Importing it links the resource.
//
// go generate renders logo.svg to tramp.ico and social.svg to social.png (needs Inkscape, see gen/main.go),
// and converts the icon to the resource.
package assets

import _ "embed"

// Social is the social preview image with logo and title as PNG, shown in the About window.
//
//go:embed social.png
var Social []byte

//go:generate go run ./gen
//go:generate go run github.com/akavel/rsrc@v0.10.2 -ico tramp.ico -o rsrc_windows_amd64.syso

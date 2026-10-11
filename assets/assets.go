// Package assets holds the app logo and the Windows resource that gives the
// executable, window and taskbar their icon. Importing it links the resource.
//
// go generate renders logo.svg to tramp.ico (needs Inkscape, see gen/main.go)
// and converts that to the resource.
package assets

//go:generate go run ./gen
//go:generate go run github.com/akavel/rsrc@v0.10.2 -ico tramp.ico -o rsrc_windows_amd64.syso

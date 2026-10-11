// Package assets holds the app logo and the Windows resource that gives the
// executable, window and taskbar their icon. Importing it links the resource.
//
// tramp.ico is logo.svg exported at 16 to 256 px.
package assets

//go:generate go run github.com/akavel/rsrc@v0.10.2 -ico tramp.ico -o rsrc_windows_amd64.syso

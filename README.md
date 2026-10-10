# TRAMP

[![Tests](https://github.com/mlange-42/tramp/actions/workflows/tests.yml/badge.svg)](https://github.com/mlange-42/tramp/actions/workflows/tests.yml)

**Track and Route Analysis, Mapping and Planning**

A free, open-source tool to visualize GPS tracks and plan routes on WMS maps. Written in [Go](https://go.dev), for Linux, Windows and MacOS.

> **Status:** early development. Viewing GPX tracks and editing routes and waypoints work, but there is no release yet and things may change.

## Features

- [x] GPX tracks on WMS and XYZ maps
- [x] Coloring by speed, elevation or slope
- [x] Profile chart, linked to the map
- [x] Route and waypoint editing
- [ ] Elevation for routes
- [ ] Waypoint symbols
- [ ] Weather overlays
- [ ] Offline tile cache

## Installation

### With Go

```sh
go install github.com/mlange-42/tramp@latest
```

TRAMP uses [Gio](https://gioui.org) for its GUI.
On Linux and macOS, this requires a C compiler.
On Linux, also the development headers listed in the [Gio Linux install docs](https://gioui.org/doc/install/linux) are required.

On Windows, add `-ldflags="-H windowsgui"` to prevent a console window from opening.

On Linux, the file dialog requires `zenity`, `kdialog` or a similar tool, which most desktops have installed.

### Precompiled binary

Binaries for Windows, Linux and macOS will be available on the GitHub [releases page](https://github.com/mlange-42/tramp/releases). [TODO].

## Usage

Run `tramp` to open the map, optionally with GPX files to open:

```sh
tramp morning.gpx evening.gpx
```

## Documentation

- [Usage](docs/usage.md): map, opening files, coloring tracks
- [Editing](docs/editing.md): creating and editing routes and waypoints
- [Configuration](docs/configuration.md): settings, maps and color gradients

## License

TRAMP and all its sources and documentation are distributed under the [MIT license](https://github.com/mlange-42/tramp/blob/main/LICENSE).

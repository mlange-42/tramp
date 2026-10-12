<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/logo-title-dark.svg">
  <img alt="TRAMP – Track and Route Analysis, Mapping and Planning" src="assets/logo-title-light.svg">
</picture>

A free, open-source tool to visualize GPS tracks and plan routes on WMS maps. Written in [Go](https://go.dev), for Linux, Windows and MacOS.

[![Tests](https://github.com/mlange-42/tramp/actions/workflows/tests.yml/badge.svg)](https://github.com/mlange-42/tramp/actions/workflows/tests.yml)
[![codecov](https://codecov.io/gh/mlange-42/tramp/graph/badge.svg?token=SIVgPByIsM)](https://codecov.io/gh/mlange-42/tramp)
[![GitHub](https://img.shields.io/badge/github-repo-blue?logo=github)](https://github.com/mlange-42/tramp)
[![MIT license](https://img.shields.io/badge/MIT-brightgreen?label=license)](https://github.com/mlange-42/tramp/blob/main/LICENSE)

> **Status:** early development. Viewing GPX tracks and editing routes and waypoints work, but there is no release yet and things may change.

## Features

- [x] GPX tracks on WMS and XYZ maps
- [x] Coloring by speed, elevation or slope
- [x] Profile chart, linked to the map
- [x] Route and waypoint editing
- [x] Live GPS position from USB and Bluetooth receivers
- [x] Search for places and coordinates
- [ ] FIT and KML/KMZ files
- [ ] Elevation for routes
- [ ] Weather overlays
- [ ] Offline tile cache

## Installation

### Precompiled binary

Binaries for Windows, Linux and macOS will be available on the GitHub [releases page](https://github.com/mlange-42/tramp/releases). [TODO].

### With Go

```sh
go install github.com/mlange-42/tramp@latest
```

TRAMP uses [Gio](https://gioui.org) for its GUI.
On Linux and macOS, this requires a C compiler.
On Linux, also the development headers listed in the [Gio Linux install docs](https://gioui.org/doc/install/linux) are required.

On Windows, add `-ldflags="-H windowsgui"` to prevent a console window from opening.

### Requirements

On Linux, the file dialog requires `zenity`, `kdialog` or a similar tool, which most desktops have installed.

## Documentation

- [Usage](docs/usage.md): map, search, opening files, coloring tracks, live GPS position
- [Editing](docs/editing.md): creating and editing routes and waypoints
- [Configuration](docs/configuration.md): settings, maps and color gradients

## Development

TRAMP is designed and maintained by [@mlange-42](https://github.com/mlange-42).
Most of the implementation was written with [Claude Code](https://claude.com/claude-code)
(Anthropic) as a pair-programming assistant.

Architecture, feature design, and technology choices are mine;
every change is reviewed, tested against real-world data,
and merged by me via pull request.

## License

TRAMP and all its sources and documentation are distributed under the [MIT license](https://github.com/mlange-42/tramp/blob/main/LICENSE).

The waypoint symbol icons are from [Maki](https://github.com/mapbox/maki), released under [CC0](internal/symbols/maki/LICENSE.txt).

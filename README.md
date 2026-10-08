# TRAMP

[![Tests](https://github.com/mlange-42/tramp/actions/workflows/tests.yml/badge.svg)](https://github.com/mlange-42/tramp/actions/workflows/tests.yml)

**Track and Route Analysis, Mapping and Planning**

A free, open-source tool to visualize GPS tracks and plan routes on WMS maps. Written in [Go](https://go.dev), for Linux, Windows and MacOS.

> **Status:** just started, not usable yet.

## Features (planned)

- Tracks drawn on WMS background maps, colored by speed, elevation, slope, ...
- Profile view (metric over distance), linked to the map
- Offline tile cache (optional)
- Route planning with elevation profile

## Installation

### With Go

```sh
go install github.com/mlange-42/tramp@latest
```

TRAMP uses [Gio](https://gioui.org) for its GUI.
On Linux and macOS, this requires a C compiler.
On Linux, also the development headers listed in the [Gio Linux install docs](https://gioui.org/doc/install/linux) are required.

### Precompiled binary

Binaries for Windows, Linux and macOS will be available on the GitHub [releases page](https://github.com/mlange-42/tramp/releases). [TODO].

## Usage

Run `tramp` to open the map. Drag to pan, scroll to zoom.
The background map and overlays like hillshading can be selected in the toolbar.

```sh
tramp -lon 11.58 -lat 48.14 -zoom 13
```

Any WMS service that supports EPSG:3857 can be added:

```sh
tramp -wms https://ows.terrestris.de/osm/service -layers SRTM30-Colored-Hillshade
```

Run `tramp -h` for all options.

## License

TRAMP and all its sources and documentation are distributed under the [MIT license](https://github.com/mlange-42/tramp/blob/main/LICENSE).


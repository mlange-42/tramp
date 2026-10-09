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

On Windows, add `-ldflags="-H windowsgui"` to prevent a console window from opening.

### Precompiled binary

Binaries for Windows, Linux and macOS will be available on the GitHub [releases page](https://github.com/mlange-42/tramp/releases). [TODO].

## Usage

Run `tramp` to open the map. Drag to pan, scroll to zoom.

The background map and overlays like hillshading can be selected in the toolbar.
Further maps can be added in the [maps configuration](#maps).

Open GPX files with the *Open* button or Ctrl+O, or pass them on the command line:

```sh
tramp morning.gpx evening.gpx
```

Opened files are listed in the side panel, with length, duration and sampling interval.
Click a file to zoom to it, use the checkbox to show or hide it, and × to close it.
Files with several tracks, routes or waypoint sets can be expanded to show or hide them individually.

Use *Color by* in the toolbar to color all tracks by speed, elevation or slope, with a choice of color gradients.
The color range covers the values of all visible tracks, ignoring the most extreme 2% at each end, and is shown in a legend on the map.
Tracks and routes without the required data, like routes without times for speed, keep their own color.

On Linux, the file dialog requires `zenity`, `kdialog` or a similar tool, which most desktops have installed.

### Settings

TRAMP remembers the opened files, the selected map, overlays, map position and window size in a YAML file `settings.yaml` in the user's config directory.
Some settings can only be changed in this file:

```yaml
tile_cache: 2048      # map tiles kept in memory, about 256 KB each
style:
  track_width: 3      # line width of tracks
  route_width: 3      # line width of routes
  waypoint_size: 8    # diameter of waypoint dots
```

Sizes are in device-independent pixels.

The file is located in:

- Windows: `%AppData%\tramp`
- Linux: `$XDG_CONFIG_HOME/tramp` or `~/.config/tramp`
- macOS: `~/Library/Application Support/tramp`

Errors and crash reports are written to `tramp.log` in the same directory.
The log of the previous run is kept as `tramp.old.log`.

### Maps

The available background maps and overlays are configured in `wms.yaml` in the same directory.
It is created with the built-in maps on the first start.
WMS services must support EPSG:3857.
XYZ tile services (`type: xyz`) are given by a URL template with `{z}`, `{x}` and `{y}`.

```yaml
maps:
  - name: OpenStreetMap
    url: https://tiles.maps.eox.at/wms
    layers: osm_3857
    format: image/png
    version: 1.3.0
    attribution: Data © OpenStreetMap contributors and others, Rendering © EOX
  - name: OpenTopoMap
    type: xyz
    url: https://tile.opentopomap.org/{z}/{x}/{y}.png
    max_zoom: 17  # highest level served, the map is magnified beyond
    attribution: Map data © OpenStreetMap contributors, SRTM | Map style © OpenTopoMap (CC-BY-SA)
overlays:
  - name: Hillshade (SRTM)
    url: https://ows.terrestris.de/osm/service
    layers: SRTM30-Hillshade
    format: image/png
    version: 1.3.0
    attribution: SRTM, terrestris
    opacity: 0.6  # 0 or missing means opaque
    shade: true   # turn a grayscale relief into a transparent shading
```

Delete the file to restore the built-in maps.

## License

TRAMP and all its sources and documentation are distributed under the [MIT license](https://github.com/mlange-42/tramp/blob/main/LICENSE).

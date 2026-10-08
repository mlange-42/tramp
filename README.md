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

Further maps can be added in the [maps configuration](#maps).

### Settings

TRAMP remembers the selected map, overlays and map position in a YAML file `settings.yaml` in the user's config directory.
The file also contains `tile_cache`, the number of map tiles kept in memory (default 2048, about 512 MB).

The file is located in:

- Windows: `%AppData%\tramp`
- Linux: `$XDG_CONFIG_HOME/tramp` or `~/.config/tramp`
- macOS: `~/Library/Application Support/tramp`

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


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

### Editing

GPX files with routes and waypoints can be edited, files with recorded tracks are read-only.
A file can contain several routes, as well as waypoints.
Create a new file with *New* or Ctrl+N, or click the pencil of an opened file to edit it.
One file at a time is in edit mode. Unsaved changes are marked with `*` after the file name.

In edit mode, the toolbar shows the edit tools, also selected with the keys S, W and R:

- **Select**: click a point to select it, drag it to move it, and press Del to delete it.
  Drag or click the small circles between route points to insert a point.
  Route points dropped on a waypoint snap to it and are linked to it; hold Shift to not snap.
  Esc or a right click clears the selection.
- **Waypoint**: click to add a waypoint.
- **Route**: click to add points to a new route. Click the first or last point of a route,
  or select the route in the side panel, to continue it.
  Esc, Enter, a right click or a double click finishes the route, Del removes the last point.
  Clicking a waypoint adds it to the route, linked to it (see below); unnamed waypoints get a name like `WP001`.
  The pointer snaps to waypoints, which are highlighted. Hold Shift to place a point without snapping.

Hover a tool for a short help.
Moved and added points have no elevation.

The names of the points of the edited file are shown on the map.
Below the file list, the *Properties* box shows the name and description of the selected route,
route point or waypoint. Changes apply on Enter, Tab or when clicking elsewhere, and Esc reverts them.
The box also has a button to delete the whole route.

Many GPS devices build routes from stored waypoints, and write each route point
with the same name and position as its waypoint.
TRAMP treats such points as linked: they share one handle on the map and are moved together.
Hold Shift while dragging a linked route point to move it alone.
Released with Shift held, it is unlinked and loses the waypoint's name, description and symbol.
Deleting a linked route point removes it from the route only.

Save with Ctrl+S. Undo and redo with the toolbar buttons, or with Ctrl+Z and Ctrl+Y or Ctrl+Shift+Z.
When leaving edit mode, closing the file or closing TRAMP with unsaved changes,
TRAMP asks whether to save or discard them.
Before overwriting a file written by other software for the first time, TRAMP asks for confirmation,
as content it doesn't read, like vendor extensions, is lost.

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
A map with `type: none` has no tiles and shows only the overlays.

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
  - name: None
    type: none
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

### Gradients

The color gradients for coloring tracks by speed, elevation or slope are configured in `gradients.yaml` in the same directory.
It is created with the built-in gradients on the first start.
Each gradient has a unique name and at least two equally spaced colors, from low to high values:

```yaml
gradients:
  - name: Viridis
    colors: ['#440154', '#3b528b', '#21918c', '#5ec962', '#fde725']
  - name: Cold–Hot
    colors: ['#0000ff', '#ffffff', '#ff0000']
```

Invalid gradients are skipped and reported in the log file.
Delete the file to restore the built-in gradients.

## License

TRAMP and all its sources and documentation are distributed under the [MIT license](https://github.com/mlange-42/tramp/blob/main/LICENSE).

# Configuration

TRAMP keeps its configuration in YAML files in the user's config directory:

- Windows: `%AppData%\tramp`
- Linux: `$XDG_CONFIG_HOME/tramp` or `~/.config/tramp`
- macOS: `~/Library/Application Support/tramp`

Errors and crash reports are written to `tramp.log` in the same directory.
The log of the previous run is kept as `tramp.old.log`.

## Settings

TRAMP remembers the opened files, the selected map, overlays, map position and window size in `settings.yaml`.
Some settings can only be changed in this file:

```yaml
tile_cache: 2048           # map tiles kept in memory, about 256 KB each
style:
  track_width: 3           # line width of tracks
  route_width: 3           # line width of routes
  waypoint_size: 8         # diameter of waypoint dots
  muted_color: '#808080c0' # color of the selected track outside the zoomed chart range,
                           # and of segments without value when coloring by value
```

Sizes are in device-independent pixels.
Colors are given like `#808080`, or with opacity like `#808080c0`.

## Maps

The available background maps and overlays are configured in `wms.yaml`.
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

## Gradients

The color gradients for coloring tracks by speed, elevation or slope are configured in `gradients.yaml`.
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

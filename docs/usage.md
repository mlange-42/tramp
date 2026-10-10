# Usage

Run `tramp` to open the map. Drag to pan, scroll to zoom.

The background map and overlays like hillshading can be selected in the toolbar.
Further maps can be added in the [maps configuration](configuration.md#maps).

## Opening files

Open GPX files with the folder button or Ctrl+O, or pass them on the command line:

```sh
tramp morning.gpx evening.gpx
```

Opened files are listed in the side panel, with length, duration and sampling interval.
Click a file to zoom to it, use the checkbox to show or hide it, and × to close it.
All buttons show a short help when hovered.
Files with several tracks, routes or waypoint sets can be expanded to show or hide them individually.

To create and edit routes and waypoints, see [Editing](editing.md).

## Coloring

Use the palette drop-down in the toolbar to color all tracks by speed, elevation or slope, with a choice of color gradients.
The color range covers the values of all visible tracks, ignoring the most extreme 2% at each end, and is shown in a legend on the map.
Tracks and routes without the required data, like routes without times for speed, keep their own color.

The gradients can be changed in the [gradients configuration](configuration.md#gradients).

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
The waypoints of a file are always listed first, and drawn above its tracks and routes.

To create and edit routes and waypoints, see [Editing](editing.md).

Waypoints with a symbol are drawn as markers with an icon.
GPX does not standardize symbol names; TRAMP knows the common outdoor symbols in the names used by Garmin devices,
like `Summit`, `Campground` or `Flag, Blue`, and some aliases like `Peak` or `Hut`, ignoring case and punctuation.
Unknown symbols are drawn with a generic pin icon, waypoints without symbol as plain dots.
Route points are drawn only if they have a symbol.

## Coloring

Use the palette drop-down in the toolbar to color all tracks by speed, elevation or slope, with a choice of color gradients.
The color range covers the values of all visible tracks, ignoring the most extreme 2% at each end, and is shown in a legend on the map.
Tracks and routes without the required data, like routes without times for speed, keep their own color.

The gradients can be changed in the [gradients configuration](configuration.md#gradients).

## Live GPS position

The round button at the bottom right of the map shows the live position of a GPS receiver,
like a handheld GPS or a GPS mouse connected by USB or Bluetooth.
The map follows the position until it is dragged; click the button again to follow it again,
and once more while following to turn the GPS off.
The status bar shows the position, its estimated accuracy and the speed.
While the receiver has no fix, the last position is gray.

TRAMP reads NMEA 0183, which almost all GPS receivers can send over a serial port.
It searches the receiver on all USB and Bluetooth serial ports at common baud rates, which takes a few seconds.
The port and baud rate can also be set in the [settings](configuration.md#settings).
Built-in serial ports on Linux (`/dev/ttyS*`) are only used if set there.
Some devices need to be set up to send NMEA:

- **Locosys GT-31** sends NMEA at 4800 baud while switched on and not in NaviLink mode.
- **Garmin** handhelds with a serial cable: set the interface to *NMEA In/NMEA Out* in the system setup.
  Garmin devices with USB use their own protocol; on Windows, install Garmin's *Spanner* driver,
  set the interface to *Garmin Spanner*, and use the virtual serial port it creates.
- **GPS mice** like GlobalSat BU-353 or u-blox receivers send NMEA right away.
- **Bluetooth** receivers like Garmin GLO, Bad Elf or Qstarz must be paired first.
  On Windows, use the *outgoing* serial port of the device (Bluetooth settings → More Bluetooth settings → COM Ports).
  On Linux, bind it to a port with `rfcomm bind`.

Bluetooth Low Energy devices without a serial port, and smartphones, are not supported.

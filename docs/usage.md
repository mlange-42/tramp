# Usage

Run `tramp` to open the map. Drag to pan, scroll to zoom.

The background map and overlays like hillshading can be selected in the toolbar.
Further maps can be added in the [maps configuration](configuration.md#maps).

## Search

The search field at the right of the toolbar (Ctrl+F) moves the map to a place or to coordinates.
Press Enter to search; the found position is marked on the map until the field is cleared with × or Esc.

Coordinates are recognized in the common formats, latitude first unless marked with N/S/E/W:

```text
51.34, 12.37
51.34N 12.37E
N 51° 20.4' E 12° 22.2'
51°20'24"N 12°22'12"E
```

Anything else is searched as a place name, by default with [Photon](https://photon.komoot.io) by komoot,
based on OpenStreetMap data.
Places are searched while typing. Places in about the area shown on the map are preferred,
but well-known places elsewhere are still found.
The matching places are listed below the field, with their beeline distance from the map center.
Click one, or select it with the arrow keys and press Enter, to show it.
Searching place names needs an internet connection.

Instead of Photon, [Nominatim](https://nominatim.org), the search service of OpenStreetMap, can be used,
or another server of either service, see the [settings](configuration.md#settings).
With Nominatim, places are only searched on Enter, and nearby places are not preferred.

Both public servers are free, but ask for moderate use:
Nominatim allows at most one search per second for all users of an application together,
and forbids searching while typing; Photon throttles extensive use.
TRAMP remembers the results of recent searches, so repeated searches don't reach the server again.

## Routing

The directions button left of the search field (Ctrl+D) opens a panel for finding a route between two points.
Click the map to set the start (A), and again to set the target (B),
or type a place or coordinates in their fields, like in the [search](#search).
The route is shown on the map, with its length and the estimated travel time.
Drag A or B to move them; the route is found again when released.
Drag the route itself to add a via point there, to make the route pass through another place.
The ⊕ button adds a via point before the target, set by the next click on the map or by searching it.
Via points are listed between A and B. They can be dragged as well, and ⊖ or a click on the map removes them.
Clearing a field with × sets its point again by the next click on the map.
Select the means of travel in the panel: on foot, by bike or by car.
The swap button reverses the route, with its via points, and the bin button removes it to start a new one.
Close the panel with × or Esc. The route is kept, also when TRAMP is closed, and shown again when the panel is opened.

Routes are found with [Valhalla](https://valhalla1.openstreetmap.de) by default, or [OSRM](https://routing.openstreetmap.de),
on public servers of [FOSSGIS e.V.](https://www.fossgis.de) using OpenStreetMap data,
see the [settings](configuration.md#settings). Routing needs an internet connection.
The Valhalla server finds routes up to 100 km on foot and 150 km by bike.
Found an error in the route? *Report a map error* in the panel leads to OpenStreetMap, where anyone can fix the map.

The route is independent of the routes in GPX files.

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
The circle around the position shows the accuracy reported by the receiver.
Most receivers don't report it; it is then estimated from the satellite geometry (HDOP),
generously, as reflections near buildings or indoors cause errors the HDOP doesn't show.
While the receiver has no fix, the last position is gray.

TRAMP reads NMEA 0183, which almost all GPS receivers can send over a serial port.
It searches the receiver on all USB and Bluetooth serial ports at common baud rates, which takes a few seconds.
The port and baud rate can also be set in the [settings](configuration.md#settings).
Built-in serial ports on Linux (`/dev/ttyS*`) are only used if set there.
Some devices need to be set up to send NMEA:

- **Locosys GT-31 / BGT-31** sends NMEA at 4800 baud while switched on and not in NaviLink mode.
- **Garmin** handhelds with a serial cable: set the interface to *NMEA In/NMEA Out* in the system setup.
  Garmin devices with USB use their own protocol; on Windows, install Garmin's *Spanner* driver,
  set the interface to *Garmin Spanner*, and use the virtual serial port it creates.
- **GPS mice** like GlobalSat BU-353 or u-blox receivers send NMEA right away.
- **Bluetooth** receivers like Garmin GLO, Bad Elf or Qstarz must be paired first.
  On Windows, use the *outgoing* serial port of the device (Bluetooth settings → More Bluetooth settings → COM Ports).
  On Linux, bind it to a port with `rfcomm bind`.

Bluetooth Low Energy devices without a serial port are not supported.

### Network sources

Instead of a serial port, the GPS `port` in the [settings](configuration.md#settings) can be a network source.
Network sources are never searched automatically.

- `tcp://host:port` connects to a server sending NMEA, like a phone app sharing its GPS
  or a NMEA multiplexer on a boat (often port 10110).
- `udp://:port` receives NMEA sent as UDP datagrams to this computer, on all network interfaces.
  Use `udp://192.168.1.10:port` to listen on one interface only.
- `gpsd://host` or `gpsd://host:port` connects to [gpsd](https://gpsd.io) (default port 2947) and asks it for NMEA.
  gpsd supports many receivers, including Garmin USB devices on Linux.

On a phone, use an app that shares the position as NMEA over TCP or UDP, and enter its address.
Both devices must be in the same network, and a firewall may need to allow incoming UDP.

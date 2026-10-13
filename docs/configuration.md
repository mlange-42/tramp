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
tile_cache: 512            # memory for map tiles in MB, about 256 KB per tile
elevation_tile_cache: 128  # memory for elevation tiles in MB
disk_cache:
  dir: ''                  # directory of the tile cache; empty for the default, see below
  max_size: 2048           # largest size of the map tiles stored on disk in MB; negative to store none
  elevation_max_size: 1024 # largest size of the elevation tiles stored on disk in MB; negative to store none
  max_age: 30              # days after which a stored tile is checked with the server again
style:
  track_width: 3           # line width of tracks
  route_width: 3           # line width of routes
  waypoint_size: 8         # diameter of waypoint dots
  symbol_size: 18          # diameter of the markers of waypoints with a symbol
  muted_color: '#808080c0' # color of the selected track outside the zoomed chart range,
                           # and of segments without value when coloring by value
gps:
  port: ''                 # serial port of the GPS receiver, like COM5 or /dev/ttyUSB0; empty to search;
                           # or a network source like tcp://192.168.1.5:10110, udp://:10110 or gpsd://localhost
  baud: 0                  # baud rate, like 4800; 0 to try common rates
search:
  service: photon          # search service for place names: photon or nominatim
  url: ''                  # search endpoint of another server; empty for the public server of the service
  limit: 10                # most places listed for a search, up to 40
routing:
  service: valhalla        # routing service: valhalla or osrm
  url: ''                  # endpoint of another server; empty for the public FOSSGIS server of the service
  profile: foot            # last selected means of travel: foot, bike or car
  tolerance: 100           # largest distance in m between a saved route and the lines between its points;
                           # points are added where needed; negative to add none
  slight_angle: 22.5       # smallest change of direction in degrees for a slight turn to get a point
                           # in a saved route; negative for none
  points: []               # start, via points and target of the last route, with lon, lat and the name of searched places
elevation:
  source: mapterhorn       # source of terrain elevations: mapterhorn, aws, or none to turn them off
  url: ''                  # tile URL template of another service, like https://tiles.example.com/{z}/{x}/{y}.png
  encoding: ''             # for another service: terrarium or mapbox
  tile_size: 0             # for another service: edge length of the tiles in pixels
  max_zoom: 0              # for another service: highest tile level
```

Sizes are in device-independent pixels.

Map and elevation tiles are stored on disk, so that maps already viewed are shown without an internet connection,
and elevations there are known.
They are kept in the user's cache directory, in the folders `tiles` and `elevation`, unless `disk_cache.dir` is set:

- Windows: `%LocalAppData%\tramp`
- Linux: `$XDG_CACHE_HOME/tramp` or `~/.cache/tramp`
- macOS: `~/Library/Caches/tramp`

When the map tiles exceed `max_size`, or the elevation tiles `elevation_max_size`, the oldest ones are deleted.
Tiles older than `max_age` are checked with the server when they are shown, and still used when it can't be reached.
The cache can be deleted at any time while TRAMP is closed.
See [Live GPS position](usage.md#live-gps-position) for the GPS receiver.
See [Search](usage.md#search) for the search services, and [Routing](usage.md#routing) for routing.
Routes are found with the public servers of [FOSSGIS e.V.](https://www.fossgis.de),
[Valhalla](https://valhalla1.openstreetmap.de) or [OSRM](https://routing.openstreetmap.de),
which allow at most one request per second.
The Valhalla server finds routes up to 100 km on foot and 150 km by bike; the OSRM servers have no such limit.
For another Valhalla server, `url` is its route endpoint, like `http://localhost:8002/route`.
For another OSRM server, `url` is its address without `/route/v1`, like `http://localhost:5000`;
`{profile}` in it is replaced by `foot`, `bike` or `car`, for servers with one address per profile
like the public ones at `https://routing.openstreetmap.de/routed-{profile}`.
Terrain elevations come from Terrain-RGB tiles, map tiles with the elevation encoded in the colors of their pixels:
[Mapterhorn](https://mapterhorn.com) by default, worldwide and with detailed national elevation models in many countries,
from the open-data sources listed on its [attribution page](https://mapterhorn.com/attribution),
or [AWS Terrain Tiles](https://registry.opendata.aws/terrain-tiles/), worldwide and mostly based on SRTM.
Mapterhorn is much more accurate, especially for summits and steep terrain.
Like the map and routing services, it logs IP addresses and user agents of requests, see its [privacy policy](https://mapterhorn.com/privacy-policy/).
For another service, set `url`, and `encoding`, `tile_size` and `max_zoom` where they differ from the source.
With `source: none`, no elevations are downloaded.
Colors are given like `#808080`, or with opacity like `#808080c0`.

## Maps

The available background maps and overlays are configured in `wms.yaml`.
It is created with the built-in maps on the first start.
WMS services must support EPSG:3857.
XYZ tile services (`type: xyz`) are given by a URL template with `{z}`, `{x}` and `{y}`.
A map with `type: none` has no tiles and shows only the overlays.
Entries like `- section: World` start a section with this heading in the list of maps or overlays.
The first map is selected by default.

```yaml
maps:
  - section: World
  - name: OpenStreetMap
    description: OpenStreetMap rendered by EOX. Worldwide.  # tooltip, optional
    url: https://tiles.maps.eox.at/wms
    layers: osm_3857
    format: image/png
    version: 1.3.0
    attribution: Data © OpenStreetMap contributors and others, Rendering © EOX
    attribution_short: © OpenStreetMap · EOX  # shown on the map, optional
    attribution_url: https://maps.eox.at       # terms or credits of the data, optional
  - name: OpenTopoMap
    type: xyz
    url: https://tile.opentopomap.org/{z}/{x}/{y}.png
    max_zoom: 17  # highest level served, the map is magnified beyond
    attribution: Map data © OpenStreetMap contributors, SRTM | Map style © OpenTopoMap (CC-BY-SA)
    attribution_short: © OpenStreetMap · OpenTopoMap
    attribution_url: https://opentopomap.org/about
  - name: None
    type: none
overlays:
  - section: Hillshade
  - name: Hillshade (SRTM)
    url: https://ows.terrestris.de/osm/service
    layers: SRTM30-Hillshade
    format: image/png
    version: 1.3.0
    attribution: SRTM, terrestris
    attribution_short: SRTM · terrestris
    attribution_url: https://www.terrestris.de
    opacity: 0.6  # 0 or missing means opaque
    shade: true   # turn a grayscale relief into a transparent shading
  - section: Weather
  - name: Precipitation radar
    url: https://maps.dwd.de/geoserver/ows
    layers: dwd:Niederschlagsradar
    format: image/png
    version: 1.3.0
    attribution: © Deutscher Wetterdienst
    attribution_short: © DWD
    attribution_url: https://www.dwd.de/EN/service/copyright/copyright_node.html
    refresh: 5m   # load the tiles again every 5 minutes
```

Overlays that change over time, like weather radar, can be loaded again regularly with `refresh`,
given like `30s`, `5m` or `1h`. The shortest interval is one minute.
Outdated tiles stay visible until the new ones arrive.
Such overlays are not stored in the disk cache (`disk_cache`), so that old weather is never shown as current.

The status bar shows the short credits of the selected map, the enabled overlays and the elevation source,
with parts separated by ` · ` shown only once, like `© OpenStreetMap · CyclOSM · Waymarked Trails`.
Without `attribution_short`, the full `attribution` is shown.

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

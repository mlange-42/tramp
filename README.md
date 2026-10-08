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

### Precompiled binary

Binaries for Windows, Linux and macOS will be available on the GitHub [releases page](https://github.com/<user>/tramp/releases). [TODO].

## License

TRAMP and all its sources and documentation are distributed under the [MIT license](https://github.com/mlange-42/tramp/blob/main/LICENSE).


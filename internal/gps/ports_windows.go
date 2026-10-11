package gps

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// bluetoothPorts returns the names of the Bluetooth serial ports, like COM7.
// Windows lists them as \Device\BthModemN in the serial port map of the registry.
// Each paired device has an incoming and an outgoing port, of which only the outgoing one receives data.
func bluetoothPorts() map[string]bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DEVICEMAP\SERIALCOMM`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer func() { _ = k.Close() }()
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return nil
	}
	ports := map[string]bool{}
	for _, n := range names {
		if !strings.Contains(strings.ToLower(n), "bthmodem") {
			continue
		}
		if v, _, err := k.GetStringValue(n); err == nil {
			ports[v] = true
		}
	}
	return ports
}

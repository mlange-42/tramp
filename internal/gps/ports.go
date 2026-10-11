package gps

import (
	"cmp"
	"fmt"
	"runtime"
	"slices"
	"strings"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

// portKind classifies serial ports for the order of probing them.
type portKind int

const (
	// kindGPS is a USB port of a vendor known for GPS receivers, like Prolific or u-blox.
	kindGPS portKind = iota
	// kindUSB is another USB serial port.
	kindUSB
	// kindBluetooth is a Bluetooth serial port. The baud rate doesn't matter,
	// but opening it connects to the device, which takes a few seconds.
	kindBluetooth
	// kindOther is any other port, like a built-in serial port or a virtual port of a driver,
	// like Garmin Spanner.
	kindOther
	// kindNetwork is a network source, like an app on a phone or gpsd. It is never searched.
	kindNetwork
)

// gpsVendors are the USB vendor IDs of GPS receivers and the USB-serial chips they use.
var gpsVendors = map[string]string{
	// Prolific PL2303, in the Locosys GT-31, GlobalSat BU-353 and many GPS cables.
	"067B": "Prolific",
	// u-blox receivers and GPS mice, with USB CDC.
	"1546": "u-blox",
	// Garmin devices that show up as serial port.
	"091E": "Garmin",
}

// port is a serial port to probe for a GPS receiver.
type port struct {
	Name string
	Kind portKind
	// Desc describes the port for the status, like "USB 067B:2303".
	Desc string
	// network and addr are the network ("tcp", "udp" or "gpsd") and address of a network source.
	network, addr string
}

func (p port) String() string {
	if p.Desc == "" {
		return p.Name
	}
	return fmt.Sprintf("%s (%s)", p.Name, p.Desc)
}

// listPorts returns the serial ports to probe, most promising first.
func listPorts() ([]port, error) {
	bt := bluetoothPorts()
	var ports []port
	details, err := enumerator.GetDetailedPortsList()
	if err == nil {
		for _, d := range details {
			p := port{Name: d.Name, Kind: kindOther}
			switch {
			case d.IsUSB:
				p.Kind, p.Desc = kindUSB, "USB "+d.VID+":"+d.PID
				if v, ok := gpsVendors[strings.ToUpper(d.VID)]; ok {
					p.Kind, p.Desc = kindGPS, v
				}
			case bt[d.Name] || isBluetoothName(d.Name):
				p.Kind, p.Desc = kindBluetooth, "Bluetooth"
			}
			ports = append(ports, p)
		}
	} else {
		// Enumerating with details is not supported everywhere.
		names, err := serial.GetPortsList()
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			p := port{Name: n, Kind: kindOther}
			if bt[n] || isBluetoothName(n) {
				p.Kind, p.Desc = kindBluetooth, "Bluetooth"
			}
			ports = append(ports, p)
		}
	}
	ports = slices.DeleteFunc(ports, func(p port) bool { return skipPort(p.Name, ports) })
	slices.SortStableFunc(ports, func(a, b port) int { return cmp.Compare(a.Kind, b.Kind) })
	return ports, nil
}

// findPort returns the port with the given name, classified if it is listed.
func findPort(name string) port {
	ports, _ := listPorts()
	if i := slices.IndexFunc(ports, func(p port) bool { return strings.EqualFold(p.Name, name) }); i >= 0 {
		return ports[i]
	}
	p := port{Name: name, Kind: kindOther}
	if bluetoothPorts()[name] || isBluetoothName(name) {
		p.Kind, p.Desc = kindBluetooth, "Bluetooth"
	}
	return p
}

// isBluetoothName reports whether a port name is that of a Bluetooth serial port on Linux or macOS.
func isBluetoothName(name string) bool {
	return strings.HasPrefix(name, "/dev/rfcomm") ||
		runtime.GOOS == "darwin" && !strings.Contains(name, "usb")
}

// skipPort reports whether a port is not probed automatically:
// legacy serial ports on Linux, which are many and mostly without hardware,
// the incoming Bluetooth port on macOS, and macOS tty ports that have a cu counterpart,
// as opening those waits for a carrier signal.
func skipPort(name string, all []port) bool {
	switch {
	case strings.HasPrefix(name, "/dev/ttyS"):
		return true
	case strings.Contains(name, "Bluetooth-Incoming-Port"):
		return true
	case strings.HasPrefix(name, "/dev/tty."):
		cu := "/dev/cu." + strings.TrimPrefix(name, "/dev/tty.")
		return slices.ContainsFunc(all, func(p port) bool { return p.Name == cu })
	}
	return false
}

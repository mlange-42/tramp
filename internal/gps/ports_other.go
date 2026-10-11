//go:build !windows

package gps

// bluetoothPorts returns the names of the Bluetooth serial ports.
// Outside Windows, they are recognized by name, see [isBluetoothName].
func bluetoothPorts() map[string]bool {
	return nil
}

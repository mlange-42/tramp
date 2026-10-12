package ui

import (
	"log"
	"os/exec"
	"runtime"
)

// openURL opens a web address in the default browser, in the background.
// Errors are logged.
func openURL(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	go func() {
		if err := cmd.Run(); err != nil {
			log.Printf("opening %s: %v", url, err)
		}
	}()
}

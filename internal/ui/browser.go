package ui

import (
	"log"
	"os"
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

// openFolder opens a directory in the file manager, in the background, creating it if necessary.
// Errors are logged.
func openFolder(dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("opening %s: %v", dir, err)
		return
	}
	if runtime.GOOS != "windows" {
		openURL(dir)
		return
	}
	// Explorer exits with status 1 even when it succeeds, so its error is not logged.
	cmd := exec.Command("explorer", dir)
	go func() { _ = cmd.Run() }()
}

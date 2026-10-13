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
	startCommand(urlCommand(runtime.GOOS, url), true)
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
	startCommand(exec.Command("explorer", dir), false)
}

// urlCommand returns the command that opens a web address or a path with the default program, on the system goos.
func urlCommand(goos, url string) *exec.Cmd {
	switch goos {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		return exec.Command("open", url)
	default:
		return exec.Command("xdg-open", url)
	}
}

// startCommand runs a command in the background, and logs its error if logErr is set.
// It is a variable, so that tests can check what would be opened without opening it.
var startCommand = func(cmd *exec.Cmd, logErr bool) {
	go func() {
		if err := cmd.Run(); err != nil && logErr {
			log.Printf("running %v: %v", cmd.Args, err)
		}
	}()
}

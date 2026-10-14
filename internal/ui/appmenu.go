package ui

import (
	"fmt"
	"log"

	"github.com/mlange-42/tramp/internal/settings"
	"github.com/ncruces/zenity"
)

// Pages linked in the Help menu.
const (
	docsURL   = repoURL + "#documentation"
	issuesURL = repoURL + "/issues"
)

// menuTitles are the menus of the menu bar, in the order of [App.menuItems].
var menuTitles = []string{"File", "Map", "Help"}

// menuItems returns the items of the menus. They are built anew for each frame,
// as labels and enabled states change.
func (a *App) menuItems() [][]menuItem {
	cacheOff := a.diskCache.tiles == nil && a.diskCache.elevation == nil
	r := a.selectedFileRoute()
	routeTrackable := r != nil && len(r.Points) >= 2
	return [][]menuItem{
		{
			{label: "New file", shortcut: "Ctrl+N", icon: iconNew, action: a.newFile},
			{label: "Open files…", shortcut: "Ctrl+O", icon: iconOpen, action: a.showOpenDialog},
			menuSeparator,
			{label: "Save route as track…", icon: iconSaveTrack, disabled: !routeTrackable, action: a.saveRouteTrack},
			menuSeparator,
			{label: "Open settings folder", icon: iconSettings, action: openSettingsFolder},
			menuSeparator,
			{label: "Quit", icon: iconQuit, action: a.closeWindow},
		},
		{
			{label: fmt.Sprintf("Clear tile cache (%s)…", formatBytes(a.diskCache.size())), icon: iconClearCache, disabled: cacheOff, action: a.clearTileCache},
			{label: "Open cache folder", icon: iconFolder, disabled: cacheOff, action: func() { openFolder(a.diskCache.dir) }},
		},
		{
			{label: "Documentation", icon: iconHelp, action: func() { openURL(docsURL) }},
			{label: "Report an issue", icon: iconBug, action: func() { openURL(issuesURL) }},
			menuSeparator,
			{label: "Data sources", icon: iconInfo, action: func() { a.sources.open = true }},
			menuSeparator,
			{label: "About TRAMP", logo: true, action: func() { a.about.open = true }},
		},
	}
}

// question and showError show dialogs. They are variables, so that tests can answer them.
var (
	question  = zenity.Question
	showError = zenity.Error
)

// clearTileCache asks whether to delete the stored map and elevation tiles, and deletes them.
// The tiles in memory are kept, so that the map doesn't go blank.
func (a *App) clearTileCache() {
	size := a.diskCache.size()
	opts := a.plat.dialogOptions()
	a.dialog(func() func() {
		err := question(fmt.Sprintf("Delete %s of stored map and elevation tiles?\n\n"+
			"They are downloaded again when needed, and are not available without an internet connection until then.",
			formatBytes(size)), append([]zenity.Option{
			zenity.Title("Clear tile cache"),
			zenity.OKLabel("Delete"),
			zenity.CancelLabel("Cancel"),
			zenity.WarningIcon,
		}, opts...)...)
		if err != nil {
			return nil
		}
		if err := a.diskCache.clear(); err != nil {
			_ = showError(err.Error(), append([]zenity.Option{zenity.Title("Clearing the tile cache failed")}, opts...)...)
		}
		return nil
	})
}

// openSettingsFolder opens the directory of the settings files in the file manager.
func openSettingsFolder() {
	dir, err := settings.Dir()
	if err != nil {
		log.Printf("settings folder: %v", err)
		return
	}
	openFolder(dir)
}

// formatBytes formats a size like "812 MB", "4.2 MB" or "350 KB", in MB like the size limits in the settings.
func formatBytes(n int64) string {
	mb := float64(n) / (1 << 20)
	if mb < 1 {
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	if mb < 10 {
		return fmt.Sprintf("%.1f MB", mb)
	}
	return fmt.Sprintf("%.0f MB", mb)
}

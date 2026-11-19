package app

import (
	"github.com/egoist/mygo"

	"waygu-music/internal/pluginapi"
)

// openSettings shows the settings window, opening it the first time.
func (a *app) openSettings() {
	if a.platformWindows[a.platformID][pluginapi.SettingsWindow].Root == nil {
		return
	}
	if a.settingsWin != nil {
		a.settingsWin.Show()
		a.settingsWin.Focus()
		a.measureCache()
		return
	}
	a.measureCache()
	options := a.windowOptions(pluginapi.SettingsWindow)
	a.settingsWin = mygo.NewWindow(options)
	a.settingsWin.OnClosed(func() {
		a.settingsWin = nil
		a.syncDock()
	})
	a.syncDock()
}

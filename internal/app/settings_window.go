package app

import (
	"github.com/egoist/mygo"
	"github.com/waygu-music-plugin/sdk/pluginapi"
)

// openSettings shows the settings window, opening it the first time.
func (a *app) openSettings() {
	for _, local := range a.localPlugins {
		if local.pending != nil {
			return
		}
	}
	if remote := a.remoteActive(); remote != nil {
		go func() { _, _ = remote.action("settings") }()
		return
	}
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

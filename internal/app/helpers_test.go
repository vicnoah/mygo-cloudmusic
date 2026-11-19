package app

import (
	"github.com/egoist/mygo/ui"

	"waygu-music/internal/player"
	"waygu-music/internal/pluginapi"
)

func newTestApp() *app {
	a := &app{}
	a.art = newArtCache(func(func()) {})
	a.pl = player.New(func(func()) {})
	a.pl.SetVolume(0.5)
	return a
}

func (a *app) view(c *ui.Context)         { a.windowRoot(pluginapi.MainWindow)(c) }
func (a *app) settingsView(c *ui.Context) { a.windowRoot(pluginapi.SettingsWindow)(c) }

func testRegistration(id pluginapi.ID, configure func(pluginapi.Startup) pluginapi.Adapter) pluginapi.Registration {
	return pluginapi.Registration{
		Info: pluginapi.Info{ID: id, Name: string(id)},
		Start: func(s pluginapi.Startup) pluginapi.Adapter {
			s.RegisterWindow(pluginapi.MainWindow, pluginapi.Window{Root: func(*ui.Context) {}})
			if configure != nil {
				return configure(s)
			}
			return testPluginAdapter{}
		},
	}
}

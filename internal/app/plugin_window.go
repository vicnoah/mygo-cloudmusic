package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/waygu-music-plugin/sdk/pluginapi"
)

type mountedWindow struct {
	native  *mygo.Window
	unmount func()
	updated func()
}

// syncPlatformWindows maintains the native settings and desktop lyrics windows
// when their roles are available in the active plugin.
func (a *app) syncPlatformWindows() {
	windows := a.platformWindows[a.platformID]
	if settings := a.settingsWin; settings != nil && windows[pluginapi.SettingsWindow].Root == nil {
		settings.Destroy()
	}
	if windows[pluginapi.DesktopLyricsWindow].Root == nil {
		a.desktopLyrics.Destroy()
	} else {
		a.newDesktopLyrics()
	}
}

// mountPlatformWindows installs the auxiliary windows of the active plugin.
func (a *app) mountPlatformWindows() {
	for id, window := range a.platformWindows[a.platformID] {
		switch id {
		case pluginapi.MainWindow, pluginapi.SettingsWindow, pluginapi.DesktopLyricsWindow:
			continue
		}
		if window.Root == nil {
			continue
		}
		options := window.Options
		options.Content = ui.View(window.Root)
		native := mygo.NewWindow(options)
		if a.mountedWindows == nil {
			a.mountedWindows = make(map[pluginapi.WindowID]mountedWindow)
		}
		var unmount func()
		if window.Mount != nil {
			unmount = window.Mount(native)
		}
		a.mountedWindows[id] = mountedWindow{native, unmount, window.Updated}
	}
}

func (a *app) unmountPlatformWindows() {
	for id, window := range a.mountedWindows {
		delete(a.mountedWindows, id)
		if window.unmount != nil {
			window.unmount()
		}
		window.native.Destroy()
	}
}

func (a *app) quickPlayerWindow() *mygo.Window {
	return a.mountedWindows[pluginapi.QuickPlayerWindow].native
}

func (a *app) windowOptions(id pluginapi.WindowID) mygo.WindowOptions {
	options := a.platformWindows[a.platformID][id].Options
	options.Content = ui.View(a.windowRoot(id))
	return options
}

func (a *app) windowRoot(id pluginapi.WindowID) func(*ui.Context) {
	return func(c *ui.Context) {
		if id == pluginapi.MainWindow && a.hasPluginManager() {
			c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyP, a.openPluginManager)
		}
		if root := a.platformWindows[a.platformID][id].Root; root != nil {
			// A reused window must not carry widget input into a different
			// activation just because both roots use the same widget keys.
			c.Root().Key(a.platformGeneration)
			root(c)
		}
	}
}

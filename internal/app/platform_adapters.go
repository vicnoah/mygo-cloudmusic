package app

import (
	"runtime"

	"github.com/egoist/mygo"

	appplatform "waygu-music/internal/pluginapi"
)

func newPlatformAdapters(a *app) {
	if a == nil {
		return
	}
	if a.platforms == nil {
		a.platforms = make(map[appplatform.ID]appplatform.Adapter, len(a.platformRegistrations))
	}
	a.platformWindows = make(map[appplatform.ID]map[appplatform.WindowID]appplatform.Window, len(a.platformRegistrations))
	a.platformMenus = make(map[appplatform.ID]map[appplatform.MenuID]func() *mygo.Menu, len(a.platformRegistrations))
	a.platformLifecycles = make(map[appplatform.ID]appplatform.Lifecycle, len(a.platformRegistrations))
	host := platformHost{a: a}
	var panel *appplatform.MenuBarPanel
	if runtime.GOOS == "darwin" {
		panel = &appplatform.MenuBarPanel{
			CameraHousing: notchSize, MenuBarHeight: menuBarHeight,
			Raise: raiseAboveMenuBar, Pin: pinToTop, WatchPointer: watchPointer,
			Preferences:     a.quickPlayerPreferences,
			SavePreferences: a.saveQuickPlayerPreferences,
		}
	}
	for _, entry := range a.platformRegistrations {
		if entry.Start == nil {
			continue
		}
		windows := make(map[appplatform.WindowID]appplatform.Window)
		menus := make(map[appplatform.MenuID]func() *mygo.Menu)

		a.platforms[entry.ID] = entry.Start(appplatform.Startup{
			Host:         host,
			Settings:     pluginSettingsStore{a: a, entry: entry},
			MenuBarPanel: panel,
			RegisterWindow: func(id appplatform.WindowID, window appplatform.Window) {
				windows[id] = window
			},
			RegisterMenu: func(id appplatform.MenuID, menu func() *mygo.Menu) {
				menus[id] = menu
			},
			RegisterLifecycle: func(lifecycle appplatform.Lifecycle) {
				a.platformLifecycles[entry.ID] = lifecycle
			},
		})
		a.platformWindows[entry.ID] = windows
		a.platformMenus[entry.ID] = menus
	}
}

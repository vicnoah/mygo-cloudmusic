package app

import (
	_ "embed"
	"log"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/protocol"
)

// The app's icon in the menu bar on macOS, as the settings choose
// (appIcons), and always in the notification area on Windows and Linux.
// While it shows, closing the window leaves the music playing there, until
// the listener quits; the icon opens the window again.

// trayTemplate is the record and tonearm of the app's icon, black on
// transparent for macOS to tint: 15.6 points tall in the menu bar's 18, at
// 2×, as icon/tray draws it.
//
//go:embed tray/tray-template.png
var trayTemplate []byte

// trayColor is the app's icon, for the notification area.
//
//go:embed tray/tray.png
var trayColor []byte

func showsTray(s settings) bool {
	return runtime.GOOS != "darwin" || s.AppIcon != "dock" || s.StatusLyrics
}

// activationPolicy is macOS's for the settings: no Dock icon for the menu
// bar alone, unless a window is open, so the listener can switch to it
// with Command-Tab and the app's menu shows.
func activationPolicy(s settings, windowOpen bool) mygo.ActivationPolicy {
	if s.AppIcon == "menubar" && !windowOpen {
		return mygo.ActivationPolicyAccessory
	}
	return mygo.ActivationPolicyRegular
}

// windowOpen tells whether the main window or the settings window is
// open: shown or minimized, as the notch player doesn't count.
func (a *app) windowOpen() bool {
	if a.settingsWin != nil || a.pluginsWin != nil || a.pluginStartup.notice != nil {
		return true
	}
	if remote := a.remoteActive(); remote != nil && remote.visible {
		return true
	}
	return a.win != nil && (a.win.IsVisible() || a.win.IsMinimized())
}

// syncDock shows or hides the Dock icon, as the settings and the open
// windows say.
func (a *app) syncDock() {
	if runtime.GOOS == "darwin" {
		mygo.App.SetActivationPolicy(activationPolicy(a.settings, a.windowOpen()))
	}
}

// changeAppIcon shows the app's icon where the settings now say, as the
// listener chose another place for it.
func (a *app) changeAppIcon() {
	a.syncDock()
	a.applyTray()
}

// applyTray adds or removes the icon in the menu bar or notification area,
// as the settings say.
func (a *app) applyTray() {
	switch show := showsTray(a.settings); {
	case show && a.tray == nil:
		icon, template := trayArtwork()
		t, err := mygo.NewTray(mygo.TrayOptions{ID: "cloudmusic", Icon: icon, IconIsTemplate: template, ToolTip: appName, Title: a.statusLyrics.title, Menu: a.trayMenu()})
		if err != nil {
			log.Println("tray:", err)
			return
		}
		// Windows' left click, where the right one opens the menu.
		t.OnClick(a.showMain)
		a.tray = t
	case !show && a.tray != nil:
		a.tray.Destroy()
		a.tray = nil
		a.statusLyrics.trayCover = nil
	}
	a.syncStatusLyrics()
}

func trayArtwork() ([]byte, bool) {
	if runtime.GOOS == "darwin" {
		return trayTemplate, true
	}
	return trayColor, false
}

// trayMenu is the icon's menu: the window, the player's controls,
// settings, updates, and quitting.
func (a *app) trayMenu() *mygo.Menu {
	if a.managedPlugins != nil {
		click := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
			return func(*mygo.MenuItem, *mygo.Window) { fn() }
		}
		items := []*mygo.MenuItem{
			{Label: "显示主窗口", Click: click(a.showMain)},
			{Label: "插件管理…", Click: click(a.openPluginManager)},
		}
		lockLabel := "锁定桌面歌词"
		if a.desktopLyricsPrefs().Locked {
			lockLabel = "解锁桌面歌词"
		}
		if remote := a.remoteActive(); remote != nil {
			invoke := func(action string) func() {
				return func() {
					if a.remoteActive() == remote {
						go func() { _, _ = remote.action(action) }()
					}
				}
			}
			if remote.info.SupportsWindowAction("settings") {
				items = append(items, &mygo.MenuItem{Label: "设置…", Click: click(invoke("settings"))})
			}
			if remote.supportsDesktopLyrics() {
				items = append(items,
					&mygo.MenuItem{Label: "桌面歌词", Click: click(invoke(protocol.WindowToggleDesktopLyrics))},
					&mygo.MenuItem{Label: lockLabel, Click: click(invoke(protocol.WindowToggleDesktopLyricsLock))},
				)
			}
		} else {
			h := platformHost{a: a}
			var local []*mygo.MenuItem
			if a.platformWindows[a.platformID][pluginapi.SettingsWindow].Root != nil {
				local = append(local, &mygo.MenuItem{Label: "设置…", Click: click(a.openSettings)})
			}
			if a.desktopLyrics.IsOpen() {
				local = append(local,
					&mygo.MenuItem{Label: "桌面歌词", Click: click(h.ToggleDesktopLyrics)},
					&mygo.MenuItem{Label: lockLabel, Click: click(h.ToggleDesktopLyricsLock)},
				)
			}
			bindMenuActions(local, a.platformContext)
			items = append(items, local...)
		}
		if statusLyricsAvailable() {
			items = append(items, &mygo.MenuItem{
				Label: "任务栏歌词", Type: mygo.MenuItemCheckbox, Checked: a.settings.StatusLyrics,
				Click: click(func() { (platformHost{a: a}).SetStatusLyrics(!a.settings.StatusLyrics) }),
			})
		}
		items = append(items,
			mygo.Separator(),
			&mygo.MenuItem{Label: "播放/暂停", Click: click(a.pl.TogglePlay)},
			&mygo.MenuItem{Label: "上一首", Click: click(a.pl.Prev)},
			&mygo.MenuItem{Label: "下一首", Click: click(a.pl.Next)},
			mygo.Separator(),
			&mygo.MenuItem{Label: "退出肥牛音乐", Click: click(mygo.App.Quit)},
		)
		return mygo.NewMenu(items)
	}
	menu := a.platformMenu(pluginapi.TrayMenu)
	if !a.hasPluginManager() {
		return menu
	}
	var items []*mygo.MenuItem
	if menu != nil {
		items = menu.Items()
	}
	return mygo.NewMenu(append([]*mygo.MenuItem{{
		Label: "插件管理…", Click: func(*mygo.MenuItem, *mygo.Window) { a.openPluginManager() },
	}}, items...))
}

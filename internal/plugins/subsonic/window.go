package subsonic

import (
	"context"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	backend "waygu-music/internal/plugins/subsonic/internal/client"
)

const appName = "肥牛音乐"

// startProfile registers the native windows for one Subsonic profile.
func startProfile(start pluginapi.Startup, profile backend.Profile) pluginapi.Adapter {
	preferences, save := pluginapi.BindSettings(start.Settings, platformPreferences{})
	adapter := NewAdapter(start.Host, profile, preferences, save)
	adapter.home.settings = start.Host.Presentation()
	adapter.home.npPane = adapter.home.lyricsPaneChosen()
	start.RegisterLifecycle(pluginapi.Lifecycle{
		Activate: func(ctx context.Context, _ pluginapi.PlaybackContinuity) {
			adapter.home.scope = ctx
			adapter.state.Catalog().activate(ctx)
			adapter.home.ResetNavigation(Route{Page: PageLibrary})
		},
		Ready: adapter.connectOnActivation,
		Deactivate: func(switching bool) {
			adapter.home.CloseTransient()
			if switching {
				_ = adapter.runtime.Cancel(context.Background())
			}
		},
	})
	start.RegisterWindow(pluginapi.MainWindow, pluginapi.Window{
		Options: mygo.WindowOptions{
			Title:                appName,
			Width:                1180,
			Height:               780,
			MinWidth:             880,
			MinHeight:            580,
			StateKey:             "main",
			TitleBarStyle:        mygo.TitleBarHidden,
			TitleBarHeight:       buttonsHeight,
			TrafficLightPosition: &mygo.Point{X: 20, Y: 20},
			Vibrancy:             mygo.VibrancySidebar,
			BackgroundColor:      "light-dark(#ffffff, #141416)",
		},
		Root: adapter.home.mainWindow,
	})
	start.RegisterWindow(pluginapi.SettingsWindow, pluginapi.Window{
		Options: mygo.WindowOptions{
			Title:           "设置",
			Width:           560,
			Height:          720,
			MinWidth:        480,
			MinHeight:       420,
			StateKey:        "settings",
			DisableMaximize: true,
			DisableMinimize: true,
			AutoHideMenuBar: true,
			BackgroundColor: "light-dark(#f4f4f6, #1c1c1f)",
		},
		Root: adapter.home.settingsWindow,
	})
	start.RegisterMenu(pluginapi.ApplicationMenu, adapter.home.applicationMenu)
	start.RegisterMenu(pluginapi.TrayMenu, adapter.home.trayMenu)
	start.RegisterWindow(pluginapi.DesktopLyricsWindow, pluginapi.Window{
		Options: mygo.WindowOptions{
			Title: "桌面歌词", Width: desktopLyricsWidth, Height: desktopLyricsHeight,
			Hidden: true, Frameless: true, Transparent: true, BackgroundColor: "#00000000",
			Vibrancy: mygo.VibrancyGlassClear, AlwaysOnTop: true, SkipTaskbar: true,
			DisableShadow: true, DisableResize: true, DisableMinimize: true, DisableMaximize: true,
		},
		Root: adapter.home.desktopLyricsView,
	})
	if start.MenuBarPanel != nil {
		start.RegisterWindow(pluginapi.QuickPlayerWindow, adapter.home.quickPlayer(start.MenuBarPanel))
	}
	return adapter
}

func (h *Home) settingsWindow(c *ui.Context) {
	if message := h.host.TakeToast(); message != "" {
		c.Toast(message)
	}
	c.OnShortcut(ui.Cmd, ui.KeyW, h.host.CloseSettings)
	h.Settings(c)
}

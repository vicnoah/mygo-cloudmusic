package netease

import (
	"context"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/plugins/netease/internal/client"
)

const appName = "肥牛音乐"

const ID = netease.PlatformID

func Registration() pluginapi.Registration {
	return pluginapi.Registration{
		Info:  pluginapi.Info{ID: ID, Name: "网易云音乐", Icon: iconNote},
		Start: Start, DeveloperOnly: true,
		LegacySettingsFields:    []string{"quality", "normalize", "captions", "hotComments"},
		FallbackOnInvalidSwitch: true,
	}
}

// Start owns the NetEase playback policy, window roots, and menus.
func Start(start pluginapi.Startup) pluginapi.Adapter {
	return startApp(start, newApp(start))
}

func startApp(start pluginapi.Startup, a *app) pluginapi.Adapter {
	start.RegisterLifecycle(pluginapi.Lifecycle{
		Activate: a.activate,
		Ready: func(event pluginapi.Activation) {
			a.boot()
			a.debugShots(event.Main)
		},
		Deactivate:  func(bool) { a.deactivate() },
		MainFocused: a.freshenHome,
		PlaybackChanged: func() {
			if a.scope != nil && a.scope.Err() == nil {
				a.syncCaptions()
			}
		},
	})
	start.RegisterWindow(pluginapi.MainWindow, pluginapi.Window{
		Options: mygo.WindowOptions{
			Title: appName, Width: 1180, Height: 780, MinWidth: 880, MinHeight: 580,
			StateKey: "main", TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: buttonsHeight,
			TrafficLightPosition: &mygo.Point{X: 20, Y: 20},
			Vibrancy:             mygo.VibrancySidebar, BackgroundColor: "light-dark(#ffffff, #141416)",
		},
		Root: a.mainWindow,
	})
	start.RegisterWindow(pluginapi.SettingsWindow, pluginapi.Window{
		Options: mygo.WindowOptions{
			Title: "设置", Width: 560, Height: 720, MinWidth: 480, MinHeight: 420,
			StateKey: "settings", DisableMaximize: true, DisableMinimize: true, AutoHideMenuBar: true,
			BackgroundColor: "light-dark(#f4f4f6, #1c1c1f)",
		},
		Root: a.settingsWindow,
	})
	start.RegisterWindow(pluginapi.DesktopLyricsWindow, pluginapi.Window{
		Options: mygo.WindowOptions{
			Title: "桌面歌词", Width: desktopLyricsWidth, Height: desktopLyricsHeight,
			Hidden: true, Frameless: true, Transparent: true, BackgroundColor: "#00000000",
			Vibrancy: mygo.VibrancyGlassClear, AlwaysOnTop: true, SkipTaskbar: true,
			DisableShadow: true, DisableResize: true, DisableMinimize: true, DisableMaximize: true,
		},
		Root: a.desktopLyricsView,
	})
	if start.MenuBarPanel != nil {
		start.RegisterWindow(pluginapi.QuickPlayerWindow, a.quickPlayer(start.MenuBarPanel))
	}
	start.RegisterMenu(pluginapi.ApplicationMenu, a.applicationMenu)
	start.RegisterMenu(pluginapi.TrayMenu, a.trayMenu)
	return &adapter{ui: a}
}

func newApp(start pluginapi.Startup) *app {
	preferences, save := pluginapi.BindSettings(start.Settings, platformPreferences{Quality: "exhigh", Normalize: true, Captions: true})
	return &app{
		host: start.Host, preferences: preferences, savePreferences: save,
		playlists: map[int64]*netease.Playlist{}, loadingPl: map[int64]bool{},
		plErr: map[int64]string{}, artists: map[int64]*artistData{},
		albums: map[int64]*albumData{}, users: map[int64]*userData{},
		podcasts: map[int64]*podcastData{}, podcastTab: &podcastTab{},
		searches: map[route]*searchData{}, lyricAt: -1, queueAt: -1,
	}
}

func (a *app) activate(ctx context.Context, continuity pluginapi.PlaybackContinuity) {
	a.scope, a.cancelScope = context.WithCancel(ctx)
	a.pl = &playback{
		PlaybackControls: a.host.Playback(), PlaybackContinuity: continuity,
		port: a.host.Player(), volume: a.host.SetVolume,
	}
	a.refreshSharedSettings()
	a.settings.Quality = a.preferences.Quality
	a.settings.HotComments = a.preferences.HotComments
	a.settings.Normalize = a.preferences.Normalize
	a.settings.Captions = a.preferences.Captions
	a.route, a.scrolled = route{page: pageHome}, route{}
	a.history, a.forward = nil, nil
	a.nowPlaying, a.npOpen = false, false
	a.npPane = a.lyricsPaneChosen()
	a.volumeOpen = false
	a.platformMenuOpen = false
	a.trackList = ui.ListState{}
	a.pageScroll, a.navScroll = ui.ScrollState{}, ui.ScrollState{}
	a.comments = commentState{}
	a.homeLoading = true
}

func (a *app) deactivate() {
	if a.npOpen {
		a.host.SetMainWindowButtonsTheme(mygo.ThemeSystem)
	}
	a.nowPlaying, a.npOpen = false, false
	if a.cancelScope != nil {
		a.cancelScope()
	}
	a.loadingPl = make(map[int64]bool)
	a.radioStarting = ""
	if a.captions.model.cancel != nil {
		a.captions.model.cancel()
		a.captions.model = modelState{}
	}
	a.login = loginState{gen: a.login.gen + 1}
	a.accountOpen, a.castOpen, a.eqOpen = false, false, false
	a.eqUI.saveOpen, a.eqUI.deleteOpen, a.eqUI.autoeqOpen = false, false, false
	a.eqUI.autoeqLoading, a.eqUI.autoeqBusy = false, ""
	a.dismissSuggestions()
	a.flushTranscript()
	a.stopCaptions()
	a.toast = ""
}

func (a *app) flushTranscript() {
	if write := a.transcriptWrite(); write != nil {
		write()
	}
}

type adapter struct {
	ui *app
}

func (*adapter) Configured() bool { return true }

func (*adapter) FetchArtwork(context.Context, string, int) ([]byte, bool, error) {
	return nil, false, nil
}

package netease

import (
	"context"
	"image"
	"slices"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"waygu-music/internal/player"
	"waygu-music/internal/player/mpv"
	"waygu-music/internal/pluginapi"
	cover "waygu-music/internal/pluginapi/artwork"
	"waygu-music/internal/pluginapi/eq"
	"waygu-music/internal/pluginapi/network"
	"waygu-music/internal/plugins/netease/internal/client"
)

type testApp struct {
	*app
	pl       *player.Player
	hostTest *testHost
}

func newTestApp() *testApp {
	p := player.New(func(func()) {})
	h := &testHost{player: p, artwork: map[string]*fixtureArtwork{}}
	var rebind func()
	var main, settingsView func(*ui.Context)
	start := pluginapi.Startup{
		Host: h,
		RegisterWindow: func(id pluginapi.WindowID, window pluginapi.Window) {
			switch id {
			case pluginapi.MainWindow:
				main = window.Root
			case pluginapi.SettingsWindow:
				settingsView = window.Root
			}
		},
		RegisterMenu: func(pluginapi.MenuID, func() *mygo.Menu) {},
		RegisterLifecycle: func(lifecycle pluginapi.Lifecycle) {
			rebind = func() { lifecycle.Activate(context.Background(), h) }
		},
	}
	a := newApp(start)
	h.controller = a
	startApp(start, a)
	h.main, h.settingsView = main, settingsView
	a.settings = defaultSettings()
	rebind()
	a.settings = defaultSettings()
	a.homeLoading = false
	p.SetVolume(0.5)
	return &testApp{app: a, pl: p, hostTest: h}
}

func (a *testApp) view(c *ui.Context)         { a.hostTest.main(c) }
func (a *testApp) settingsView(c *ui.Context) { a.hostTest.settingsView(c) }
func (a *testApp) mpvReady() bool             { return mpv.Ready() }

func defaultSettings() settings {
	return settings{
		DesktopPreferences: pluginapi.DesktopPreferences{
			Appearance: "system", AppIcon: "dock-and-menubar", CacheMB: 2048, NotchActivity: true,
		},
		Quality: "exhigh", Normalize: true, Captions: true, EQ: eq.Default(),
	}
}

type testHost struct {
	pluginapi.Services
	controller         *app
	player             *player.Player
	main, settingsView func(*ui.Context)
	artwork            map[string]*fixtureArtwork
	searchHistory      []string
	savedSession       *pluginapi.PlaybackSession
	posted             []func()
	collectPosts       bool
	castingScan        bool
	desktopLyrics      pluginapi.DesktopLyricsState
	windowButtonsTheme mygo.ThemeSource
	updates            bool
}

func (h *testHost) Player() pluginapi.PlayerPort         { return testPlayer{h.player} }
func (h *testHost) Playback() pluginapi.PlaybackControls { return h.player }
func (h *testHost) LoadSession() (pluginapi.PlaybackSession, bool) {
	if h.savedSession == nil {
		return pluginapi.PlaybackSession{}, false
	}
	saved := *h.savedSession
	h.savedSession = nil
	return saved, true
}
func (h *testHost) Restore(s pluginapi.PlaybackSession) { h.player.Restore(s) }
func (h *testHost) RadioFrom(n string, t []pluginapi.Track, f func(func([]pluginapi.Track))) {
	h.player.RadioFrom(n, t, f)
}
func (h *testHost) AttachRadio(n string, f func(func([]pluginapi.Track))) { h.player.AttachRadio(n, f) }
func (h *testHost) OpenAudio() (pluginapi.Audio, error)                   { return h.player.OpenAudio() }
func (*testHost) TakeToast() string                                       { return "" }
func (h *testHost) Post(fn func()) {
	if h.collectPosts {
		h.posted = append(h.posted, fn)
	}
}
func (*testHost) Invalidate()                                        {}
func (*testHost) ShowMain()                                          {}
func (*testHost) OpenSettings()                                      {}
func (*testHost) CloseSettings()                                     {}
func (h *testHost) SetMainWindowButtonsTheme(theme mygo.ThemeSource) { h.windowButtonsTheme = theme }
func (*testHost) CurrentPlatform() pluginapi.ID                      { return ID }
func (*testHost) PlatformOptions() []pluginapi.Info                  { return nil }
func (h *testHost) DesktopLyricsVisible() bool                       { return false }
func (h *testHost) ToggleDesktopLyrics()                             {}
func (h *testHost) DesktopLyricsState() pluginapi.DesktopLyricsState { return h.desktopLyrics }
func (h *testHost) SetDesktopLyricsStyle(translation bool, size float32, color int) {
	h.desktopLyrics.ShowTranslation, h.desktopLyrics.FontSize, h.desktopLyrics.Color = translation, size, color
}
func (h *testHost) EqualizerAvailable() bool {
	return h.player.UsingMPV()
}
func (h *testHost) EqualizerSettings() eq.Settings { return h.controller.settings.EQ }
func (h *testHost) ApplyEqualizer(s eq.Settings)   { h.controller.settings.EQ = s }
func (h *testHost) Casting() pluginapi.CastingState {
	state := pluginapi.CastingState{Scanning: h.castingScan}
	ctrl := h.player.DLNA()
	if ctrl == nil {
		return state
	}
	state.Available, state.Sending = true, h.player.IsCasting()
	for _, d := range ctrl.Devices() {
		state.Devices = append(state.Devices, pluginapi.CastingDevice{ID: d.ID, Name: d.FriendlyName})
	}
	if d := ctrl.ActiveDevice(); d != nil {
		state.Active = &pluginapi.CastingDevice{ID: d.ID, Name: d.FriendlyName}
	}
	return state
}
func (h *testHost) ScanCastingDevices()            {}
func (h *testHost) SwitchPlaybackOutput(id string) { h.player.SwitchOutput(id) }
func (h *testHost) DecodedArtwork(url string, size int, blur bool) pluginapi.DecodedArtwork {
	key := netease.Sized(url, size)
	if blur {
		key += "#backdrop"
	}
	if art := h.artwork[key]; art != nil {
		return art
	}
	return nil
}
func (*testHost) Volume() float64       { return 0 }
func (h *testHost) SetVolume(v float64) { h.player.SetVolume(v) }
func (h *testHost) DesktopSettings() pluginapi.DesktopSettings {
	return pluginapi.DesktopSettings{Preferences: h.controller.settings.DesktopPreferences, CacheUsed: -1, Updates: h.updates}
}
func (h *testHost) Presentation() pluginapi.Presentation {
	s := h.controller.settings
	return pluginapi.Presentation{Smoke: s.Smoke, Fog: s.Fog, LyricsStage: s.LyricsStage, StageStyle: s.StageStyle, StageSparks: s.StageSparks}
}
func (h *testHost) SetLyricsPresentation(stage bool, style string, sparks bool) {
	h.controller.settings.LyricsStage, h.controller.settings.StageStyle, h.controller.settings.StageSparks = stage, style, sparks
}
func (h *testHost) DecoderStatus() pluginapi.DecoderStatus {
	return pluginapi.DecoderStatus{Ready: mpv.Ready(), Downloaded: mpv.IsDownloaded(), SystemPath: mpv.SystemPath()}
}
func (h *testHost) GlobalSearchHistory() []string { return h.searchHistory }
func (h *testHost) RememberGlobalSearch(q string) {
	h.searchHistory = append([]string{q}, slices.DeleteFunc(h.searchHistory, func(s string) bool { return s == q })...)
	if len(h.searchHistory) > 20 {
		h.searchHistory = h.searchHistory[:20]
	}
}
func (h *testHost) ClearSearchHistory() { h.searchHistory = nil }
func (*testHost) CopyText(text string)  { _ = copyText(text) }

var copyText = func(string) error { return nil }

type testPlayer struct{ *player.Player }

func (p testPlayer) SetShuffle(on bool) {
	if p.Shuffle() != on {
		p.ToggleShuffle()
	}
}

type fixtureArtwork struct {
	bmp, backdrop *ui.Bitmap
	tint          ui.Color
}

func (a *fixtureArtwork) Bitmap() *ui.Bitmap  { return a.bmp }
func (a *fixtureArtwork) Blurred() *ui.Bitmap { return a.backdrop }
func (a *fixtureArtwork) Tint() ui.Color      { return a.tint }
func (*fixtureArtwork) Failed() bool          { return false }

func fetchImage(url string) (image.Image, error) {
	data, err := network.Fetch(url)
	if err != nil {
		return nil, err
	}
	return cover.Decode(data)
}

func (h *testHost) ApplyDesktopPreferences(p pluginapi.DesktopPreferences) {
	h.controller.settings.DesktopPreferences = p
	h.player.SetUseMPV(p.UseMPV)
}

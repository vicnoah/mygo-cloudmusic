// Package plugintest supplies a controlled implementation of the host
// services for tests of plugin-owned windows.
package plugintest

import (
	"context"
	"slices"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"waygu-music/internal/player"
	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/eq"
)

type Host struct {
	pluginapi.Services
	ID                                   pluginapi.ID
	Engine                               *player.Player
	Windows                              map[pluginapi.WindowID]pluginapi.Window
	Menus                                map[pluginapi.MenuID]func() *mygo.Menu
	Lifecycle                            pluginapi.Lifecycle
	Desktop                              pluginapi.DesktopSettings
	Lyrics                               pluginapi.DesktopLyricsState
	History                              []string
	QuickPanel                           *pluginapi.MenuBarPanel
	Save                                 func()
	Preferences                          *Settings
	VersionClicks, UpdateChecksRequested int
	message                              string
	volume                               float64
}

func NewHost(id pluginapi.ID) *Host {
	h := &Host{
		ID: id, Engine: player.New(func(func()) {}),
		Windows: make(map[pluginapi.WindowID]pluginapi.Window),
		Menus:   make(map[pluginapi.MenuID]func() *mygo.Menu),
		Desktop: pluginapi.DesktopSettings{CacheUsed: -1},
	}
	h.SetVolume(0.5)
	return h
}

func (h *Host) Start(entry pluginapi.Registration, preferences any) pluginapi.Adapter {
	h.Preferences = NewSettings(preferences, h.save)
	return entry.Start(pluginapi.Startup{
		Host: h, Settings: h.Preferences,
		MenuBarPanel:      h.QuickPanel,
		RegisterWindow:    func(id pluginapi.WindowID, window pluginapi.Window) { h.Windows[id] = window },
		RegisterMenu:      func(id pluginapi.MenuID, menu func() *mygo.Menu) { h.Menus[id] = menu },
		RegisterLifecycle: func(lifecycle pluginapi.Lifecycle) { h.Lifecycle = lifecycle },
	})
}

// Start begins an activation with offscreen roots and no native windows.
func Start(t *testing.T, entry pluginapi.Registration) *Host {
	t.Helper()
	h := NewHost(entry.ID)
	h.Start(entry, nil)
	h.Activate(t)
	return h
}

func (h *Host) Activate(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	if activate := h.Lifecycle.Activate; activate != nil {
		activate(ctx, nil)
	}
	t.Cleanup(func() {
		cancel()
		if deactivate := h.Lifecycle.Deactivate; deactivate != nil {
			deactivate(false)
		}
	})
	return ctx
}

func (h *Host) Playback() pluginapi.PlaybackControls { return h.Engine }
func (h *Host) Player() pluginapi.PlayerPort         { return playerPort{h.Engine} }
func (h *Host) Volume() float64                      { return h.volume }
func (h *Host) SetVolume(volume float64) {
	h.volume = volume
	h.Engine.SetVolume(volume)
}
func (h *Host) CurrentPlatform() pluginapi.ID   { return h.ID }
func (*Host) PlatformOptions() []pluginapi.Info { return nil }
func (*Host) PlaybackLevels() ([pluginapi.SpectrumBands]float32, bool) {
	return [pluginapi.SpectrumBands]float32{}, false
}
func (*Host) Artwork(string, int) (*ui.Bitmap, bool)                    { return nil, false }
func (*Host) DecodedArtwork(string, int, bool) pluginapi.DecodedArtwork { return nil }
func (*Host) SetMainWindowButtonsTheme(mygo.ThemeSource)                {}
func (*Host) ShowMain()                                                 {}
func (*Host) CloseSettings()                                            {}
func (*Host) Invalidate()                                               {}
func (*Host) EqualizerAvailable() bool                                  { return false }
func (*Host) EqualizerSettings() eq.Settings                            { return eq.Default() }
func (*Host) Casting() pluginapi.CastingState                           { return pluginapi.CastingState{} }
func (*Host) DecoderStatus() pluginapi.DecoderStatus                    { return pluginapi.DecoderStatus{} }
func (h *Host) DesktopSettings() pluginapi.DesktopSettings              { return h.Desktop }

func (h *Host) ApplyDesktopPreferences(preferences pluginapi.DesktopPreferences) {
	h.Desktop.Preferences = preferences
	h.save()
}

func (h *Host) Presentation() pluginapi.Presentation {
	return pluginapi.Presentation{Smoke: h.Desktop.Preferences.Smoke, Fog: h.Desktop.Preferences.Fog}
}

func (*Host) Post(func())            {}
func (h *Host) Toast(message string) { h.message = message }

func (h *Host) TakeToast() string {
	message := h.message
	h.message = ""
	return message
}

func (h *Host) GlobalSearchHistory() []string { return h.History }
func (h *Host) RememberGlobalSearch(query string) {
	h.History = slices.DeleteFunc(h.History, func(old string) bool { return old == query })
	h.History = append([]string{query}, h.History...)
}

func (h *Host) DesktopLyricsState() pluginapi.DesktopLyricsState { return h.Lyrics }
func (*Host) DesktopLyricsVisible() bool                         { return false }
func (h *Host) DesktopLyricsLocked() bool                        { return h.Lyrics.Locked }
func (h *Host) ToggleDesktopLyricsLock()                         { h.Lyrics.Locked = !h.Lyrics.Locked }
func (h *Host) SetDesktopLyricsStyle(translation bool, size float32, color int) {
	h.Lyrics.ShowTranslation, h.Lyrics.FontSize, h.Lyrics.Color = translation, size, color
}

func (h *Host) RecordVersionClick()             { h.VersionClicks++ }
func (h *Host) SetUpdateChecks(enabled bool)    { h.Desktop.UpdateChecks = enabled }
func (h *Host) SetUpdateDownloads(enabled bool) { h.Desktop.UpdateDownloads = enabled }
func (h *Host) CheckForUpdates()                { h.UpdateChecksRequested++ }

func (h *Host) save() {
	if h.Save != nil {
		h.Save()
	}
}

type playerPort struct{ *player.Player }

func (p playerPort) SetShuffle(on bool) {
	if on != p.Shuffle() {
		p.ToggleShuffle()
	}
}

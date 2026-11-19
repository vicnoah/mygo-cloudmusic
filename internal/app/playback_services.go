package app

import (
	"context"
	"slices"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/eq"
)

func (h platformHost) Playback() pluginapi.PlaybackControls {
	if h.a.pl == nil {
		return nil
	}
	return h.a.pl
}

func (h platformHost) Volume() float64 { return h.a.volume }

func (h platformHost) SetVolume(volume float64) {
	h.a.volume = volume
	h.a.pl.SetVolume(volume)
	h.a.redraw()
}

func (h platformHost) Presentation() pluginapi.Presentation {
	s := h.a.settings
	return pluginapi.Presentation{
		Smoke: s.Smoke, Fog: s.Fog, LyricsStage: s.LyricsStage,
		StageStyle: s.StageStyle, StageSparks: s.StageSparks,
	}
}

func (h platformHost) SetLyricsPresentation(stage bool, style string, sparks bool) {
	s := &h.a.settings
	s.LyricsStage, s.StageStyle, s.StageSparks = stage, style, sparks
	_ = s.save()
}

func (h platformHost) DecodedArtwork(url string, size int, blurred bool) pluginapi.DecodedArtwork {
	if h.a.art == nil {
		return nil
	}
	if art := h.cachedArtwork(url, size, blurred); art != nil {
		return art
	}
	return nil
}

func (art *artwork) Bitmap() *ui.Bitmap  { return art.bmp }
func (art *artwork) Blurred() *ui.Bitmap { return art.backdrop }
func (art *artwork) Tint() ui.Color      { return art.tint }
func (art *artwork) Failed() bool        { return art.failed }

func (h platformHost) SetMainWindowButtonsTheme(theme mygo.ThemeSource) {
	if win := h.a.win; win != nil {
		win.SetWindowButtonsTheme(theme)
	}
}

func (h platformHost) DesktopLyricsVisible() bool {
	win := h.a.desktopLyricsWin
	return win != nil && win.IsVisible()
}

func (h platformHost) ToggleDesktopLyrics() { h.a.toggleDesktopLyrics() }

func (h platformHost) DesktopLyricsLocked() bool { return h.a.desktopLyricsLocked }
func (h platformHost) ToggleDesktopLyricsLock()  { h.a.toggleDesktopLyricsLock() }

func (h platformHost) DesktopLyricsState() pluginapi.DesktopLyricsState {
	a := h.a
	return pluginapi.DesktopLyricsState{
		Locked: a.desktopLyricsLocked, PointerInside: a.desktopLyricsPointerInside,
		ShowTranslation: a.desktopLyricsShowTranslation,
		FontSize:        a.desktopLyricsFontSize, Color: a.desktopLyricsColor,
	}
}

func (h platformHost) SetDesktopLyricsStyle(translation bool, size float32, color int) {
	a := h.a
	a.desktopLyricsShowTranslation, a.desktopLyricsFontSize, a.desktopLyricsColor = translation, size, color
	a.saveDesktopLyricsState()
}

func (h platformHost) HideDesktopLyrics() {
	if win := h.a.desktopLyricsWin; win != nil {
		win.Hide()
	}
}

func (h platformHost) Casting() pluginapi.CastingState {
	pl := h.a.pl
	if pl == nil || pl.DLNA() == nil {
		return pluginapi.CastingState{}
	}
	ctrl := pl.DLNA()
	state := pluginapi.CastingState{Available: true, Sending: pl.IsCasting(), Scanning: h.a.dlnaScanning}
	for _, device := range ctrl.Devices() {
		state.Devices = append(state.Devices, pluginapi.CastingDevice{ID: device.ID, Name: device.FriendlyName})
	}
	if device := ctrl.ActiveDevice(); device != nil {
		state.Active = &pluginapi.CastingDevice{ID: device.ID, Name: device.FriendlyName}
	}
	return state
}

func (h platformHost) ScanCastingDevices()            { h.a.scanDLNADevices() }
func (h platformHost) SwitchPlaybackOutput(id string) { h.a.pl.SwitchOutput(id) }
func (h platformHost) EqualizerAvailable() bool       { return h.a.eqAvailable() }

func (h platformHost) EqualizerSettings() eq.Settings {
	s := h.a.settings.EQ
	s.Bands = slices.Clone(s.Bands)
	return s
}

func (h platformHost) ApplyEqualizer(s eq.Settings) {
	s.Bands = slices.Clone(s.Bands)
	h.a.settings.EQ = s
	h.a.syncMPVSettings()
	h.a.redraw()
}

// playbackContinuity gives one activation access to the playback session.
type playbackContinuity struct {
	a   *app
	ctx context.Context
}

func (p playbackContinuity) active() bool {
	return p.a != nil && p.a.pl != nil && p.ctx != nil &&
		p.ctx == p.a.platformContext && p.ctx.Err() == nil
}

func (p playbackContinuity) Restore(s pluginapi.PlaybackSession) {
	if p.active() {
		p.a.pl.Restore(s)
	}
}
func (p playbackContinuity) RadioFrom(name string, tracks []pluginapi.Track, feed func(func([]pluginapi.Track))) {
	if p.active() {
		p.a.pl.RadioFrom(name, tracks, feed)
	}
}
func (p playbackContinuity) AttachRadio(name string, feed func(func([]pluginapi.Track))) {
	if p.active() {
		p.a.pl.AttachRadio(name, feed)
	}
}
func (p playbackContinuity) OpenAudio() (pluginapi.Audio, error) {
	if !p.active() {
		return nil, context.Canceled
	}
	return p.a.pl.OpenAudio()
}

var _ pluginapi.PlaybackContinuity = playbackContinuity{}

package app

import (
	"context"
	"slices"
	"time"

	"github.com/egoist/mygo"
	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/pluginapi/eq"
	"github.com/waygu-music-plugin/sdk/protocol"
)

func (h platformHost) Playback() pluginapi.PlaybackControls {
	if h.a.pl == nil {
		return nil
	}
	_, guard := h.authorization()
	return scopedPlayback{PlaybackControls: h.a.pl, write: guard}
}

type scopedPlayback struct {
	pluginapi.PlaybackControls
	write func() bool
}

func (p scopedPlayback) change(fn func()) {
	if allowedWrite(p.write) {
		fn()
	}
}

func (p scopedPlayback) LoadingText() string {
	return pluginapi.PlaybackLoadingText(p.PlaybackControls)
}
func (p scopedPlayback) TogglePlay()    { p.change(p.PlaybackControls.TogglePlay) }
func (p scopedPlayback) Prev()          { p.change(p.PlaybackControls.Prev) }
func (p scopedPlayback) Next()          { p.change(p.PlaybackControls.Next) }
func (p scopedPlayback) ToggleShuffle() { p.change(p.PlaybackControls.ToggleShuffle) }
func (p scopedPlayback) CycleRepeat()   { p.change(p.PlaybackControls.CycleRepeat) }
func (p scopedPlayback) EndRadio()      { p.change(p.PlaybackControls.EndRadio) }
func (p scopedPlayback) Seek(pos time.Duration) {
	p.change(func() { p.PlaybackControls.Seek(pos) })
}
func (p scopedPlayback) Jump(i int) { p.change(func() { p.PlaybackControls.Jump(i) }) }

func (h platformHost) Volume() float64 { return h.a.volume }

func (h platformHost) SetVolume(volume float64) {
	if !h.canWrite() {
		return
	}
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
	if !h.canWrite() {
		return
	}
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

func (h platformHost) SetMainWindowButtonsTheme(theme mygo.ThemeSource) {
	if !h.canWrite() {
		return
	}
	if win := h.a.win; win != nil {
		win.SetWindowButtonsTheme(theme)
	}
}

func (h platformHost) DesktopLyricsVisible() bool {
	return h.a.desktopLyrics.Visible()
}

func (h platformHost) ToggleDesktopLyrics() {
	if !h.canWrite() {
		return
	}
	h.a.desktopLyrics.Toggle()
	h.a.redraw()
}

func (h platformHost) DesktopLyricsLocked() bool {
	return h.a.localDesktopLyrics().State().Locked
}

func (h platformHost) ToggleDesktopLyricsLock() {
	if !h.canWrite() {
		return
	}
	h.a.localDesktopLyrics().ToggleLock()
	h.a.redraw()
}

func (h platformHost) DesktopLyricsState() pluginapi.DesktopLyricsState {
	return h.a.localDesktopLyrics().State()
}

func (h platformHost) SetDesktopLyricsStyle(translation bool, size float32, color int) {
	if !h.canWrite() {
		return
	}
	h.a.localDesktopLyrics().SetStyle(translation, size, color)
}

func (h platformHost) HideDesktopLyrics() {
	if !h.canWrite() {
		return
	}
	h.a.desktopLyrics.Hide()
}

func (h platformHost) Casting() pluginapi.CastingState {
	pl := h.a.pl
	if pl == nil || pl.DLNA() == nil {
		return pluginapi.CastingState{}
	}
	ctrl := pl.DLNA()
	scan, output := h.a.desktopTasks.latest[protocol.TaskCastingScan], h.a.desktopTasks.latest[protocol.TaskCastingOutput]
	state := pluginapi.CastingState{
		Available: true, Sending: pl.IsCasting(), Scanning: scan.Running(), ScanTask: scan, OutputTask: output,
	}
	for _, device := range ctrl.Devices() {
		state.Devices = append(state.Devices, pluginapi.CastingDevice{ID: device.ID, Name: device.FriendlyName})
	}
	if device := ctrl.ActiveDevice(); device != nil {
		state.Active = &pluginapi.CastingDevice{ID: device.ID, Name: device.FriendlyName}
	}
	return state
}

func (h platformHost) ScanCastingDevices() {
	if !h.canWrite() {
		return
	}
	if _, err := h.a.scanDLNADevices(); err != nil {
		h.Toast(err.Error())
	}
}
func (h platformHost) SwitchPlaybackOutput(id string) {
	if !h.canWrite() {
		return
	}
	if _, err := h.a.switchPlaybackOutput(id); err != nil {
		h.Toast(err.Error())
	}
}
func (h platformHost) EqualizerAvailable() bool { return h.a.eqAvailable() }

func (h platformHost) EqualizerSettings() eq.Settings {
	s := h.a.settings.EQ
	s.Bands = slices.Clone(s.Bands)
	return s
}

func (h platformHost) ApplyEqualizer(s eq.Settings) {
	if !h.canWrite() {
		return
	}
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

func (p playbackContinuity) ownsTracks(tracks []pluginapi.Track) bool {
	return p.a.ownsTracksFor(p.a.platformID, tracks)
}

func (p playbackContinuity) Restore(s pluginapi.PlaybackSession) {
	if p.active() && p.ownsTracks(s.Queue) {
		p.a.pl.Restore(s)
	}
}
func (p playbackContinuity) RadioFrom(name string, tracks []pluginapi.Track, feed pluginapi.QueueFeed) {
	if p.active() && p.ownsTracks(tracks) {
		p.a.pl.RadioFrom(name, tracks, scopedQueueFeed(p.ctx, p.active, p.ownsTracks, feed))
	}
}
func (p playbackContinuity) AttachRadio(name string, feed pluginapi.QueueFeed) {
	if p.active() {
		p.a.pl.AttachRadio(name, scopedQueueFeed(p.ctx, p.active, p.ownsTracks, feed))
	}
}
func (p playbackContinuity) OpenAudio() (pluginapi.Audio, error) {
	if !p.active() {
		return nil, context.Canceled
	}
	return p.a.pl.OpenAudioFor(p.ctx)
}

var _ pluginapi.PlaybackContinuity = playbackContinuity{}

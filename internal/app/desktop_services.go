package app

import (
	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/protocol"

	"waygu-music/internal/player/mpv"
)

func (h platformHost) DesktopSettings() pluginapi.DesktopSettings {
	a, s := h.a, &h.a.settings
	state := pluginapi.DesktopSettings{
		Preferences: pluginapi.DesktopPreferences{
			Appearance: s.Appearance, AppIcon: s.AppIcon, CacheMB: s.CacheMB,
			Notch: s.Notch, NotchActivity: s.NotchActivity, NotchVirtual: s.NotchVirtual,
			Smoke: s.Smoke, Fog: s.Fog,
			UseMPV: s.UseMPV, MPVExclusive: s.MPVExclusive, MPVGapless: s.MPVGapless,
			MPVAudioDevice: s.MPVAudioDevice,
		},
		CacheUsed:      a.cacheUsed,
		CacheTask:      a.desktopTasks.latest[protocol.TaskCacheClear],
		NotchAvailable: a.quickPlayerWindow() != nil,
		Updates:        a.updates,
	}
	if a.updates {
		state.UpdateChecks, state.UpdateDownloads = updateChecks(), updateDownloads()
		state.LastUpdateCheck = lastUpdateCheck()
	}
	return state
}

func (a *app) quickPlayerPreferences() pluginapi.QuickPlayerPreferences {
	s := &a.settings
	return pluginapi.QuickPlayerPreferences{
		Enabled: s.Notch, Activity: s.NotchActivity,
		Virtual: s.NotchVirtual, Greeted: s.NotchGreeted,
	}
}

func (a *app) saveQuickPlayerPreferences(p pluginapi.QuickPlayerPreferences) {
	s := &a.settings
	s.Notch, s.NotchActivity, s.NotchVirtual, s.NotchGreeted = p.Enabled, p.Activity, p.Virtual, p.Greeted
	_ = s.save()
	if a.settingsWin != nil {
		a.settingsWin.Invalidate()
	}
}

func (h platformHost) ApplyDesktopPreferences(p pluginapi.DesktopPreferences) {
	if !h.canWrite() {
		return
	}
	if err := h.a.applyDesktopPreferences(p); err != nil {
		h.Toast(err.Error())
	}
}

func (a *app) applyDesktopPreferences(p pluginapi.DesktopPreferences) error {
	next := a.settings
	s := &next
	cacheChanged := s.CacheMB != p.CacheMB
	iconChanged := s.AppIcon != p.AppIcon
	appearanceChanged := s.Appearance != p.Appearance
	audioChanged := s.UseMPV != p.UseMPV || s.MPVExclusive != p.MPVExclusive ||
		s.MPVGapless != p.MPVGapless || s.MPVAudioDevice != p.MPVAudioDevice
	s.CacheMB, s.AppIcon, s.Appearance = p.CacheMB, p.AppIcon, p.Appearance
	s.Notch, s.NotchActivity, s.NotchVirtual = p.Notch, p.NotchActivity, p.NotchVirtual
	s.Smoke, s.Fog = p.Smoke, p.Fog
	s.UseMPV, s.MPVExclusive, s.MPVGapless = p.UseMPV, p.MPVExclusive, p.MPVGapless
	s.MPVAudioDevice = p.MPVAudioDevice
	if err := s.save(); err != nil {
		return err
	}
	a.settings = next
	if appearanceChanged {
		s.applyAppearance()
	}
	if cacheChanged {
		a.applyCacheLimit()
	}
	if iconChanged {
		a.changeAppIcon()
	}
	if audioChanged {
		a.syncMPVSettings()
	}
	a.desktopTasksChanged()
	return nil
}

func (h platformHost) DecoderStatus() pluginapi.DecoderStatus {
	a := h.a
	download, remove := a.desktopTasks.latest[protocol.TaskDecoderDownload], a.desktopTasks.latest[protocol.TaskDecoderDelete]
	status := pluginapi.DecoderStatus{
		Ready: a.mpvReady(), Downloading: download.Running(), Done: a.mpvDone, Total: a.mpvTotal,
		DownloadTask: download, DeleteTask: remove,
	}
	if !status.Downloading {
		status.Downloaded = mpv.IsDownloaded()
	}
	if a.settings.UseMPV && status.Ready && a.pl != nil {
		for _, device := range a.pl.AudioDevices() {
			status.Devices = append(status.Devices, pluginapi.AudioDevice{
				Name: device.Name, Description: device.Description,
			})
		}
	}
	return status
}

func (h platformHost) ClearAudioCache() {
	if !h.canWrite() {
		return
	}
	if _, err := h.a.clearCache(); err != nil {
		h.Toast(err.Error())
	}
}
func (h platformHost) StartDecoderDownload() {
	if !h.canWrite() {
		return
	}
	if _, err := h.a.startMPVDownload(); err != nil {
		h.Toast(err.Error())
	}
}
func (h platformHost) CancelDecoderDownload() {
	if !h.canWrite() {
		return
	}
	_, _ = h.a.cancelMPVDownload(0, protocol.TaskCanceled)
}
func (h platformHost) DeleteDecoder() {
	if !h.canWrite() {
		return
	}
	h.a.deleteMPV()
}
func (h platformHost) SetUpdateChecks(on bool) {
	if !h.canWrite() {
		return
	}
	setUpdateChecks(on)
}
func (h platformHost) SetUpdateDownloads(on bool) {
	if !h.canWrite() {
		return
	}
	setUpdateDownloads(on)
}
func (h platformHost) CheckForUpdates() {
	if !h.canWrite() {
		return
	}
	checkForUpdates()
}

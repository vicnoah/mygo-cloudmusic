package app

import (
	"time"

	"waygu-music/internal/player/mpv"
	"waygu-music/internal/pluginapi"
)

func (h platformHost) DesktopSettings() pluginapi.DesktopSettings {
	a, s := h.a, &h.a.settings
	var remaining int
	if a.devClicks >= 3 && a.devClicks < 5 {
		if time.Since(a.devClickTime) <= 3*time.Second {
			remaining = 5 - a.devClicks
		} else {
			a.devClicks = 0
		}
	}
	state := pluginapi.DesktopSettings{
		Preferences: pluginapi.DesktopPreferences{
			Appearance: s.Appearance, AppIcon: s.AppIcon, CacheMB: s.CacheMB,
			Notch: s.Notch, NotchActivity: s.NotchActivity, NotchVirtual: s.NotchVirtual,
			Smoke: s.Smoke, Fog: s.Fog,
			UseMPV: s.UseMPV, MPVExclusive: s.MPVExclusive, MPVGapless: s.MPVGapless,
			MPVAudioDevice: s.MPVAudioDevice,
		},
		CacheUsed:              a.cacheUsed,
		NotchAvailable:         a.quickPlayerWindow() != nil,
		Updates:                a.updates,
		DeveloperMode:          s.DeveloperMode,
		VersionClicksRemaining: remaining,
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
	a, s := h.a, &h.a.settings
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
	_ = s.save()
	if appearanceChanged {
		s.applyAppearance()
	}
	a.redraw()
	if cacheChanged {
		a.applyCacheLimit()
	}
	if iconChanged {
		a.changeAppIcon()
	}
	if audioChanged {
		a.syncMPVSettings()
	}
}

func (h platformHost) DecoderStatus() pluginapi.DecoderStatus {
	a := h.a
	status := pluginapi.DecoderStatus{
		Ready: a.mpvReady(), Downloading: a.mpvDownloading, Done: a.mpvDone, Total: a.mpvTotal,
	}
	if !status.Downloading {
		status.Downloaded = mpv.IsDownloaded()
		if !status.Downloaded {
			status.SystemPath = mpv.SystemPath()
		}
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

func (h platformHost) ClearAudioCache()           { h.a.clearCache() }
func (h platformHost) StartDecoderDownload()      { h.a.startMPVDownload() }
func (h platformHost) CancelDecoderDownload()     { h.a.cancelMPVDownload() }
func (h platformHost) DeleteDecoder()             { h.a.deleteMPV() }
func (h platformHost) SetUpdateChecks(on bool)    { setUpdateChecks(on) }
func (h platformHost) SetUpdateDownloads(on bool) { setUpdateDownloads(on) }
func (h platformHost) CheckForUpdates()           { checkForUpdates() }
func (h platformHost) RecordVersionClick()        { h.a.onVersionClicked() }

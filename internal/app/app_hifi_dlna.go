package app

import (
	"context"
	"fmt"
	"time"

	"waygu-music/internal/player/mpv"
)

func (a *app) syncMPVSettings() {
	if a.pl == nil {
		return
	}
	a.settings.EQ.Normalize()
	a.pl.SetUseMPV(a.settings.UseMPV)
	a.pl.SetMPVConfig(mpv.Config{
		Exclusive:        a.settings.MPVExclusive,
		ReplayGain:       a.settings.MPVReplayGain,
		AudioDevice:      a.settings.MPVAudioDevice,
		Gapless:          a.settings.MPVGapless,
		EqualizerEnabled: a.settings.EQ.Enabled,
		EqualizerType:    a.settings.EQ.Type,
		EqualizerPreamp:  a.settings.EQ.Preamp,
		EqualizerBands:   a.settings.EQ.Bands,
	})
}

// eqAvailable reports whether the equalizer has an MPV engine to run in.
func (a *app) eqAvailable() bool {
	return a.pl != nil && a.pl.UsingMPV()
}

func (a *app) mpvReady() bool {
	return mpv.Ready()
}

func (a *app) startMPVDownload() {
	if a.mpvDownloading {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.mpvCancel = cancel
	a.mpvDownloading = true
	a.mpvDone, a.mpvTotal = 0, 0
	a.saveDesktopSettings()

	go func() {
		err := mpv.Download(ctx, func(done, total int64) {
			a.post(func() {
				a.mpvDone = done
				a.mpvTotal = total
			})
		})
		a.post(func() {
			a.mpvDownloading = false
			a.mpvCancel = nil
			if err == nil {
				_ = mpv.Load()
				a.syncMPVSettings()
				a.toast = "高级解码组件下载完成"
			} else if ctx.Err() == nil {
				a.toast = fmt.Sprintf("下载失败: %v", err)
			}
			a.saveDesktopSettings()
		})
	}()
}

func (a *app) cancelMPVDownload() {
	if a.mpvCancel != nil {
		a.mpvCancel()
		a.mpvCancel = nil
	}
	a.mpvDownloading = false
	a.saveDesktopSettings()
}

func (a *app) deleteMPV() {
	if a.pl != nil {
		a.pl.SetUseMPV(false)
	}
	a.settings.UseMPV = false
	_ = mpv.Delete()
	a.saveDesktopSettings()
	a.syncMPVSettings()
	a.toast = "已删除高级解码组件"
}

func (a *app) scanDLNADevices() {
	if a.pl == nil {
		return
	}
	ctrl := a.pl.DLNA()
	if ctrl == nil || a.dlnaScanning {
		return
	}
	a.dlnaScanning = true
	a.redraw()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_, _ = ctrl.RefreshDevices(ctx)
		a.post(func() {
			a.dlnaScanning = false
			a.redraw()
		})
	}()
}

func (a *app) saveDesktopSettings() {
	_ = a.settings.save()
	a.redraw()
}

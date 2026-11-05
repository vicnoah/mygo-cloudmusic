package app

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/player/mpv"
)

func (a *app) syncMPVSettings() {
	if a.pl == nil {
		return
	}
	a.pl.SetUseMPV(a.settings.UseMPV)
	a.pl.SetMPVConfig(mpv.Config{
		Exclusive:  a.settings.MPVExclusive,
		ReplayGain: a.settings.MPVReplayGain,
		Gapless:    a.settings.MPVGapless,
	})
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
	a.changed()

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
			a.changed()
		})
	}()
}

func (a *app) cancelMPVDownload() {
	if a.mpvCancel != nil {
		a.mpvCancel()
		a.mpvCancel = nil
	}
	a.mpvDownloading = false
	a.changed()
}

func (a *app) deleteMPV() {
	if a.pl != nil {
		a.pl.SetUseMPV(false)
	}
	a.settings.UseMPV = false
	_ = mpv.Delete()
	a.changed()
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

// renderHifiSettings renders the "高级音频与硬件解码" section into any settings page (both NetEase and third-party platforms).
func (a *app) renderHifiSettings(c *ui.Context, group func(title string, rows func()), row func(key, label, desc string, control func())) {
	s := &a.settings
	group("高级音频与硬件解码", func() {
		row("hifi-mpv-toggle", "高级解码引擎 (MPV)", "基于 MPV 的发烧级解码引擎，支持 DSD (DSF/DFF)、APE、ALAC、AAC、Hi-Res FLAC 全格式直接硬件输出", func() {
			if ui.Switch(c.Key("use-mpv"), &s.UseMPV).Label("高级解码引擎").Disabled(!a.mpvReady()).Changed() {
				a.changed()
				a.syncMPVSettings()
			}
		})

		var statusDesc string
		switch {
		case a.mpvDownloading:
			statusDesc = fmt.Sprintf("正在下载 libmpv (%s / %s)...", megabytes(a.mpvDone), megabytes(a.mpvTotal))
		case mpv.IsDownloaded():
			statusDesc = "组件已就绪 (独立组件已缓存)"
		case mpv.SystemPath() != "":
			statusDesc = "系统已安装 libmpv (" + filepath.Base(mpv.SystemPath()) + ")，亦可下载独立组件"
		default:
			statusDesc = "未下载组件，下载后即可开启高级解码引擎"
		}

		row("hifi-mpv-status", "解码组件状态", statusDesc, func() {
			switch {
			case a.mpvDownloading:
				if ui.Button(c.Key("mpv-dl"), "取消").Clicked() {
					a.cancelMPVDownload()
				}
			case mpv.IsDownloaded():
				if ui.Button(c.Key("mpv-dl"), "删除组件").Clicked() {
					a.deleteMPV()
				}
			default:
				if ui.Button(c.Key("mpv-dl"), "下载组件").Clicked() {
					a.startMPVDownload()
				}
			}
		})

		if s.UseMPV && a.mpvReady() {
			row("hifi-mpv-exclusive", "独占音频输出 (Exclusive Mode)", "绕过系统音频混音器，提供无损 Bit-perfect 直通原音输出", func() {
				if ui.Switch(c.Key("mpv-exclusive"), &s.MPVExclusive).Label("独占音频输出").Changed() {
					a.changed()
					a.syncMPVSettings()
				}
			})
			row("hifi-mpv-gapless", "无缝切歌 (Gapless Playback)", "连续曲目切换时保持音频流连续，消除曲目间隙缓冲与爆音", func() {
				if ui.Switch(c.Key("mpv-gapless"), &s.MPVGapless).Label("无缝切歌").Changed() {
					a.changed()
					a.syncMPVSettings()
				}
			})
		}
	})
}

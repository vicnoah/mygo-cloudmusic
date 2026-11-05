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
	spdif := a.audioSpdifValue()
	mode := a.settings.spatialMode()
	channels := a.settings.MPVAudioChannels
	switch mode {
	case "speakers":
		channels = multichannelLayout
	case "headphones", "stereo":
		channels = "stereo"
	}
	if channels == "" {
		channels = "auto-safe"
	}
	a.pl.SetUseMPV(a.settings.UseMPV)
	a.pl.SetMPVConfig(mpv.Config{
		Exclusive:        a.settings.MPVExclusive,
		ReplayGain:       a.settings.MPVReplayGain,
		AudioDevice:      a.settings.MPVAudioDevice,
		AudioChannels:    channels,
		AudioSpdif:       spdif,
		SpatialMode:      mode,
		HrtfProfile:      a.settings.hrtfProfile(),
		Gapless:          a.settings.MPVGapless,
		EqualizerEnabled: a.settings.EqualizerEnabled,
		EqualizerPreamp:  a.settings.EqualizerPreamp,
		EqualizerGains:   a.settings.EqualizerGains,
	})
	a.pl.SetEqualizer(a.settings.EqualizerPreamp, a.settings.EqualizerGains, a.settings.EqualizerEnabled)
}

// audioSpdifValue returns the MPV audio-spdif value only when the user has
// explicitly selected an HDMI/SPDIF receiver. Built-in speakers and "auto"
// must stay on MPV's normal decoder: they expose only PCM substreams, so
// forcing compressed passthrough makes CoreAudio fail before E-AC-3 can be
// decoded. The MPV engine also retries without passthrough if a receiver
// rejects the stream.
func (a *app) audioSpdifValue() string {
	if !a.settings.MPVAudioSpdif || a.pl == nil {
		return ""
	}
	dev := a.settings.MPVAudioDevice
	if dev == "" || dev == "auto" {
		return ""
	}
	for _, d := range a.pl.AudioDevices() {
		if d.Name == dev {
			if mpv.SpdifCapableDevice(d) {
				return "eac3"
			}
			return ""
		}
	}
	return ""
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
			devices := a.pl.AudioDevices()
			if len(devices) > 0 {
				var devLabels []string
				chosenDevLabel := "系统默认 (自动选择)"
				devNameToLabel := make(map[string]string)
				labelToDevName := make(map[string]string)
				labelCounts := make(map[string]int)

				for _, d := range devices {
					lbl := d.Description
					if lbl == "" {
						lbl = d.Name
					}
					if d.Name == "auto" {
						lbl = "系统默认 (自动选择)"
					}
					labelCounts[lbl]++
				}

				for _, d := range devices {
					lbl := d.Description
					if lbl == "" {
						lbl = d.Name
					}
					if d.Name == "auto" {
						lbl = "系统默认 (自动选择)"
					} else if labelCounts[lbl] > 1 {
						lbl = fmt.Sprintf("%s (%s)", lbl, d.Name)
					}
					devLabels = append(devLabels, lbl)
					devNameToLabel[d.Name] = lbl
					labelToDevName[lbl] = d.Name
				}

				if curLbl, ok := devNameToLabel[s.MPVAudioDevice]; ok {
					chosenDevLabel = curLbl
				} else if s.MPVAudioDevice == "" || s.MPVAudioDevice == "auto" {
					chosenDevLabel = "系统默认 (自动选择)"
				}

				row("hifi-mpv-device", "音频输出设备", "独立选择音频路由设备 (如耳机、板载声卡或外置 USB DAC/耳放)", func() {
					if ui.Select(c.Key("mpv-audio-device"), &chosenDevLabel, devLabels).Label("音频输出设备").Width(220).Changed() {
						if targetName, ok := labelToDevName[chosenDevLabel]; ok {
							s.MPVAudioDevice = targetName
							a.changed()
							a.syncMPVSettings()
						}
					}
				})
			}

			row("hifi-mpv-spatial", "空间音频模式", "自动跟随系统；多声道扬声器适合 HDMI 功放或 USB 多声道声卡；耳机模式在无 SOFA/HRTF 组件时使用低串扰立体声下混；高清臻音已是空间化立体声，不会再叠加 HRTF", func() {
				labels := []string{"自动（跟随系统）", "多声道扬声器（5.1 / 7.1.4）", "耳机空间化（串扰下混）", "立体声（下混）"}
				values := []string{"auto", "speakers", "headphones", "stereo"}
				chosen := labels[0]
				mode := s.spatialMode()
				for i, v := range values {
					if v == mode {
						chosen = labels[i]
					}
				}
				if ui.Select(c.Key("mpv-spatial-mode"), &chosen, labels).Label("空间音频模式").Width(220).Changed() {
					for i, label := range labels {
						if label == chosen {
							s.SpatialMode = values[i]
						}
					}
					a.changed()
					a.syncMPVSettings()
				}
			})
			if s.spatialMode() == "headphones" {
				row("hifi-mpv-hrtf", "耳机空间化强度", "均衡适合日常聆听；宽广会加深串扰与声场宽度，可能牺牲一点人声定位", func() {
					labels := []string{"均衡", "宽广"}
					values := []string{"balanced", "wide"}
					chosen := labels[0]
					for i, v := range values {
						if v == s.hrtfProfile() {
							chosen = labels[i]
						}
					}
					if ui.Select(c.Key("mpv-hrtf-profile"), &chosen, labels).Label("耳机空间化强度").Width(220).Changed() {
						for i, label := range labels {
							if label == chosen {
								s.HrtfProfile = values[i]
							}
						}
						a.changed()
						a.syncMPVSettings()
					}
				})
			}
			spdifDesc := "内置扬声器会自动把 E-AC-3 解为 PCM；只有已选择具体 HDMI/SPDIF 接收器时才尝试直通，若设备拒绝会自动回退到 PCM 解码"
			if a.audioSpdifValue() != "" {
				spdifDesc = "已选择 HDMI/SPDIF 输出，将尝试把 E-AC-3 原样交给功放；若设备拒绝会自动回退到 PCM 解码"
			} else if s.MPVAudioSpdif {
				spdifDesc = "当前输出设备不支持压缩直通，已自动使用 PCM 解码（内置扬声器无需也无法做 bitstream 直通）"
			}
			row("hifi-mpv-spdif", "杜比 E-AC-3 直通（仅 HDMI 功放）", spdifDesc, func() {
				if ui.Switch(c.Key("mpv-spdif"), &s.MPVAudioSpdif).Label("杜比 E-AC-3 直通").Changed() {
					a.changed()
					a.syncMPVSettings()
				}
			})
			row("hifi-mpv-exclusive", "独占音频输出 (Exclusive Mode)", "仅立体声 PCM 曲目使用；空间音频与杜比会自动走系统混音，保证 macOS 音量键和音量滑块有效", func() {
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

			row("hifi-mpv-eq-info", "图形均衡器 (10-Band EQ)", "基于专业音频滤镜的 10 段图形调音台，已整合至底部播控条右侧，可随时快捷唤出", func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, iconEqualizer).Size(14, 14).TextColor(c.Theme().TextMuted)
					ui.Text(c, "播控条已启用快捷入口").FontSize(12).TextColor(c.Theme().TextMuted)
				})
			})
		}
	})
}

func (a *app) drawEqSlider(c *ui.Context, key string, val *float64, label string) {
	const knob, track = 16, 4
	ui.Column(c).AlignItems(ui.Center).Gap(3).Children(func() {
		ui.Text(c, fmt.Sprintf("%+.0f", *val)).FontSize(10).TextColor(c.Theme().TextMuted).SingleLine()
		s := ui.SliderBase(c.Key(key), val, -12, 12).Vertical().Size(knob+10, 84).PaddingY(knob / 2).FocusRing(false).Label(label)
		if s.Changed() {
			a.settings.EqualizerPreset = "custom"
			a.changed()
			a.syncMPVSettings()
		}
		v := float32((*val - (-12)) / (12 - (-12)))
		accent := a.pal.accent
		dark := c.Theme().Dark
		trackColor := ui.RGBA(128, 128, 128, 0.25)
		knobColor := ui.RGBA(255, 255, 255, 0.95)
		if dark {
			knobColor = ui.RGBA(220, 220, 220, 0.95)
		}
		pressed := s.Pressed()
		s.Draw(func(p *ui.Painter, r ui.Rect) {
			top, foot := r.Y+knob/2, r.Y+r.H-knob/2
			x := r.X + (r.W-track)/2
			y := foot - (foot-top)*v
			p.Fill(ui.Rect{X: x, Y: top - track/2, W: track, H: foot - top + track}, trackColor, track/2)
			p.Fill(ui.Rect{X: x, Y: y, W: track, H: foot - y + track/2}, accent, track/2)
			k := ui.Rect{X: r.X + (r.W-knob)/2, Y: y - knob/2, W: knob, H: knob}
			p.Shadow(k, knob/2, 0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.2))
			fill := knobColor
			if pressed {
				fill = fill.Mix(ui.RGB(0, 0, 0), 0.1)
			}
			p.Fill(k, fill, knob/2)
			p.Stroke(k, ui.RGBA(0, 0, 0, 0.1), knob/2, 0.5)
		})
		ui.Text(c, label).FontSize(11).SingleLine()
	})
}

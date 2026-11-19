package netease

import (
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo/ui"
)

func (a *app) renderHifiSettings(c *ui.Context, group func(title string, rows func()), row func(key, label, desc string, control func())) {
	s := &a.settings
	decoder := a.host.DecoderStatus()
	group("高级音频与硬件解码", func() {
		row("hifi-mpv-toggle", "高级解码引擎 (MPV)", "基于 MPV 的发烧级解码引擎，支持 DSD (DSF/DFF)、APE、ALAC、AAC、Hi-Res FLAC 全格式直接硬件输出", func() {
			if ui.Switch(c.Key("use-mpv"), &s.UseMPV).Label("高级解码引擎").Disabled(!decoder.Ready).Changed() {
				a.applyDesktopPreferences()
			}
		})

		var statusDesc string
		switch {
		case decoder.Downloading:
			statusDesc = fmt.Sprintf("正在下载 libmpv (%s / %s)...", megabytes(decoder.Done), megabytes(decoder.Total))
		case decoder.Downloaded:
			statusDesc = "组件已就绪 (独立组件已缓存)"
		case decoder.SystemPath != "":
			statusDesc = "系统已安装 libmpv (" + filepath.Base(decoder.SystemPath) + ")，亦可下载独立组件"
		default:
			statusDesc = "未下载组件，下载后即可开启高级解码引擎"
		}

		row("hifi-mpv-status", "解码组件状态", statusDesc, func() {
			switch {
			case decoder.Downloading:
				if ui.Button(c.Key("mpv-dl"), "取消").Clicked() {
					a.host.CancelDecoderDownload()
				}
			case decoder.Downloaded:
				if ui.Button(c.Key("mpv-dl"), "删除组件").Clicked() {
					a.host.DeleteDecoder()
				}
			default:
				if ui.Button(c.Key("mpv-dl"), "下载组件").Clicked() {
					a.host.StartDecoderDownload()
				}
			}
		})

		if s.UseMPV && decoder.Ready {
			devices := decoder.Devices
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
					if ui.Select(c.Key("mpv-audio-device"), &chosenDevLabel, devLabels).Label("音频输出设备").Width(240).Changed() {
						if targetName, ok := labelToDevName[chosenDevLabel]; ok {
							s.MPVAudioDevice = targetName
							a.applyDesktopPreferences()
						}
					}
				})
			}

			row("hifi-mpv-exclusive", "独占音频输出 (Exclusive Mode)", "绕过系统音频混音器，提供无损 Bit-perfect 直通原音输出", func() {
				if ui.Switch(c.Key("mpv-exclusive"), &s.MPVExclusive).Label("独占音频输出").Changed() {
					a.applyDesktopPreferences()
				}
			})
			row("hifi-mpv-gapless", "无缝切歌 (Gapless Playback)", "连续曲目切换时保持音频流连续，消除曲目间隙缓冲与爆音", func() {
				if ui.Switch(c.Key("mpv-gapless"), &s.MPVGapless).Label("无缝切歌").Changed() {
					a.applyDesktopPreferences()
				}
			})

			row("hifi-mpv-eq-info", "图形均衡器 (ISO 10/15 段)", "基于专业音频滤镜的图形调音台，已整合至底部播控条右侧，可随时快捷唤出", func() {
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
		s := ui.SliderBase(c.Key(key), val, -12, 12).Vertical().Size(knob+8, 84).PaddingY(knob / 2).FocusRing(false).Label(label)
		if s.Changed() {
			a.settings.EQ.Preset = ""
			a.settings.EQ.AutoEQPath, a.settings.EQ.AutoEQName = "", ""
			a.eqChanged()
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

package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/player/mpv"
)

func (a *app) eqButton(c *ui.Context, pal palette) {
	s := &a.settings
	color := pal.muted
	if a.eqOpen {
		color = pal.accent
	}

	b := iconButton(c.Key("eq-button"), iconEqualizer, 16, color, "图形均衡器 (EQ)")
	if b.Clicked() {
		a.eqOpen = !a.eqOpen
	}

	pop := popoverColors(pal.dark)
	ui.PopoverBase(c.Key("eq-popover"), b, &a.eqOpen, func(panel ui.Element) {
		const arrow = 7
		panel.AttachTo(b, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, arrow+3, 0).Width(346).Padding(12, 14).
			Radius(12).Background(pop.fill).Border(0.5, pop.edge).Shadow(0, 8, 24, -4, ui.RGBA(0, 0, 0, 0.2)).
			Gap(10).Label("图形均衡器")
		panel.Draw(func(p *ui.Painter, r ui.Rect) {
			cx, foot := r.X+r.W/2, r.Y+r.H
			tip := (&ui.Path{}).MoveTo(cx-arrow-1, foot-0.75).LineTo(cx-1.2, foot+arrow-1.2).
				QuadTo(cx, foot+arrow, cx+1.2, foot+arrow-1.2).LineTo(cx+arrow+1, foot-0.75).Close()
			p.FillPath(tip, pop.fill)
			edge := (&ui.Path{}).MoveTo(cx-arrow-1, foot).LineTo(cx-1.2, foot+arrow-1.2).
				QuadTo(cx, foot+arrow, cx+1.2, foot+arrow-1.2).LineTo(cx+arrow+1, foot)
			p.StrokePath(edge, 0.5, pop.edge)
		})

		// 1. Header row: Title + Switch
		ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconEqualizer).Size(14, 14).TextColor(pal.text)
				ui.Text(c, "图形均衡器 (10-Band EQ)").FontSize(12.5).FontWeight(600).TextColor(pal.text)
			})
			if ui.Switch(c.Key("eq-pop-toggle"), &s.EqualizerEnabled).Label("启用均衡器").Changed() {
				a.changed()
				a.syncMPVSettings()
			}
		})

		if !s.EqualizerEnabled {
			ui.Box(c).Padding(16, 4).Center().Children(func() {
				ui.Text(c, "当前为原声直通模式，点击右上角开关开启 10 段调音").FontSize(11.5).TextColor(c.Theme().TextMuted).SingleLine()
			})
			return
		}

		// 2. Preset & Reset row
		presetNames := []string{"原声直通", "流行人声", "低音增强", "清澈高音", "摇滚", "古典", "电子舞曲", "自定义"}
		chosen := "原声直通"
		for _, p := range mpv.Presets {
			if p.ID == s.EqualizerPreset {
				chosen = p.Name
			}
		}
		if s.EqualizerPreset == "custom" {
			chosen = "自定义"
		}

		ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "预设").FontSize(11.5).TextColor(c.Theme().TextMuted)
				if ui.Select(c.Key("eq-pop-preset"), &chosen, presetNames).Label("预设音效").Width(120).Changed() {
					for _, p := range mpv.Presets {
						if p.Name == chosen {
							s.EqualizerPreset = p.ID
							s.EqualizerGains = p.Gains
							break
						}
					}
					if chosen == "自定义" {
						s.EqualizerPreset = "custom"
					}
					a.changed()
					a.syncMPVSettings()
				}
			})
			if ui.Button(c.Key("eq-pop-reset"), "重置").Clicked() {
				s.EqualizerPreset = "flat"
				s.EqualizerPreamp = 0
				s.EqualizerGains = [10]float64{}
				a.changed()
				a.syncMPVSettings()
			}
		})

		// 3. Preamp row
		ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "前置增益").FontSize(11.5).TextColor(c.Theme().TextMuted)
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				sl := ui.Slider(c.Key("eq-pop-preamp"), &s.EqualizerPreamp, -12, 12).Width(140)
				if sl.Changed() {
					a.changed()
					a.syncMPVSettings()
				}
				ui.Text(c, fmt.Sprintf("%+.1f dB", s.EqualizerPreamp)).FontSize(11).TextColor(c.Theme().TextMuted).Width(48).TextAlign(ui.End).SingleLine()
			})
		})

		// 4. 10-Band Sliders console
		ui.Row(c).Gap(5).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			for i := range mpv.ISO10Bands {
				idx := i
				label := mpv.ISO10BandLabels[idx]
				val := &s.EqualizerGains[idx]
				a.drawEqSlider(c, fmt.Sprintf("eq-pop-band-%d", idx), val, label)
			}
		})
	})
}

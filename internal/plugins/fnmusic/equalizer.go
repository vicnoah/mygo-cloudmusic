package fnmusic

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi/eq"
	"waygu-music/internal/pluginapi/storage"
)

// eqUIState is the equalizer's transient interface state: its managers,
// the presets it has read, and the dialogs and AutoEQ list it shows.
type eqUIState struct {
	presets *eq.PresetStore
	autoeq  *eq.AutoEQ
	loaded  []eq.Preset

	saveOpen bool
	saveName string
	saveErr  string

	deleteOpen bool
	deleteName string

	autoeqOpen    bool
	autoeqQuery   string
	autoeqLoading bool
	autoeqErr     string
	autoeqMetas   []eq.Meta
	autoeqBusy    string
}

func (h *Home) eqManagers() (*eq.PresetStore, *eq.AutoEQ) {
	if h.eqUI.presets == nil {
		path, err := storage.LegacyDataPath("eq_presets")
		if err != nil {
			path = "eq_presets"
		}
		h.eqUI.presets = eq.NewPresetStore(path)
	}
	if h.eqUI.autoeq == nil {
		path, err := storage.LegacyCachePath("autoeq")
		if err != nil {
			path = "autoeq"
		}
		h.eqUI.autoeq = eq.NewAutoEQ(path, 20*time.Second)
	}
	return h.eqUI.presets, h.eqUI.autoeq
}

func (h *Home) loadEQPresets() {
	store, _ := h.eqManagers()
	h.eqUI.loaded = store.List()
}

// eqChanged redraws the windows after a change to the equalizer. The
// settings themselves are saved when the app quits: a slider drag changes
// them many times a second, and each would otherwise write the file.
func (h *Home) eqChanged() { h.host.Invalidate() }

// eqButton is the graphic equalizer's button and panel, over the player.
// It shows only while MPV plays, where the equalizer runs.
func (h *Home) eqButton(c *ui.Context, pal palette) {
	if !h.host.EqualizerAvailable() {
		h.eqOpen = false
		return
	}
	s := &h.eqSettings
	s.Normalize()
	if h.eqUI.loaded == nil {
		h.loadEQPresets()
	}

	color := pal.muted
	if h.eqOpen {
		color = pal.accent
	}
	b := iconButton(c.Key("eq-button"), iconEqualizer, 16, color, "图形均衡器 (EQ)")
	if b.Clicked() {
		h.eqOpen = !h.eqOpen
	}

	pop := popoverColors(pal.dark)
	ui.PopoverBase(c.Key("eq-popover"), b, &h.eqOpen, func(panel ui.Element) {
		const arrow = 7
		panel.AttachTo(b, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, arrow+3, 0).Width(460).Padding(14, 16).
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

		ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconEqualizer).Size(14, 14).TextColor(pal.text)
				ui.Text(c, "图形均衡器").FontSize(12.5).FontWeight(600).TextColor(pal.text)
			})
			if ui.Switch(c.Key("eq-pop-toggle"), &s.Enabled).Label("启用均衡器").Changed() {
				h.eqChanged()
				h.host.ApplyEqualizer(h.eqSettings)
			}
		})

		h.eqPresetRow(c, pal)
		h.eqTypeRow(c, pal)
		h.eqPreampRow(c, pal)
		h.eqSliders(c, pal)
		h.eqAutoEQRow(c, pal)
	})

	h.eqSaveDialog(c)
	h.eqDeleteDialog(c)
	h.eqAutoEQDialog(c)
}

func (h *Home) eqPresetRow(c *ui.Context, pal palette) {
	s := &h.eqSettings
	names := make([]string, 0, len(h.eqUI.loaded)+1)
	chosen := "自定义"
	for _, p := range h.eqUI.loaded {
		label := p.Label
		if label == "" {
			label = p.Name
		}
		names = append(names, label)
		if p.Name == s.Preset {
			chosen = label
		}
	}
	names = append(names, "自定义")

	ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "预设").FontSize(11.5).TextColor(pal.muted)
			if ui.Select(c.Key("eq-pop-preset"), &chosen, names).Label("预设音效").Width(130).Changed() {
				if chosen == "自定义" {
					s.Preset = ""
					h.eqChanged()
					h.host.ApplyEqualizer(h.eqSettings)
					return
				}
				for _, p := range h.eqUI.loaded {
					label := p.Label
					if label == "" {
						label = p.Name
					}
					if label == chosen {
						s.Type = p.Type
						s.Preamp = p.Preamp
						s.Bands = append([]float64(nil), p.Bands...)
						s.Preset = p.Name
						s.AutoEQPath, s.AutoEQName = "", ""
						h.eqChanged()
						h.host.ApplyEqualizer(h.eqSettings)
						return
					}
				}
			}
		})
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if ui.Button(c.Key("eq-save"), "保存").Clicked() {
				h.eqUI.saveName = s.Preset
				if _, ok := eq.Builtin(h.eqUI.saveName); ok {
					h.eqUI.saveName = ""
				}
				h.eqUI.saveErr = ""
				h.eqUI.saveOpen = true
			}
			if h.eqUserPreset(s.Preset) {
				if ui.Button(c.Key("eq-delete"), "删除").Clicked() {
					h.eqUI.deleteName = s.Preset
					h.eqUI.deleteOpen = true
				}
			}
		})
	})
}

func (h *Home) eqUserPreset(name string) bool {
	if name == "" {
		return false
	}
	for _, p := range h.eqUI.loaded {
		if p.Name == name && !p.Builtin {
			return true
		}
	}
	return false
}

func (h *Home) eqTypeRow(c *ui.Context, pal palette) {
	s := &h.eqSettings
	options := []string{"ISO 15 段", "ISO 10 段"}
	chosen := options[0]
	if s.Type == eq.TypeISO10 {
		chosen = options[1]
	}
	ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "频段").FontSize(11.5).TextColor(pal.muted)
		if ui.Select(c.Key("eq-pop-type"), &chosen, options).Label("均衡器频段").Width(130).Changed() {
			typ := eq.TypeISO15
			if chosen == options[1] {
				typ = eq.TypeISO10
			}
			if typ != s.Type {
				s.Bands = eq.Interpolate(s.Bands, s.Type, typ)
				s.Type = typ
				s.Preset = ""
				s.AutoEQPath, s.AutoEQName = "", ""
				h.eqChanged()
				h.host.ApplyEqualizer(h.eqSettings)
			}
		}
	})
}

func (h *Home) eqPreampRow(c *ui.Context, pal palette) {
	s := &h.eqSettings
	ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "前置增益").FontSize(11.5).TextColor(pal.muted)
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			sl := ui.Slider(c.Key("eq-pop-preamp"), &s.Preamp, -12, 12).Width(160)
			if sl.Changed() {
				s.Preset = ""
				s.AutoEQPath, s.AutoEQName = "", ""
				h.eqChanged()
				h.host.ApplyEqualizer(h.eqSettings)
			}
			ui.Text(c, fmt.Sprintf("%+.1f dB", s.Preamp)).FontSize(11).TextColor(pal.muted).Width(52).TextAlign(ui.End).SingleLine()
		})
	})
}

func (h *Home) eqSliders(c *ui.Context, pal palette) {
	s := &h.eqSettings
	labels := eq.Labels(s.Type)
	ui.Row(c).Gap(3).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		for i, label := range labels {
			if i >= len(s.Bands) {
				break
			}
			h.drawEqSlider(c, fmt.Sprintf("eq-pop-band-%d", i), &s.Bands[i], label)
		}
	})
}

func (h *Home) eqAutoEQRow(c *ui.Context, pal palette) {
	s := &h.eqSettings
	ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			ui.Text(c, "耳机校正").FontSize(11.5).TextColor(pal.muted)
			if s.AutoEQName != "" {
				ui.Text(c, s.AutoEQName).FontSize(11).TextColor(pal.text).SingleLine().Shrink(1).MinWidth(0)
			}
		})
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if s.AutoEQName != "" {
				if ui.Button(c.Key("eq-autoeq-clear"), "清除").Clicked() {
					s.Bands = make([]float64, eq.BandCount(s.Type))
					s.Preamp = 0
					s.Preset = ""
					s.AutoEQPath, s.AutoEQName = "", ""
					h.eqChanged()
					h.host.ApplyEqualizer(h.eqSettings)
				}
			}
			if ui.Button(c.Key("eq-autoeq"), "AutoEQ…").Clicked() {
				h.eqUI.autoeqOpen = true
				h.loadAutoEQIndex()
			}
		})
	})
}

func (h *Home) eqSaveDialog(c *ui.Context) {
	d := &h.eqUI
	ui.DialogBase(c.Key("eq-save-dialog"), &d.saveOpen, func(backdrop, panel ui.Element) {
		p := h.pal
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(360).Padding(24).Radius(18).Gap(14).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4))
		ui.Text(c, "保存均衡器预设").FontSize(17).FontWeight(700)
		in := ui.TextInput(c.Key("eq-preset-name"), &d.saveName).Placeholder("预设名称").Label("预设名称").FontSize(14).AutoFocus()
		if in.Submitted() {
			h.saveEQPreset()
		}
		if d.saveErr != "" {
			ui.Text(c, d.saveErr).FontSize(11.5).TextColor(p.accent).LineHeight(1.4)
		}
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if ui.Button(c.Key("eq-save-cancel"), "取消").Clicked() {
				d.saveOpen = false
			}
			ok := ui.Button(c.Key("eq-save-ok"), "保存")
			if strings.TrimSpace(d.saveName) == "" {
				ok.Disabled(true).Opacity(0.5)
			}
			if ok.Clicked() {
				h.saveEQPreset()
			}
		})
	})
}

func (h *Home) saveEQPreset() {
	d := &h.eqUI
	name := strings.TrimSpace(d.saveName)
	if name == "" {
		d.saveErr = "请输入预设名称"
		return
	}
	if name == "自定义" {
		d.saveErr = "这个名称被占用了"
		return
	}
	for _, p := range eq.Builtins() {
		if p.Label == name {
			d.saveErr = "这个名称被内置预设占用了"
			return
		}
	}
	store, _ := h.eqManagers()
	s := &h.eqSettings
	err := store.Save(eq.Preset{
		Name:   name,
		Type:   s.Type,
		Preamp: s.Preamp,
		Bands:  append([]float64(nil), s.Bands...),
	})
	if err != nil {
		d.saveErr = err.Error()
		return
	}
	s.Preset = name
	s.AutoEQPath, s.AutoEQName = "", ""
	h.loadEQPresets()
	d.saveOpen, d.saveErr = false, ""
	h.eqChanged()
	h.host.ApplyEqualizer(h.eqSettings)
	h.host.Toast("已保存均衡器预设")
}

func (h *Home) eqDeleteDialog(c *ui.Context) {
	d := &h.eqUI
	ui.DialogBase(c.Key("eq-delete-dialog"), &d.deleteOpen, func(backdrop, panel ui.Element) {
		p := h.pal
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(360).Padding(24).Radius(18).Gap(10).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4))
		ui.Text(c, "删除均衡器预设").FontSize(17).FontWeight(700)
		ui.Text(c, "确定删除预设“"+d.deleteName+"”吗？删除后无法恢复。").FontSize(13).TextColor(p.muted).LineHeight(1.45)
		ui.Row(c).Gap(10).Justify(ui.End).Margin(8, 0, 0, 0).Children(func() {
			if ui.Button(c.Key("eq-delete-cancel"), "取消").Clicked() {
				d.deleteOpen = false
			}
			if ui.Button(c.Key("eq-delete-ok"), "删除").Clicked() {
				store, _ := h.eqManagers()
				if err := store.Delete(d.deleteName); err != nil {
					h.host.Toast(err.Error())
				} else {
					h.eqSettings.Preset = ""
					h.loadEQPresets()
					h.eqChanged()
					h.host.ApplyEqualizer(h.eqSettings)
					h.host.Toast("已删除均衡器预设")
				}
				d.deleteOpen = false
			}
		})
	})
}

func (h *Home) loadAutoEQIndex() {
	d := &h.eqUI
	if d.autoeqLoading || len(d.autoeqMetas) > 0 {
		return
	}
	_, auto := h.eqManagers()
	d.autoeqLoading, d.autoeqErr = true, ""
	h.eqChanged()
	ctx := h.scope
	go func() {
		metas, err := auto.Index(ctx)
		h.postResult(ctx, func() {
			d.autoeqLoading = false
			if err != nil {
				d.autoeqErr = err.Error()
			} else {
				d.autoeqMetas = metas
			}
			h.eqChanged()
		})
	}()
}

func (h *Home) eqAutoEQDialog(c *ui.Context) {
	d := &h.eqUI
	ui.DialogBase(c.Key("eq-autoeq-dialog"), &d.autoeqOpen, func(backdrop, panel ui.Element) {
		p := h.pal
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(520).Padding(22).Radius(18).Gap(12).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4))
		ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "AutoEQ 耳机校正").FontSize(17).FontWeight(700)
			if ui.Button(c.Key("eq-autoeq-close"), "关闭").Clicked() {
				d.autoeqOpen = false
			}
		})
		ui.Text(c, "选择你的耳机型号，套用对应的校正曲线。第一次打开需要联网载入列表。").FontSize(11.5).TextColor(p.muted).LineHeight(1.4)
		ui.TextInput(c.Key("eq-autoeq-search"), &d.autoeqQuery).Placeholder("搜索耳机型号…").Label("搜索耳机型号").FontSize(14)

		switch {
		case d.autoeqLoading:
			ui.Text(c, "正在载入耳机列表…").FontSize(12).TextColor(p.muted)
		case d.autoeqErr != "":
			ui.Text(c, d.autoeqErr).FontSize(12).TextColor(p.accent).LineHeight(1.4)
			if ui.Button(c.Key("eq-autoeq-retry"), "重试").Clicked() {
				d.autoeqLoading, d.autoeqErr = false, ""
				h.loadAutoEQIndex()
			}
		default:
			query := strings.ToLower(strings.TrimSpace(d.autoeqQuery))
			var shown []eq.Meta
			for _, m := range d.autoeqMetas {
				if query == "" || strings.Contains(strings.ToLower(m.Name+" "+m.Source+" "+m.Type), query) {
					shown = append(shown, m)
				}
			}
			capped := false
			if len(shown) > 200 {
				shown, capped = shown[:200], true
			}
			if len(shown) == 0 {
				ui.Text(c, "没有匹配的耳机型号").FontSize(12).TextColor(p.muted)
			}
			if capped {
				ui.Text(c, "只显示前 200 个结果，输入关键词缩小范围").FontSize(11).TextColor(p.muted)
			}
			ui.Scroll(c.Key("eq-autoeq-list")).Width(470).Height(340).Children(func() {
				ui.Column(c).Gap(2).Children(func() {
					for _, m := range shown {
						row := ui.ButtonBase(c.Key("eq-autoeq-" + m.Path)).Width(450).Height(42).PaddingX(10).Gap(8).
							AlignItems(ui.Center).Radius(8).Label(m.Name)
						row.Children(func() {
							ui.Column(c).Grow(1).Gap(1).Children(func() {
								ui.Text(c, m.Name).FontSize(13).FontWeight(600).SingleLine()
								ui.Text(c, strings.TrimSpace(m.Source+" · "+m.Type)).FontSize(10.5).TextColor(p.muted).SingleLine()
							})
							if d.autoeqBusy == m.Path {
								ui.Text(c, "载入中…").FontSize(11).TextColor(p.accent)
							}
						})
						if row.Clicked() && d.autoeqBusy == "" {
							h.applyAutoEQ(m)
						}
					}
				})
			})
		}
	})
}

func (h *Home) applyAutoEQ(m eq.Meta) {
	d := &h.eqUI
	_, auto := h.eqManagers()
	d.autoeqBusy, d.autoeqErr = m.Path, ""
	h.eqChanged()
	ctx := h.scope
	go func() {
		profile, err := auto.Profile(ctx, m)
		h.postResult(ctx, func() {
			d.autoeqBusy = ""
			if err != nil {
				d.autoeqErr = err.Error()
				h.eqChanged()
				return
			}
			s := &h.eqSettings
			bands := append([]float64(nil), profile.Bands...)
			if s.Type == eq.TypeISO15 {
				bands = eq.Interpolate(profile.Bands, eq.TypeISO10, eq.TypeISO15)
			}
			s.Bands = bands
			s.Preamp = profile.Preamp
			s.Preset = ""
			s.AutoEQPath = profile.Path
			s.AutoEQName = profile.Name
			d.autoeqOpen = false
			h.eqChanged()
			h.host.ApplyEqualizer(h.eqSettings)
			h.host.Toast("已应用耳机校正：" + profile.Name)
		})
	}()
}

func (h *Home) drawEqSlider(c *ui.Context, key string, val *float64, label string) {
	const knob, track = 16, 4
	ui.Column(c).AlignItems(ui.Center).Gap(3).Children(func() {
		ui.Text(c, fmt.Sprintf("%+.0f", *val)).FontSize(10).TextColor(c.Theme().TextMuted).SingleLine()
		s := ui.SliderBase(c.Key(key), val, -12, 12).Vertical().Size(knob+8, 84).PaddingY(knob / 2).FocusRing(false).Label(label)
		if s.Changed() {
			h.eqSettings.Preset = ""
			h.eqSettings.AutoEQPath, h.eqSettings.AutoEQName = "", ""
			h.eqChanged()
			h.host.ApplyEqualizer(h.eqSettings)
		}
		v := float32((*val - (-12)) / (12 - (-12)))
		accent := h.pal.accent
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

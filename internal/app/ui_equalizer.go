package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/eq"
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

func (a *app) eqManagers() (*eq.PresetStore, *eq.AutoEQ) {
	if a.eqUI.presets == nil {
		dir, _ := mygo.App.Path(mygo.PathUserData)
		a.eqUI.presets = eq.NewPresetStore(filepath.Join(dir, "eq_presets"))
	}
	if a.eqUI.autoeq == nil {
		dir, _ := mygo.App.Path(mygo.PathCache)
		a.eqUI.autoeq = eq.NewAutoEQ(filepath.Join(dir, "autoeq"), 20*time.Second)
	}
	return a.eqUI.presets, a.eqUI.autoeq
}

func (a *app) loadEQPresets() {
	store, _ := a.eqManagers()
	a.eqUI.loaded = store.List()
}

// eqChanged redraws the windows after a change to the equalizer. The
// settings themselves are saved when the app quits: a slider drag changes
// them many times a second, and each would otherwise write the file.
func (a *app) eqChanged() { a.redraw() }

// eqButton is the graphic equalizer's button and panel, over the player.
// It shows only while MPV plays, where the equalizer runs.
func (a *app) eqButton(c *ui.Context, pal palette) {
	if !a.eqAvailable() {
		a.eqOpen = false
		return
	}
	s := &a.settings
	s.EQ.Normalize()
	if a.eqUI.loaded == nil {
		a.loadEQPresets()
	}

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
			if ui.Switch(c.Key("eq-pop-toggle"), &s.EQ.Enabled).Label("启用均衡器").Changed() {
				a.eqChanged()
				a.syncMPVSettings()
			}
		})

		a.eqPresetRow(c, pal)
		a.eqTypeRow(c, pal)
		a.eqPreampRow(c, pal)
		a.eqSliders(c, pal)
		a.eqAutoEQRow(c, pal)
	})

	a.eqSaveDialog(c)
	a.eqDeleteDialog(c)
	a.eqAutoEQDialog(c)
}

func (a *app) eqPresetRow(c *ui.Context, pal palette) {
	s := &a.settings
	names := make([]string, 0, len(a.eqUI.loaded)+1)
	chosen := "自定义"
	for _, p := range a.eqUI.loaded {
		label := p.Label
		if label == "" {
			label = p.Name
		}
		names = append(names, label)
		if p.Name == s.EQ.Preset {
			chosen = label
		}
	}
	names = append(names, "自定义")

	ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "预设").FontSize(11.5).TextColor(pal.muted)
			if ui.Select(c.Key("eq-pop-preset"), &chosen, names).Label("预设音效").Width(130).Changed() {
				if chosen == "自定义" {
					s.EQ.Preset = ""
					a.eqChanged()
					a.syncMPVSettings()
					return
				}
				for _, p := range a.eqUI.loaded {
					label := p.Label
					if label == "" {
						label = p.Name
					}
					if label == chosen {
						s.EQ.Type = p.Type
						s.EQ.Preamp = p.Preamp
						s.EQ.Bands = append([]float64(nil), p.Bands...)
						s.EQ.Preset = p.Name
						s.EQ.AutoEQPath, s.EQ.AutoEQName = "", ""
						a.eqChanged()
						a.syncMPVSettings()
						return
					}
				}
			}
		})
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if ui.Button(c.Key("eq-save"), "保存").Clicked() {
				a.eqUI.saveName = s.EQ.Preset
				if _, ok := eq.Builtin(a.eqUI.saveName); ok {
					a.eqUI.saveName = ""
				}
				a.eqUI.saveErr = ""
				a.eqUI.saveOpen = true
			}
			if a.eqUserPreset(s.EQ.Preset) {
				if ui.Button(c.Key("eq-delete"), "删除").Clicked() {
					a.eqUI.deleteName = s.EQ.Preset
					a.eqUI.deleteOpen = true
				}
			}
		})
	})
}

func (a *app) eqUserPreset(name string) bool {
	if name == "" {
		return false
	}
	for _, p := range a.eqUI.loaded {
		if p.Name == name && !p.Builtin {
			return true
		}
	}
	return false
}

func (a *app) eqTypeRow(c *ui.Context, pal palette) {
	s := &a.settings
	options := []string{"ISO 15 段", "ISO 10 段"}
	chosen := options[0]
	if s.EQ.Type == eq.TypeISO10 {
		chosen = options[1]
	}
	ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "频段").FontSize(11.5).TextColor(pal.muted)
		if ui.Select(c.Key("eq-pop-type"), &chosen, options).Label("均衡器频段").Width(130).Changed() {
			typ := eq.TypeISO15
			if chosen == options[1] {
				typ = eq.TypeISO10
			}
			if typ != s.EQ.Type {
				s.EQ.Bands = eq.Interpolate(s.EQ.Bands, s.EQ.Type, typ)
				s.EQ.Type = typ
				s.EQ.Preset = ""
				s.EQ.AutoEQPath, s.EQ.AutoEQName = "", ""
				a.eqChanged()
				a.syncMPVSettings()
			}
		}
	})
}

func (a *app) eqPreampRow(c *ui.Context, pal palette) {
	s := &a.settings
	ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "前置增益").FontSize(11.5).TextColor(pal.muted)
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			sl := ui.Slider(c.Key("eq-pop-preamp"), &s.EQ.Preamp, -12, 12).Width(160)
			if sl.Changed() {
				s.EQ.Preset = ""
				s.EQ.AutoEQPath, s.EQ.AutoEQName = "", ""
				a.eqChanged()
				a.syncMPVSettings()
			}
			ui.Text(c, fmt.Sprintf("%+.1f dB", s.EQ.Preamp)).FontSize(11).TextColor(pal.muted).Width(52).TextAlign(ui.End).SingleLine()
		})
	})
}

func (a *app) eqSliders(c *ui.Context, pal palette) {
	s := &a.settings
	labels := eq.Labels(s.EQ.Type)
	ui.Row(c).Gap(3).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		for i, label := range labels {
			if i >= len(s.EQ.Bands) {
				break
			}
			a.drawEqSlider(c, fmt.Sprintf("eq-pop-band-%d", i), &s.EQ.Bands[i], label)
		}
	})
}

func (a *app) eqAutoEQRow(c *ui.Context, pal palette) {
	s := &a.settings
	ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			ui.Text(c, "耳机校正").FontSize(11.5).TextColor(pal.muted)
			if s.EQ.AutoEQName != "" {
				ui.Text(c, s.EQ.AutoEQName).FontSize(11).TextColor(pal.text).SingleLine().Shrink(1).MinWidth(0)
			}
		})
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if s.EQ.AutoEQName != "" {
				if ui.Button(c.Key("eq-autoeq-clear"), "清除").Clicked() {
					s.EQ.Bands = make([]float64, eq.BandCount(s.EQ.Type))
					s.EQ.Preamp = 0
					s.EQ.Preset = ""
					s.EQ.AutoEQPath, s.EQ.AutoEQName = "", ""
					a.eqChanged()
					a.syncMPVSettings()
				}
			}
			if ui.Button(c.Key("eq-autoeq"), "AutoEQ…").Clicked() {
				a.eqUI.autoeqOpen = true
				a.loadAutoEQIndex()
			}
		})
	})
}

func (a *app) eqSaveDialog(c *ui.Context) {
	d := &a.eqUI
	ui.DialogBase(c.Key("eq-save-dialog"), &d.saveOpen, func(backdrop, panel ui.Element) {
		p := a.pal
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(360).Padding(24).Radius(18).Gap(14).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4))
		ui.Text(c, "保存均衡器预设").FontSize(17).FontWeight(700)
		in := ui.TextInput(c.Key("eq-preset-name"), &d.saveName).Placeholder("预设名称").Label("预设名称").FontSize(14).AutoFocus()
		if in.Submitted() {
			a.saveEQPreset()
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
				a.saveEQPreset()
			}
		})
	})
}

func (a *app) saveEQPreset() {
	d := &a.eqUI
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
	store, _ := a.eqManagers()
	s := &a.settings
	err := store.Save(eq.Preset{
		Name:   name,
		Type:   s.EQ.Type,
		Preamp: s.EQ.Preamp,
		Bands:  append([]float64(nil), s.EQ.Bands...),
	})
	if err != nil {
		d.saveErr = err.Error()
		return
	}
	s.EQ.Preset = name
	s.EQ.AutoEQPath, s.EQ.AutoEQName = "", ""
	a.loadEQPresets()
	d.saveOpen, d.saveErr = false, ""
	a.eqChanged()
	a.syncMPVSettings()
	a.toast = "已保存均衡器预设"
}

func (a *app) eqDeleteDialog(c *ui.Context) {
	d := &a.eqUI
	ui.DialogBase(c.Key("eq-delete-dialog"), &d.deleteOpen, func(backdrop, panel ui.Element) {
		p := a.pal
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
				store, _ := a.eqManagers()
				if err := store.Delete(d.deleteName); err != nil {
					a.toast = err.Error()
				} else {
					a.settings.EQ.Preset = ""
					a.loadEQPresets()
					a.eqChanged()
					a.syncMPVSettings()
					a.toast = "已删除均衡器预设"
				}
				d.deleteOpen = false
			}
		})
	})
}

func (a *app) loadAutoEQIndex() {
	d := &a.eqUI
	if d.autoeqLoading || len(d.autoeqMetas) > 0 {
		return
	}
	_, auto := a.eqManagers()
	d.autoeqLoading, d.autoeqErr = true, ""
	a.eqChanged()
	go func() {
		metas, err := auto.Index(context.Background())
		a.post(func() {
			d.autoeqLoading = false
			if err != nil {
				d.autoeqErr = err.Error()
			} else {
				d.autoeqMetas = metas
			}
			a.eqChanged()
		})
	}()
}

func (a *app) eqAutoEQDialog(c *ui.Context) {
	d := &a.eqUI
	ui.DialogBase(c.Key("eq-autoeq-dialog"), &d.autoeqOpen, func(backdrop, panel ui.Element) {
		p := a.pal
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
				a.loadAutoEQIndex()
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
							a.applyAutoEQ(m)
						}
					}
				})
			})
		}
	})
}

func (a *app) applyAutoEQ(m eq.Meta) {
	d := &a.eqUI
	_, auto := a.eqManagers()
	d.autoeqBusy, d.autoeqErr = m.Path, ""
	a.eqChanged()
	go func() {
		profile, err := auto.Profile(context.Background(), m)
		a.post(func() {
			d.autoeqBusy = ""
			if err != nil {
				d.autoeqErr = err.Error()
				a.eqChanged()
				return
			}
			s := &a.settings
			bands := append([]float64(nil), profile.Bands...)
			if s.EQ.Type == eq.TypeISO15 {
				bands = eq.Interpolate(profile.Bands, eq.TypeISO10, eq.TypeISO15)
			}
			s.EQ.Bands = bands
			s.EQ.Preamp = profile.Preamp
			s.EQ.Preset = ""
			s.EQ.AutoEQPath = profile.Path
			s.EQ.AutoEQName = profile.Name
			d.autoeqOpen = false
			a.eqChanged()
			a.syncMPVSettings()
			a.toast = "已应用耳机校正：" + profile.Name
		})
	}()
}

package app

import (
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/waygu-music-plugin/sdk/pluginapi"
)

func (a *app) hasPluginManager() bool {
	return len(a.platformRegistrations) > 1
}

func (a *app) openPluginManager() {
	if a.pluginsWin != nil {
		a.pluginsWin.Show()
		a.pluginsWin.Focus()
		return
	}
	win := mygo.NewWindow(mygo.WindowOptions{
		Title: "插件管理", Width: 810, Height: 650, MinWidth: 510, MinHeight: 500,
		Content: ui.View(a.pluginManagerView),
	})
	a.pluginsWin = win
	win.SetMenu(mygo.NewMenu(nil))
	win.OnClosed(func() {
		a.pluginsWin = nil
		a.pluginSource = pluginSourceEditor{}
	})
	win.Focus()
}

func (a *app) pluginManagerView(c *ui.Context) {
	theme := *c.Theme()
	p := managerColors(theme.Dark)
	theme.Background, theme.Text, theme.TextMuted = p.background, p.ink, p.muted
	theme.Surface, theme.SurfaceHover, theme.SurfacePressed = p.row, p.active, p.border
	theme.Border, theme.Accent, theme.Focus = p.border, p.accent, p.accent
	theme.AccentText = ui.RGB(255, 255, 255)
	if theme.Dark {
		theme.AccentText = ui.RGB(24, 45, 36)
	}
	theme.AccentHover = p.accent.Mix(ui.RGB(255, 255, 255), 0.10)
	theme.AccentPressed = p.accent.Mix(ui.RGB(0, 0, 0), 0.12)
	theme.Selection = p.accent.Alpha(0.26)
	c.SetTheme(&theme)
	width, _ := c.Size()
	ui.Scroll(c).Fill().Padding(22).Gap(18).Children(func() {
		a.pluginManagerHeader(c)
		packages := a.catalogRepresentatives()
		if len(packages) == 0 || width < 720 {
			a.pluginPlatformList(c, p)
			if len(packages) != 0 {
				a.pluginCatalogView(c, p, packages, false)
			}
			return
		}
		ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(18).Children(func() {
			a.pluginPlatformList(c, p)
			a.pluginCatalogView(c, p, packages, true)
		})
	})
	a.pluginSourceDialog(c, p, width)
}

type pluginManagerColors struct {
	background, panel, border, row, active, accent, muted, ink ui.Color
}

func managerColors(dark bool) pluginManagerColors {
	if dark {
		return pluginManagerColors{
			background: ui.RGB(17, 24, 24), panel: ui.RGB(24, 34, 34), border: ui.RGB(46, 61, 58),
			row: ui.RGB(28, 39, 38), active: ui.RGB(34, 58, 48), accent: ui.RGB(141, 219, 175),
			muted: ui.RGB(163, 180, 172), ink: ui.RGB(232, 243, 237),
		}
	}
	return pluginManagerColors{
		background: ui.RGB(240, 245, 242), panel: ui.RGB(255, 255, 255), border: ui.RGB(214, 227, 219),
		row: ui.RGB(247, 250, 248), active: ui.RGB(228, 245, 235), accent: ui.RGB(29, 109, 75),
		muted: ui.RGB(82, 104, 95), ink: ui.RGB(24, 45, 36),
	}
}

func (a *app) pluginManagerHeader(c *ui.Context) {
	var current string
	for _, entry := range a.platformRegistrations {
		if a.platformID == entry.ID {
			current = entry.Info.Name
			break
		}
	}
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(16).Padding(18, 22).Radius(16).
		Gradient(ui.RGB(24, 48, 43), ui.RGB(41, 89, 69), 0).Children(func() {
		ui.Column(c).Grow(1).Gap(4).Children(func() {
			ui.Text(c, "插件管理").FontSize(25).FontWeight(700).TextColor(ui.RGB(243, 251, 246))
			ui.Text(c, "切换音乐平台，管理已安装的插件包").FontSize(12).TextColor(ui.RGB(192, 222, 207))
		})
		if current != "" {
			ui.Column(c).Gap(4).Padding(8, 13).Radius(9).
				Background(ui.RGB(255, 255, 255).Alpha(0.10)).Children(func() {
				ui.Text(c, "当前平台").FontSize(11).TextColor(ui.RGB(194, 227, 209))
				ui.Text(c, current).FontSize(14).FontWeight(650).TextColor(ui.RGB(245, 253, 248))
			})
		}
	})
}

func (a *app) pluginPlatformList(c *ui.Context, p pluginManagerColors) {
	ui.Column(c).Grow(1).MinWidth(0).Background(p.panel).Border(1, p.border).Radius(14).Padding(13).Gap(6).Children(func() {
		ui.Text(c, "音乐平台").FontSize(16).FontWeight(700).TextColor(p.ink).Padding(2, 3, 4, 3)
		for _, registration := range a.platformRegistrations {
			if a.hasPlatform(registration.ID) {
				a.pluginPlatformRow(c.Key(string(registration.ID)), p, registration.Info)
			}
		}
	})
}

func (a *app) pluginPlatformRow(c *ui.Context, p pluginManagerColors, entry pluginapi.Info) {
	local := a.localPlugins[entry.ID]
	active := a.platformID == entry.ID
	background, border := p.row, p.border.Alpha(0)
	if active {
		background, border = p.active, p.accent.Alpha(0.36)
	}
	ui.Column(c).FillWidth().Gap(6).Padding(5, 9).Radius(9).Background(background).Border(1, border).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			mark, color := pluginPlatformMark(entry)
			ui.Row(c).Size(32, 32).Shrink(0).Radius(8).AlignItems(ui.Center).Justify(ui.Center).Background(color).Children(func() {
				ui.Text(c, mark).FontSize(14).FontWeight(700).TextColor(ui.RGB(255, 255, 255))
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, entry.Name).FontSize(13).FontWeight(650).TextColor(p.ink)
				detail := "已安装"
				switch {
				case local == nil:
					detail = "内置"
				case local.version != "":
					detail = local.program.Identity.Package + " · " + local.version
				case local.program.Path == "":
					detail = local.program.Identity.Package
				}
				ui.Text(c, detail).FontSize(11).TextColor(p.muted)
			})
			switch {
			case active:
				ui.Text(c, "●  使用中").FontSize(11).FontWeight(650).TextColor(p.accent)
			case local != nil && local.pending != nil:
				ui.Text(c, "切换中…").FontSize(11).TextColor(p.accent)
			case local != nil && local.program.Path == "":
				ui.Text(c, "未安装").FontSize(11).TextColor(p.muted)
			default:
				if ui.Button(c, "切换").Label("使用 " + entry.Name).Height(30).Width(58).Radius(7).Clicked() {
					a.setPlatform(entry.ID)
				}
			}
		})
		if local != nil && local.startError != "" {
			ui.Text(c, local.startError).FontSize(12).TextColor(c.Theme().Danger).Wrap()
		}
	})
}

func pluginPlatformMark(entry pluginapi.Info) (string, ui.Color) {
	switch entry.ID {
	case "fnmusic":
		return "飞", ui.RGB(40, 108, 90)
	case "netease":
		return "云", ui.RGB(188, 45, 59)
	case "qqmusic":
		return "Q", ui.RGB(22, 127, 89)
	case "navidrome":
		return "N", ui.RGB(46, 100, 158)
	case "jellyfin":
		return "J", ui.RGB(99, 75, 167)
	case "gonic":
		return "G", ui.RGB(151, 74, 123)
	case "songloft":
		return "S", ui.RGB(162, 86, 41)
	case "subsonic":
		return "S", ui.RGB(54, 106, 113)
	default:
		initial, _ := utf8.DecodeRuneInString(entry.Name)
		return strings.ToUpper(string(initial)), ui.RGB(78, 107, 100)
	}
}

func (a *app) pluginCatalogView(c *ui.Context, p pluginManagerColors, packages []*localPlugin, sideBySide bool) {
	selected := packages[0]
	for _, local := range packages {
		if local.program.Identity.Package == a.catalogPackage {
			selected = local
			break
		}
	}
	card := ui.Column(c).Background(p.panel).Border(1, p.border).Radius(14).Padding(16).Gap(13)
	if sideBySide {
		card.Width(310).Shrink(0)
	} else {
		card.FillWidth()
	}
	card.Children(func() {
		ui.Text(c, "插件包").FontSize(16).FontWeight(700).TextColor(p.ink)
		ui.Column(c).Gap(4).Children(func() {
			for _, local := range packages {
				id := local.program.Identity.Package
				item := ui.ButtonBase(c.Key(id)).FillWidth().AlignItems(ui.Center).Gap(8).Padding(8, 10).Radius(7).
					Border(1, ui.Transparent).Label("管理 " + pluginPackageName(local))
				if local == selected {
					item.Background(p.active).Border(1, p.accent.Alpha(0.34))
				} else if item.Hovered() {
					item.Background(p.row)
				}
				item.Children(func() {
					ui.Text(c, pluginPackageName(local)).FontSize(13).FontWeight(600)
					ui.Spacer(c)
					if local.version != "" {
						ui.Text(c, local.version).FontSize(11).TextColor(p.accent)
					} else {
						ui.Text(c, "未安装").FontSize(11).TextColor(p.muted)
					}
				})
				if item.Clicked() {
					a.catalogPackage = id
				}
			}
		})
		a.pluginPackageActions(c.Key(selected.program.Identity.Package), p, selected)
	})
}

func pluginPackageName(local *localPlugin) string {
	if local.program.Identity.Package == "subsonic" {
		return "Subsonic 兼容平台"
	}
	return local.entry.Info.Name
}

func (a *app) pluginPackageActions(c *ui.Context, p pluginManagerColors, local *localPlugin) {
	disabled := local.busy || local.pending != nil || local.registry == nil
	configured := local.catalog != ""
	ui.Box(c).FillWidth().Height(1).Background(p.border)
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		source := "未配置来源"
		if u, err := url.Parse(local.catalog); configured && err == nil && u.Host != "" {
			source = u.Host
		}
		ui.Text(c, source).Grow(1).MinWidth(0).SingleLine().FontSize(12).TextColor(p.muted)
		if ui.Button(c, "来源设置…").Disabled(disabled).Clicked() {
			a.pluginSource = pluginSourceEditor{open: true, local: local, draft: local.catalog}
		}
	})
	ui.Row(c).FillWidth().Gap(7).Children(func() {
		if ui.PrimaryButton(c, "安装或更新").Grow(1).Disabled(disabled || !configured).Clicked() {
			a.changeInstalledPlugin(local, "install")
		}
		if local.version != "" {
			if ui.Button(c, "回退").Disabled(disabled).Clicked() {
				a.changeInstalledPlugin(local, "rollback")
			}
			if ui.Button(c, "卸载").Disabled(disabled).Clicked() {
				a.changeInstalledPlugin(local, "uninstall")
			}
		}
	})
	if local.message != "" {
		ui.Text(c, local.message).FontSize(12).TextColor(p.muted).Wrap()
	}
}

type pluginSourceEditor struct {
	open    bool
	local   *localPlugin
	draft   string
	message string
}

func (a *app) pluginSourceDialog(c *ui.Context, p pluginManagerColors, width float32) {
	editor := &a.pluginSource
	ui.DialogBase(c.Key("plugin-source"), &editor.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.36))
		panel.Width(min(450, width-40)).Padding(22).Radius(14).Gap(14).Background(p.panel).
			Border(1, p.border).Shadow(0, 14, 36, -6, ui.RGBA(0, 0, 0, 0.25))
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, "插件来源").FontSize(19).FontWeight(700)
			ui.Text(c, pluginPackageName(editor.local)).FontSize(12).TextColor(p.muted)
		})
		ui.Column(c).Gap(5).Children(func() {
			ui.Text(c, "插件目录 URL").FontSize(12).FontWeight(600)
			ui.TextInput(c.Key("catalog"), &editor.draft).Label("插件目录 URL").Placeholder("https://…/catalog.json").AutoFocus()
		})
		if editor.message != "" {
			ui.Text(c, editor.message).FontSize(12).TextColor(c.Theme().Danger).Wrap()
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if editor.local.version == "" && editor.local.catalog != "" {
				if ui.Button(c, "移除来源").Clicked() {
					a.commitPluginSource("")
				}
			}
			ui.Spacer(c)
			if ui.Button(c, "取消").Clicked() {
				editor.open = false
			}
			if ui.PrimaryButton(c, "保存来源").Clicked() {
				if catalog, err := checkedPluginCatalog(editor.draft); err != nil {
					editor.message = err.Error()
				} else {
					a.commitPluginSource(catalog)
				}
			}
		})
	})
}

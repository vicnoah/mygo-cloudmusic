package app

import (
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) hasPluginManager() bool {
	return a.managedPlugins != nil || len(a.platformRegistrations) > 1
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
	_ = win.SetIcon(desktopApplicationIcon())
	a.syncDock()
	a.refreshPluginCatalog()
	win.SetMenu(mygo.NewMenu(nil))
	win.OnClosed(func() {
		a.pluginsWin = nil
		a.pluginSource = pluginSourceEditor{}
		a.syncDock()
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
	ui.Scroll(c).Fill().Padding(22).Gap(16).Children(func() {
		a.pluginManagerHeader(c, p)
		ui.Column(c).FillWidth().Background(p.panel).Border(1, p.border).Radius(14).Padding(10).Gap(5).Children(func() {
			for _, entry := range a.platformRegistrations {
				if !a.hasPlatform(entry.ID) {
					continue
				}
				row := c.Key(string(entry.ID))
				a.pluginManagerRow(row, p, entry.Name, "内置", a.platformID == entry.ID, false, true, func() {
					a.setPlatform(entry.ID)
				}, nil)
			}
			var packages []*managedPlugin
			for _, pkg := range a.managedPlugins {
				packages = append(packages, pkg)
			}
			slices.SortFunc(packages, func(a, b *managedPlugin) int {
				return strings.Compare(a.id, b.id)
			})
			for _, pkg := range packages {
				a.managedPluginRow(c.Key("package:"+pkg.id), p, pkg)
			}
		})
		if a.pluginCatalogBusy {
			ui.Text(c, "正在读取插件目录…").FontSize(12).TextColor(p.muted)
		}
		if a.pluginCatalogError != "" {
			ui.Text(c, a.pluginCatalogError).FontSize(12).TextColor(c.Theme().Danger).Wrap()
		}
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

func (a *app) pluginManagerHeader(c *ui.Context, p pluginManagerColors) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, "插件管理").Grow(1).FontSize(24).FontWeight(700).TextColor(p.ink)
		if a.managedPlugins != nil {
			if ui.Button(c, "刷新").Disabled(a.pluginCatalogBusy || a.pluginRegistry == nil).Clicked() {
				a.refreshPluginCatalog()
			}
			if ui.Button(c, "来源设置…").Clicked() {
				a.pluginSource = pluginSourceEditor{
					open: true, draft: a.settings.PluginCatalogURL, useGitHubProxy: !a.settings.DisablePluginGitHubProxy,
				}
			}
		}
	})
}

func (a *app) managedPluginRow(c *ui.Context, p pluginManagerColors, pkg *managedPlugin) {
	name := pkg.name
	if name == "" {
		name = pkg.id
	}
	id := a.preferredPluginEntry(pkg)
	active := false
	if current := a.localPlugins[a.platformID]; current != nil {
		active = current.program.Identity.Package == pkg.id
	}
	var pending bool
	var message = pkg.message
	for _, entry := range pkg.installed.Platforms {
		if local := a.localPlugins[externalRegistration(pkg.id, entry).ID]; local != nil {
			pending = pending || local.pending != nil
			if local.entry.ID == id && local.startError != "" {
				message = local.startError
			}
		}
	}
	installed := pkg.installed.Path != ""
	detail := pkg.installed.Version.Version
	if !installed {
		detail = "未安装"
	}
	primary := func() {
		a.catalogPackage = pkg.id
		if installed {
			a.setPlatform(id)
		} else {
			a.changeInstalledPlugin(pkg, "install")
		}
	}
	var manage func()
	if installed {
		manage = func() {
			if a.catalogPackage == pkg.id {
				a.catalogPackage = ""
			} else {
				a.catalogPackage = pkg.id
			}
		}
	}
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		a.pluginManagerRow(c, p, name, detail, active, pkg.busy || pending, installed, primary, manage)
		if a.catalogPackage != pkg.id {
			return
		}
		if installed {
			ui.Row(c).Padding(4, 10, 6, 10).Gap(8).Children(func() {
				disabled := pkg.busy || pending
				if ui.Button(c, "更新").Disabled(disabled || pkg.source == "").Clicked() {
					a.changeInstalledPlugin(pkg, "install")
				}
				if pkg.installed.CanRollback && ui.Button(c, "回退").Disabled(disabled).Clicked() {
					a.changeInstalledPlugin(pkg, "rollback")
				}
				if ui.Button(c, "卸载").Disabled(disabled).Clicked() {
					a.changeInstalledPlugin(pkg, "uninstall")
				}
			})
		}
		if message != "" {
			ui.Text(c, message).Padding(4, 10, 6, 10).FontSize(12).TextColor(p.muted).Wrap()
		}
	})
}

func (a *app) pluginManagerRow(c *ui.Context, p pluginManagerColors, name, detail string, active, busy, installed bool, primary, manage func()) {
	background := p.row
	if active {
		background = p.active
	}
	ui.Row(c).FillWidth().Padding(8, 10).Radius(8).Background(background).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, name).FontSize(14).FontWeight(650).TextColor(p.ink).SingleLine()
			ui.Text(c, detail).FontSize(11).TextColor(p.muted)
		})
		switch {
		case busy:
			ui.Text(c, "处理中…").FontSize(12).TextColor(p.muted)
		case active:
			ui.Text(c, "使用中").FontSize(12).FontWeight(650).TextColor(p.accent)
		case installed:
			if ui.Button(c, "启用").Label("启用 " + name).Clicked() {
				primary()
			}
		default:
			if ui.PrimaryButton(c, "安装").Label("安装 " + name).Clicked() {
				primary()
			}
		}
		if manage != nil && ui.Button(c, "管理…").Label("管理 "+name).Clicked() {
			manage()
		}
	})
}

type pluginSourceEditor struct {
	open           bool
	draft          string
	useGitHubProxy bool
	message        string
}

func (a *app) pluginSourceDialog(c *ui.Context, p pluginManagerColors, width float32) {
	editor := &a.pluginSource
	ui.DialogBase(c.Key("plugin-source"), &editor.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.36))
		panel.Width(min(450, width-40)).Padding(22).Radius(14).Gap(14).Background(p.panel).
			Border(1, p.border).Shadow(0, 14, 36, -6, ui.RGBA(0, 0, 0, 0.25))
		ui.Text(c, "插件来源").FontSize(19).FontWeight(700)
		ui.Column(c).Gap(5).Children(func() {
			ui.Text(c, "额外插件目录 URL").FontSize(12).FontWeight(600)
			ui.TextInput(c.Key("catalog"), &editor.draft).Label("额外插件目录 URL").Placeholder("https://…/catalog.json").AutoFocus()
		})
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Padding(10, 12).Radius(8).Background(p.row).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, "GitHub 下载加速").FontSize(13).FontWeight(600)
				ui.Text(c, "通过 gh-proxy.com 获取插件的 GitHub 目录、清单和安装包。").FontSize(11).TextColor(p.muted).Wrap()
			})
			ui.Switch(c.Key("github-proxy"), &editor.useGitHubProxy).Shrink(0).Label("启用 GitHub 下载加速")
		})
		if editor.message != "" {
			ui.Text(c, editor.message).FontSize(12).TextColor(c.Theme().Danger).Wrap()
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spacer(c)
			if ui.Button(c, "取消").Clicked() {
				editor.open = false
			}
			if ui.PrimaryButton(c, "保存").Clicked() {
				if strings.TrimSpace(editor.draft) == "" {
					a.commitPluginSource("")
				} else if catalog, err := checkedPluginCatalog(editor.draft); err != nil {
					editor.message = err.Error()
				} else {
					a.commitPluginSource(catalog)
				}
			}
		})
	})
}

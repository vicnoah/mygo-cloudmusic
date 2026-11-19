package netease

import (
	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/plugins/netease/internal/client"
)

// platformSwitcher offers a way to select another platform before signing in.
func (a *app) platformSwitcher(c *ui.Context) {
	if a.profile != nil || netease.SessionConfigured() {
		return
	}
	infos := a.host.PlatformOptions()
	if len(infos) < 2 {
		return
	}
	p := a.pal
	current := a.host.CurrentPlatform()
	b := ui.ButtonBase(c.Key("platform-switcher")).Height(40).PaddingX(8).Gap(10).
		Radius(10).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label("切换音乐平台")
	if b.Hovered() || a.platformMenuOpen {
		b.Background(p.hover)
	}
	b.Children(func() {
		ui.Box(c).Size(26, 26).Radius(13).Background(p.press).Center().Children(func() {
			ui.Icon(c, iconNote).Size(14, 14).TextColor(p.muted)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, "切换音乐平台").FontSize(13).FontWeight(600).SingleLine()
			ui.Text(c, platformName(infos, current)).FontSize(11.5).TextColor(p.faint).SingleLine()
		})
		ui.Icon(c, iconUpDown).Size(13, 13).TextColor(p.faint)
	})
	if b.Clicked() {
		a.platformMenuOpen = !a.platformMenuOpen
	}
	pop := popoverColors(a.pal.dark)
	ui.PopoverBase(c.Key("platform-switcher-menu"), b, &a.platformMenuOpen, func(panel ui.Element) {
		panel.AttachTo(b, ui.AnchorTopStart, ui.AnchorBottomStart).Margin(0, 0, 6, 0).Padding(5).Gap(1).
			Radius(12).Background(pop.fill).Border(0.5, pop.edge).Shadow(0, 8, 24, -4, ui.RGBA(0, 0, 0, 0.2)).
			Role(ui.RoleMenu).FocusGroup(ui.Vertical).Label("音乐平台").TransformOrigin(0.5, 1)
		if w := b.Bounds().W; w > 0 {
			panel.Width(w)
		}
		for _, info := range infos {
			info := info
			icon := info.Icon
			if icon == nil {
				icon = iconNote
			}
			item := a.menuItem(c, icon, info.Name, "")
			if info.ID == current {
				item = item.TextColor(p.accent)
			}
			if item.Clicked() {
				a.platformMenuOpen = false
				a.host.SwitchPlatform(info.ID)
				if !a.host.PlatformConfigured(info.ID) {
					a.openSettings()
				}
			}
		}
	})
}

func platformName(infos []pluginapi.Info, id pluginapi.ID) string {
	for _, info := range infos {
		if info.ID == id {
			return info.Name
		}
	}
	return "网易云音乐"
}

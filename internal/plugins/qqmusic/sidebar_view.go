package qqmusic

import (
	"time"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
)

func (h *Home) navItem(c *ui.Context, icon *ui.SVG, label string, r Route) {
	p := paletteFor(c.Theme())
	selected := h.route == r && !h.nowPlaying
	b := ui.ButtonBase(c.Key("platform-" + label)).Height(34).PaddingX(10).Gap(10).Radius(8).
		AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(label)
	bg := ui.Transparent
	if selected {
		bg = p.press
	} else if b.Hovered() {
		bg = p.hover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	color := p.text
	if selected {
		color = p.accent
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(17, 17).TextColor(color)
		ui.Text(c, label).FontSize(13.5).FontWeight(500).TextColor(p.text)
	})
	if b.Clicked() {
		h.navigate(r)
	}
}

func playlistHeader(c *ui.Context, create func()) {
	p := paletteFor(c.Theme())
	ui.Row(c).AlignItems(ui.Center).Margin(18, 4, 6, 10).Children(func() {
		ui.Text(c, "歌单").FontSize(11.5).FontWeight(600).TextColor(p.faint).Grow(1)
		if create != nil && iconButton(c.Key("platform-new-playlist"), iconPlus, 13, p.faint, "新建歌单").Clicked() {
			create()
		}
	})
}

func (h *Home) playlistItem(c *ui.Context, key string, pl pluginapi.Playlist, selected bool, menu func(*ui.Menu), click func()) {
	p := paletteFor(c.Theme())
	b := ui.ButtonBase(c.Key(key)).Height(34).PaddingX(8).Gap(10).Radius(8).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(pl.Name)
	if selected {
		b.Background(p.press)
	} else if b.Hovered() {
		b.Background(p.hover)
	}
	b.Children(func() {
		h.cover(c, pl.Cover(), 80, 22, 5)
		ui.Text(c, pl.Name).FontSize(13).SingleLine().Shrink(1).MinWidth(0)
	})
	if menu != nil {
		b.ContextMenu(menu)
	}
	if b.Clicked() && click != nil {
		click()
	}
}

func (h *Home) accountRow(c *ui.Context, avatar, username, status string, actions []AccountAction) {
	p := paletteFor(c.Theme())
	b := ui.ButtonBase(c.Key("platform-account")).Height(40).PaddingX(8).Gap(10).Radius(10).
		AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(username)
	if b.Hovered() || h.accountOpen {
		b.Background(p.hover)
	}
	b.Expanded(h.accountOpen).Children(func() {
		if avatar != "" {
			h.avatar(c, avatar, 26, p.press)
		} else {
			ui.Box(c).Size(26, 26).Radius(13).Background(p.press).Center().Children(func() {
				ui.Icon(c, iconUser).Size(14, 14).TextColor(p.muted)
			})
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, username).FontSize(13).FontWeight(600).SingleLine().Shrink(1).MinWidth(0)
			if status != "" {
				ui.Text(c, status).FontSize(11.5).TextColor(p.faint).SingleLine().Shrink(1).MinWidth(0)
			}
		})
		ui.Icon(c, iconUpDown).Size(13, 13).TextColor(p.faint)
	})
	if b.Clicked() {
		h.accountOpen = !h.accountOpen
	}
	pop := popoverColors(p.dark)
	ui.PopoverBase(c.Key("platform-account-menu"), b, &h.accountOpen, func(panel ui.Element) {
		panel.AttachTo(b, ui.AnchorTopStart, ui.AnchorBottomStart).Margin(0, 0, 6, 0).Padding(5).Gap(1).
			Radius(12).Background(pop.fill).Border(0.5, pop.edge).Shadow(0, 8, 24, -4, ui.RGBA(0, 0, 0, 0.2)).
			Role(ui.RoleMenu).FocusGroup(ui.Vertical).Label("账户").TransformOrigin(0.5, 1).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.96, Y: 4}, Exit: &ui.Motion{Scale: 0.98}})
		if w := b.Bounds().W; w > 0 {
			panel.Width(w)
		}
		for i, action := range actions {
			if action.Separator && i > 0 {
				ui.Box(c).Height(0.5).Margin(4, 8).Background(pop.edge)
			}
			item := menuItem(c, action.Icon, action.Label, action.Shortcut)
			if action.Danger {
				item.TextColor(p.accent)
			}
			if item.Clicked() {
				h.accountOpen = false
				if action.Action != nil {
					action.Action()
				}
			}
		}
	})
}

func (h *Home) sidebarNowPlaying(c *ui.Context, tr *pluginapi.Track) {
	p := paletteFor(c.Theme())
	card := ui.ButtonBase(c.Key("np-card")).Gap(10).Padding(8).Radius(12).AlignItems(ui.Center).FocusRing(false).Label("正在播放")
	bg := p.hover
	if card.Hovered() {
		bg = p.press
	}
	card.Background(card.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	card.Children(func() {
		h.cover(c, tr.Album.PicURL, 96, 40, 6).Shadow(0, 2, 6, 0, p.cardShade)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, tr.Name).FontSize(12.5).FontWeight(600).SingleLine()
			h.artistLinks(c, tr, p.muted, p.text).FontSize(11.5).SingleLine()
		})
	})
	if card.Clicked() {
		h.nowPlaying = true
	}
}

func menuItem(c *ui.Context, icon *ui.SVG, label, shortcut string) ui.Element {
	p := paletteFor(c.Theme())
	b := ui.ButtonBase(c.Key(label)).Role(ui.RoleMenuItem).Label(label).Height(30).PaddingX(8).Gap(9).Radius(7).
		AlignItems(ui.Center).Justify(ui.Start).FocusRing(false)
	if b.Hovered() || b.FocusVisible() {
		b.Background(p.press)
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(15, 15).TextColor(p.muted)
		ui.Text(c, label).FontSize(13).Grow(1)
		if shortcut != "" {
			ui.Text(c, shortcut).FontSize(12).TextColor(p.faint)
		}
	})
	return b
}

// popover holds the colors of a popover as AppKit draws them.
type popover struct {
	fill, edge, track, knob ui.Color
}

func popoverColors(dark bool) popover {
	if dark {
		return popover{fill: ui.Hex("#2b2b2e"), edge: ui.RGBA(255, 255, 255, 0.14), track: ui.RGBA(255, 255, 255, 0.16), knob: ui.Hex("#ececec")}
	}
	return popover{fill: ui.Hex("#f6f6f6"), edge: ui.RGBA(0, 0, 0, 0.12), track: ui.RGBA(0, 0, 0, 0.12), knob: ui.Hex("#ffffff")}
}

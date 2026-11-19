package fnmusic

import (
	"github.com/egoist/mygo/ui"
	"waygu-music/internal/pluginapi"
)

func (h *Home) castButton(c *ui.Context, pal palette) {
	state := h.host.Casting()
	if !state.Available {
		return
	}

	color := pal.muted
	if state.Sending {
		color = pal.accent
	}

	b := iconButton(c.Key("cast-button"), iconCast, 16, color, "无线投送 (DLNA)")
	if b.Clicked() {
		h.castOpen = !h.castOpen
		if h.castOpen {
			h.host.ScanCastingDevices()
		}
	}

	pop := popoverColors(pal.dark)
	ui.PopoverBase(c.Key("cast-popover"), b, &h.castOpen, func(panel ui.Element) {
		const arrow = 7
		panel.AttachTo(b, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, arrow+3, 0).Width(240).Padding(12, 14).
			Radius(12).Background(pop.fill).Border(0.5, pop.edge).Shadow(0, 8, 24, -4, ui.RGBA(0, 0, 0, 0.2)).
			Gap(8).Label("无线投送")
		panel.Draw(func(p *ui.Painter, r ui.Rect) {
			cx, foot := r.X+r.W/2, r.Y+r.H
			tip := (&ui.Path{}).MoveTo(cx-arrow-1, foot-0.75).LineTo(cx-1.2, foot+arrow-1.2).
				QuadTo(cx, foot+arrow, cx+1.2, foot+arrow-1.2).LineTo(cx+arrow+1, foot-0.75).Close()
			p.FillPath(tip, pop.fill)
			edge := (&ui.Path{}).MoveTo(cx-arrow-1, foot).LineTo(cx-1.2, foot+arrow-1.2).
				QuadTo(cx, foot+arrow, cx+1.2, foot+arrow-1.2).LineTo(cx+arrow+1, foot)
			p.StrokePath(edge, 0.5, pop.edge)
		})

		// Header row
		ui.Row(c).Justify(ui.SpaceBetween).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "投送至设备 (DLNA)").FontSize(12).FontWeight(600).TextColor(pal.text)
			scanLabel := "刷新"
			if state.Scanning {
				scanLabel = "搜索中…"
			}
			refreshBtn := ui.ButtonBase(c.Key("refresh-dlna")).Padding(2, 6).Radius(4).Children(func() {
				ui.Text(c, scanLabel).FontSize(11).TextColor(pal.accent)
			})
			if refreshBtn.Clicked() && !state.Scanning {
				h.host.ScanCastingDevices()
			}
		})

		// Divider
		ui.Box(c).Height(0.5).Background(pop.edge)

		// Option 1: Local speaker
		isLocal := !state.Sending
		localRow := ui.ButtonBase(c.Key("local-device")).Padding(6, 8).Radius(6).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconHeadset).Size(14, 14).TextColor(pal.muted)
				ui.Text(c, "本机播放").FontSize(12.5).TextColor(pal.text)
			})
			if isLocal {
				ui.Icon(c, iconCheck).Size(14, 14).TextColor(pal.accent)
			}
		})
		if localRow.Clicked() {
			h.host.SwitchPlaybackOutput("")
			h.castOpen = false
			h.host.Toast("已切换至本机播放")
			h.host.Invalidate()
		}

		// Discovered DLNA devices
		devices := state.Devices
		active := state.Active

		// Always retain the currently active device in the view so background scans never cause it to disappear
		if active != nil {
			found := false
			for _, d := range devices {
				if d.ID == active.ID || d.Name == active.Name {
					found = true
					break
				}
			}
			if !found {
				devices = append([]pluginapi.CastingDevice{*active}, devices...)
			}
		}

		for _, d := range devices {
			dev := d
			isActive := active != nil && (active.ID == dev.ID || active.Name == dev.Name)
			dRow := ui.ButtonBase(c.Key("dev-"+dev.ID)).Padding(6, 8).Radius(6).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(1).MinWidth(0).Children(func() {
					ui.Icon(c, iconCast).Size(14, 14).TextColor(pal.muted)
					ui.Text(c, dev.Name).FontSize(12.5).TextColor(pal.text).SingleLine()
				})
				if isActive {
					ui.Icon(c, iconCheck).Size(14, 14).TextColor(pal.accent)
				}
			})
			if dRow.Clicked() {
				h.host.SwitchPlaybackOutput(dev.ID)
				h.castOpen = false
				h.host.Toast("已连接投送设备: " + dev.Name)
				h.host.Invalidate()
			}
		}

		if len(devices) == 0 && !state.Scanning {
			ui.Text(c, "未发现局域网音响/电视设备").FontSize(11).TextColor(pal.muted).Padding(6, 4)
		}
	})
}

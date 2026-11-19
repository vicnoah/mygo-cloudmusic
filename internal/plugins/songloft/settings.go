package songloft

import (
	"context"

	"github.com/egoist/mygo/ui"
	backend "waygu-music/internal/plugins/songloft/internal/client"
)

func (h *Home) openConnection() {
	cfg := toConnectionConfig(h.state.Config())
	h.formAddress = cfg.Address
	h.formUser = cfg.Username
	h.connectionOpen = true
	h.host.OpenSettings()
}

func (h *Home) disconnect() {
	_ = h.runtime.Disconnect(context.Background())
}

func (h *Home) settingsView(c *ui.Context) {
	if h == nil || h.host == nil || h.state == nil {
		return
	}
	t := *c.Theme()
	c.Root().Background(t.Background)
	ui.Scroll(c).Fill().Padding(24, 28, 32, 28).Gap(8).Children(func() {
		ui.Text(c, backend.DefaultProfile.Name).FontSize(12).FontWeight(600).TextColor(t.TextMuted).Margin(14, 4, 4, 4)
		ui.Column(c).Radius(12).Background(t.Background).Border(1, t.Border).Dividers(1, t.Border).Children(func() {
			var cfg connectionConfig
			if h.runtime != nil {
				cfg = h.runtime.Config()
			}
			ui.Row(c).Padding(12, 16).Gap(16).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Gap(3).Children(func() {
					name := "未连接"
					desc := "连接后可播放音乐库、专辑、歌手和歌单"
					if h.state.Connected() {
						name = cfg.Username
						if name == "" {
							name = backend.DefaultProfile.Name
						}
						desc = "已连接 · " + cfg.Address
					}
					if err := h.state.Error(); err != nil {
						desc = "连接失败：" + err.Error()
					}
					ui.Text(c, name).FontSize(13.5).FontWeight(500)
					ui.Text(c, desc).FontSize(12).TextColor(t.TextMuted)
				})
				label := "连接…"
				if h.state.Connected() {
					label = "重新连接…"
				}
				if actionPill(c, "songloft-connection-open", iconServer, label, true).Clicked() {
					h.openConnection()
				}
				if h.state.Connected() && actionPill(c, "songloft-disconnect", iconSignOut, "断开", false).Clicked() {
					h.disconnect()
				}
			})
		})
		h.desktopSettings(c)
	})
	h.connectionDialog(c)
}

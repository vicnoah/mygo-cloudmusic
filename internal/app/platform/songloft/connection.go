package songloft

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"
	backend "netease-music-mygo/internal/platform/songloft"
)

func (h *Home) submitConnection() {
	if h.connectionBusy {
		return
	}
	address := strings.TrimSpace(h.formAddress)
	user := strings.TrimSpace(h.formUser)
	if address == "" || user == "" {
		h.connectionError = "请填写服务器地址和用户名"
		return
	}
	cfg := h.runtime.Config()
	cfg.Address = address
	cfg.Username = user
	if h.formPassword != "" {
		cfg.PassHash = h.formPassword
	}
	if cfg.Address == "" || cfg.Username == "" || cfg.PassHash == "" {
		h.connectionError = "请填写完整的连接信息和密码"
		return
	}
	h.connectionBusy, h.connectionError = true, ""
	if h.runtime == nil {
		h.connectionBusy = false
		h.connectionError = "平台未初始化"
		return
	}
	h.connectionGen++
	gen := h.connectionGen
	go func() {
		err := h.runtime.Connect(context.Background(), cfg)
		h.host.Post(func() {
			if gen != h.connectionGen {
				return
			}
			h.connectionBusy = false
			if err != nil {
				h.connectionError = err.Error()
				return
			}
			h.connectionOpen, h.formPassword = false, ""
			h.host.Invalidate()
		})
	}()
}

func (h *Home) connectionDialog(c *ui.Context) {
	if h == nil || !h.connectionOpen {
		return
	}
	t := c.Theme()
	ui.DialogBase(c.Key("connection-dialog-songloft"), &h.connectionOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(380).Padding(24).Radius(18).Gap(14).Background(t.Background).Border(1, t.Border).
			Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4))
		ui.Text(c, "连接 Songloft 服务器").FontSize(17).FontWeight(700)
		ui.Text(c, backend.DefaultProfile.Description).FontSize(12.5).TextColor(t.TextMuted).LineHeight(1.4).Margin(-8, 0, 0, 0)

		field := func(key, label, hint string, value *string, secret bool) {
			ui.Column(c.Key("songloft-dialog-" + key)).Gap(5).Children(func() {
				ui.Text(c, label).FontSize(12).FontWeight(600).TextColor(t.TextMuted)
				in := ui.TextInput(c.Key("songloft-dialog-"+key+"-input"), value).Placeholder(hint).Label(label).FontSize(14)
				if secret {
					in.Password()
				}
				if key == "address" {
					in.AutoFocus()
				}
				if in.Submitted() {
					h.submitConnection()
				}
			})
		}

		field("address", "服务器地址", backend.DefaultProfile.DefaultHint, &h.formAddress, false)
		field("user", "用户名", "Songloft 账号用户名", &h.formUser, false)
		field("password", "密码", "Songloft 账号密码", &h.formPassword, true)

		if h.connectionError != "" {
			ui.Text(c, h.connectionError).FontSize(12.5).TextColor(ui.Hex("#ef4444"))
		}

		ui.Row(c).Gap(10).Justify(ui.End).Margin(4, 0, 0, 0).Children(func() {
			if h.host.PillButton(c, "songloft-dialog-cancel", nil, "取消", false).Disabled(h.connectionBusy).Clicked() {
				h.connectionOpen = false
			}
			label := "连接"
			if h.connectionBusy {
				label = "正在连接…"
			}
			ok := h.host.PillButton(c, "songloft-dialog-submit", iconServer, label, true).Disabled(h.connectionBusy)
			if ok.Clicked() {
				h.submitConnection()
			}
		})
	})
}

func (h *Home) connectionPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(10).Children(func() {
		ui.Text(c, "连接 Songloft 服务器").FontSize(20).FontWeight(700)
		ui.Text(c, backend.DefaultProfile.Description).FontSize(13).TextColor(t.TextMuted)
		ui.Text(c, "直接串流原始音质（FLAC、MP3、AAC、OGG 等），支持歌词与封面").FontSize(12).TextColor(t.TextMuted)
		if err := h.state.Error(); err != nil {
			ui.Text(c, err.Error()).FontSize(12.5).TextColor(ui.Hex("#ef4444"))
		}
		ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(func() {
			if h.host.PillButton(c, "connect-songloft-now", iconServer, "连接服务器", true).Clicked() {
				h.openConnection()
			}
			if h.host.PillButton(c, "open-songloft-settings", iconServer, "打开设置", false).Clicked() {
				h.host.OpenSettings()
			}
		})
	})
}

func (h *Home) dialogs(c *ui.Context) {
	h.playlistEditDialog(c)
	h.playlistDeleteDialog(c)
}

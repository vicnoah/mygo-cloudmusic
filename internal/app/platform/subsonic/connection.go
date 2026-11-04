package subsonic

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"
	backend "netease-music-mygo/internal/platform/subsonic"
)

func (h *Home) disconnect() {
	h.connectionGen++
	gen := h.connectionGen
	h.connectionOpen, h.connectionBusy, h.connectionError = false, false, ""
	if h.runtime != nil {
		go func() {
			err := h.runtime.Disconnect(context.Background())
			h.host.Post(func() {
				if gen != h.connectionGen {
					return
				}
				if err != nil {
					h.connectionError = err.Error()
				}
				h.ResetNavigation(Route{Page: PageLibrary})
				h.host.Invalidate()
			})
		}()
	}
}

func (h *Home) openConnection() {
	if h == nil || h.state == nil || h.runtime == nil {
		return
	}
	if h.connectionBusy && h.runtime != nil {
		_ = h.runtime.Cancel(context.Background())
	}
	cfg := h.runtime.Config()
	h.connectionGen++
	h.formAddress = cfg.Address
	h.formUser = cfg.Username
	h.formInsecure = cfg.HTTPS
	h.formPassword = ""
	h.connectionError, h.connectionBusy, h.connectionOpen = "", false, true
	h.host.OpenSettings()
}

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
	cfg.Address = backend.NormalizeAddress(address, false)
	cfg.Username = user
	cfg.HTTPS = h.formInsecure // used for InsecureSkipVerify
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
	prof := h.state.Profile()
	dialogKey := "connection-dialog-" + string(prof.ID)
	ui.DialogBase(c.Key(dialogKey), &h.connectionOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(380).Padding(24).Radius(18).Gap(14).Background(t.Background).Border(1, t.Border).
			Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		ui.Text(c, "连接 "+prof.Name+" 服务器").FontSize(17).FontWeight(700)
		ui.Text(c, prof.Description).FontSize(12.5).TextColor(t.TextMuted).LineHeight(1.4).Margin(-8, 0, 0, 0)

		field := func(key, label, hint string, value *string, secret bool) {
			ui.Column(c.Key(string(prof.ID) + "-dialog-" + key)).Gap(5).Children(func() {
				ui.Text(c, label).FontSize(12).FontWeight(600).TextColor(t.TextMuted)
				in := ui.TextInput(c.Key(string(prof.ID)+"-dialog-"+key+"-input"), value).Placeholder(hint).Label(label).FontSize(14)
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

		field("address", "服务器地址", prof.DefaultHint, &h.formAddress, false)
		field("user", "用户名", prof.Name+" 账号用户名", &h.formUser, false)
		field("password", "密码", prof.Name+" 账号密码", &h.formPassword, true)

		ui.Row(c.Key(string(prof.ID) + "-dialog-tls")).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Checkbox(c.Key(string(prof.ID)+"-dialog-insecure"), &h.formInsecure, "允许自签名证书（跳过 TLS 验证）")
		})

		if h.connectionError != "" {
			ui.Text(c, h.connectionError).FontSize(12.5).TextColor(t.Accent)
		}

		ui.Row(c).Gap(10).Justify(ui.End).Margin(4, 0, 0, 0).Children(func() {
			if h.host.PillButton(c, string(prof.ID)+"-dialog-cancel", nil, "取消", false).Disabled(h.connectionBusy).Clicked() {
				h.connectionOpen = false
			}
			label := "连接"
			if h.connectionBusy {
				label = "正在连接…"
			}
			ok := h.host.PillButton(c, string(prof.ID)+"-dialog-submit", iconServer, label, true).Disabled(h.connectionBusy)
			if ok.Clicked() {
				h.submitConnection()
			}
		})
	})
}

func (h *Home) connectionPage(c *ui.Context) {
	t := c.Theme()
	prof := h.state.Profile()
	ui.Column(c).Fill().Center().Gap(10).Children(func() {
		ui.Text(c, "连接 "+prof.Name+" 服务器").FontSize(20).FontWeight(700)
		ui.Text(c, prof.Description).FontSize(13).TextColor(t.TextMuted)
		ui.Text(c, "直接串流原始音质（FLAC、MP3、AAC、OGG 等），支持歌词与封面").FontSize(12).TextColor(t.TextMuted)
		if err := h.state.Error(); err != nil {
			ui.Text(c, err.Error()).FontSize(12.5).TextColor(t.Accent)
		}
		if h.host.PillButton(c, "open-"+string(prof.ID)+"-settings", iconServer, "打开设置并连接", true).Clicked() {
			h.host.OpenSettings()
		}
	})
}

package qqmusic

import (
	"context"
	"time"

	backend "waygu-music/internal/plugins/qqmusic/internal/client"

	"github.com/egoist/mygo/ui"
)

type loginState struct {
	open     bool
	gen      int
	key      string
	code     *ui.Bitmap
	status   int
	scanner  string
	err      string
	finished bool
	wechat   bool
	cancel   context.CancelFunc
}

func (h *Home) openLogin() { h.openLoginChannel(false) }

func (h *Home) cancelLogin() {
	if h.login.cancel != nil {
		h.login.cancel()
	}
	backend.CancelQR(h.login.key)
	h.login = loginState{gen: h.login.gen + 1}
}

func (h *Home) openLoginChannel(wechat bool) {
	h.cancelLogin()
	ctx, cancel := context.WithCancel(h.scope)
	h.login.open, h.login.wechat, h.login.cancel = true, wechat, cancel
	gen := h.login.gen
	go func() {
		begin := backend.QRKeyContext
		if wechat {
			begin = backend.WechatQRKeyContext
		}
		key, err := begin(ctx)
		var code *ui.Bitmap
		if err == nil {
			context.AfterFunc(ctx, func() { backend.CancelQR(key) })
			var png []byte
			png, err = backend.QRImage(key)
			if err == nil {
				code, err = ui.DecodeBitmap(png)
			}
		}
		h.postResult(ctx, func() {
			if gen != h.login.gen || !h.login.open {
				backend.CancelQR(key)
				return
			}
			if err != nil {
				h.login.err = backend.LoginErrorMessage(err)
				return
			}
			h.login.key, h.login.code, h.login.status = key, code, backend.QRWaiting
			h.host.Invalidate()
			h.pollLogin(ctx, gen, key)
		})
	}()
}

func (h *Home) pollLogin(ctx context.Context, gen int, key string) {
	go func() {
		for {
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			code, _, name, _, err := backend.QRCheckContext(ctx, key)
			var prof *backend.Profile
			if code == backend.QRConfirmed {
				prof, err = backend.AccountContext(ctx)
			}
			done := make(chan bool, 1)
			h.postResult(ctx, func() {
				if gen != h.login.gen || !h.login.open {
					done <- true
					return
				}
				if code == backend.QRConfirmed && err == nil {
					err = backend.SaveSession()
				}
				switch {
				case err != nil:
					h.login.err = backend.LoginErrorMessage(err)
				case code == backend.QRConfirmed:
					if prof == nil {
						h.login.err = "未能获取 QQ 音乐账号，请刷新二维码"
						break
					}
					h.login.status, h.login.finished = code, true
					h.login.scanner = name
					h.signedIn(prof)
					h.closeLoginSoon(ctx, gen)
				case code == backend.QRScanned:
					h.login.status, h.login.scanner = code, name
				case code == backend.QRExpired:
					h.login.status = code
				}
				h.host.Invalidate()
				done <- code == backend.QRConfirmed || code == backend.QRExpired || err != nil
			})
			select {
			case <-ctx.Done():
				return
			case ended := <-done:
				if ended {
					return
				}
			}
		}
	}()
}

// closeLoginSoon leaves the welcome up for a moment before closing.
func (h *Home) closeLoginSoon(ctx context.Context, gen int) {
	go func() {
		timer := time.NewTimer(1200 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
			h.postResult(ctx, func() {
				if gen == h.login.gen {
					h.cancelLogin()
				}
			})
		}
	}()
}

func (h *Home) loginDialog(c *ui.Context) {
	if !h.login.open {
		return
	}
	t := c.Theme()
	l := &h.login
	ui.DialogBase(c.Key("qq-login"), &l.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(340).Padding(28, 28, 26, 28).Radius(22).Gap(6).AlignItems(ui.Center).
			Background(t.Background).Border(1, t.Border).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		channel := "QQ"
		if l.wechat {
			channel = "微信"
		}
		title, sub := "扫码登录", "打开"+channel+"，扫一扫登录"
		switch {
		case l.finished:
			name := l.scanner
			if p := h.state.Profile(); p != nil {
				name = p.Nickname
			}
			title, sub = "登录成功", "欢迎回来，"+name
		case l.err != "":
			title, sub = "登录失败", l.err
		case l.status == backend.QRScanned:
			title, sub = "扫描成功", "请在手机上确认登录"
		case l.status == backend.QRExpired:
			sub = "二维码已失效"
		}
		ui.Text(c, title).FontSize(20).FontWeight(700)
		if !l.finished {
			kind := 0
			if l.wechat {
				kind = 1
			}
			if segmented(c.Key("qq-login-channel"), &kind, "QQ", "微信") {
				h.openLoginChannel(kind == 1)
			}
		}
		ui.Text(c, sub).Width(284).MaxLines(3).FontSize(13).TextColor(t.TextMuted).TextAlign(ui.Center).Margin(0, 0, 14, 0)
		ui.Box(c).Size(212, 212).Radius(16).Background(ui.RGB(255, 255, 255)).Border(1, t.Border).Children(func() {
			if l.code != nil {
				opacity := float32(1)
				if l.status == backend.QRScanned || l.status == backend.QRExpired || l.finished || l.err != "" {
					opacity = 0.12
				}
				ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Opacity(opacity).Children(func() {
					ui.Image(c, l.code).Fill().Fit(ui.Contain)
				})
			}
			if l.code == nil && l.err == "" {
				ui.Column(c).Fill().Center().Gap(8).Children(func() {
					bone(c, 10).Size(104, 104)
					bone(c, 6).Width(96).Height(12)
				})
			}
			ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center().Gap(10).Children(func() {
				if l.status == backend.QRExpired || l.err != "" {
					if actionPill(c, "qq-login-refresh", iconReload, "刷新二维码", true).Clicked() {
						h.openLoginChannel(l.wechat)
					}
				} else if l.status == backend.QRScanned {
					if l.scanner != "" {
						ui.Text(c, l.scanner).FontSize(14).FontWeight(600).TextColor(t.Accent)
					}
					ui.Text(c, "请在手机上确认").FontSize(12.5).TextColor(t.TextMuted)
				}
			})
		})
	})
	if !l.open {
		h.cancelLogin()
	}
}

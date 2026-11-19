package fnmusic

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
)

// notch is the quick player around the camera housing at the top of the
// screen: black like the housing, it grows out of it as music plays (the
// cover on the left, the music's levels on the right) and opens into the
// player under the pointer. On a display without a notch it hangs from the
// top of the screen all the same.
type notch struct {
	home        *Home
	panel       *pluginapi.MenuBarPanel
	preferences pluginapi.QuickPlayerPreferences
	win         *mygo.Window

	w0, h0     float32 // the housing's size
	real       bool    // the display has one: else the notch hides until wanted
	winW, winH float32
	screenX    float32 // the window's left on the screen
	screenY    float32

	open       bool // the pointer is over it: it shows the player
	hello      bool // it shows itself, turned off, with a way to keep it
	enteredAt  time.Time
	lastInside time.Time
	passing    bool   // clicks pass through to what is under the window
	watched    bool   // the pointer's moves poll
	unwatch    func() // stops them polling, nil while they don't
	active     bool   // the app is active, when they go unseen over the menu bar
	polling    bool   // a poll is due
	playing    bool   // the previous update saw a song ready and playing

	bars  barState
	scrub scrubState
}

const (
	notchOpenW  = 468
	notchIdleW  = 330
	notchHelloW = 340
)

func (h *Home) quickPlayer(panel *pluginapi.MenuBarPanel) pluginapi.Window {
	n := &notch{home: h, panel: panel, w0: 200, h0: 32, winW: notchOpenW + 60, winH: 240}
	return pluginapi.Window{
		Options: mygo.WindowOptions{
			Title: appName + "迷你播放器", Width: int(n.winW), Height: int(n.winH),
			Panel: true, Hidden: true, Frameless: true, Transparent: true,
			DisableShadow: true, DisableResize: true,
		},
		Root: n.view, Mount: n.mount, Updated: n.update,
	}
}

func (n *notch) mount(win *mygo.Window) func() {
	n.win = win
	n.measure()
	n.place()
	offClose := win.OnClose(func(e *mygo.CloseEvent) { e.PreventDefault() })
	n.panel.Raise(win.NativeHandle())
	win.SetIgnoreMouseEvents(true)
	n.passing = true
	n.preferences = n.panel.Preferences()
	if n.preferences.Enabled {
		win.ShowInactive()
		n.pin()
	}
	offDisplays := mygo.Screen.OnDisplaysChanged(func() {
		n.measure()
		n.place()
		n.pin()
	})
	n.active = mygo.App.IsActive()
	offActive := mygo.App.OnDidBecomeActive(func() { n.active = true; n.poll() })
	offInactive := mygo.App.OnDidResignActive(func() { n.active = false })
	n.watch()
	n.poll()
	n.update()
	return func() {
		offClose()
		offDisplays()
		offActive()
		offInactive()
		if n.unwatch != nil {
			n.unwatch()
		}
		n.win, n.unwatch = nil, nil
		n.open, n.hello, n.polling, n.watched = false, false, false, false
		n.playing = false
		n.enteredAt, n.lastInside = time.Time{}, time.Time{}
		n.bars, n.scrub = barState{}, scrubState{}
	}
}

func (n *notch) pin() {
	n.panel.Pin(n.win.NativeHandle(), float64(n.winW), float64(n.winH))
}

func (n *notch) update() {
	prev := n.preferences
	n.preferences = n.panel.Preferences()
	if prev.Enabled != n.preferences.Enabled {
		n.setEnabled(n.preferences.Enabled)
	} else if prev.Virtual != n.preferences.Virtual {
		n.poll()
	}
	pl := n.home.host.Playback()
	playing := pl != nil && pl.Playing() && !pl.Loading()
	if playing && !n.playing {
		n.greet()
	}
	n.playing = playing
}

func (n *notch) after(d time.Duration, fn func()) {
	win := n.win
	time.AfterFunc(d, func() {
		mygo.RunOnMain(func() {
			if n.win == win {
				fn()
			}
		})
	})
}

// watch follows the pointer's moves while the notch shows, turned on or
// greeting. Otherwise it lets them be: each, anywhere on the screen, would
// wake the app for nothing.
func (n *notch) watch() {
	on := n.preferences.Enabled || n.hello
	switch {
	case on && n.unwatch == nil:
		n.unwatch, n.watched = n.panel.WatchPointer(n.poll)
	case !on && n.unwatch != nil:
		n.unwatch()
		n.unwatch, n.watched = nil, false
	}
}

// measure finds the housing of the display with the menu bar.
func (n *notch) measure() {
	n.w0, n.h0, n.real = 200, 32, false
	if w, h, ok := n.panel.CameraHousing(); ok {
		n.w0, n.h0, n.real = float32(w), float32(h), true
	} else if h := n.panel.MenuBarHeight(); h > 0 {
		// A notch of its own, as high as the menu bar it hangs over.
		n.h0 = float32(h)
	}
	if n.win != nil {
		n.win.Invalidate()
	}
}

// place centers the window at the top of the display with the menu bar,
// where the pointer finds it.
func (n *notch) place() {
	d := mygo.Screen.PrimaryDisplay()
	n.screenX = float32(d.Bounds.X) + (float32(d.Bounds.Width)-n.winW)/2
	n.screenY = float32(d.Bounds.Y)
}

// setEnabled shows the notch player, or hides it for good, as the
// settings say.
func (n *notch) setEnabled(on bool) {
	if n.win == nil {
		return // as in tests
	}
	if on == n.win.IsVisible() {
		n.win.Invalidate()
		n.watch()
		return
	}
	if on {
		n.win.ShowInactive()
		n.pin()
		n.watch()
		n.poll()
		return
	}
	n.open, n.hello = false, false
	n.win.Hide()
	n.watch()
}

// greet shows the notch once, as the first song plays, to a listener who
// never turned it on: it grows out of the housing, or the top of a screen
// without one, with the song and a way to keep it, and goes back in.
func (n *notch) greet() {
	if n.preferences.Enabled || n.preferences.Greeted {
		return
	}
	n.preferences.Greeted = true
	n.panel.SavePreferences(n.preferences)
	n.hello = true
	if n.win == nil {
		return // as in tests
	}
	n.win.ShowInactive()
	n.pin()
	n.watch()
	n.poll()
	n.byeSoon(7 * time.Second)
}

// byeSoon ends the greeting after d, unless the pointer is on it then.
func (n *notch) byeSoon(d time.Duration) {
	n.after(d, func() {
		switch {
		case !n.hello:
		case n.open || !n.passing:
			n.byeSoon(2 * time.Second)
		default:
			n.bye()
		}
	})
}

// bye ends the greeting: the notch goes back into the housing and, still
// turned off, hides there.
func (n *notch) bye() {
	n.hello = false
	n.win.Invalidate()
	if n.preferences.Enabled {
		return
	}
	n.open = false
	n.passing = true
	n.win.SetIgnoreMouseEvents(true)
	n.after(time.Second, func() {
		if !n.hello && !n.preferences.Enabled {
			n.win.Hide()
			n.watch()
		}
	})
}

// keep turns the notch player on, from its greeting.
func (n *notch) keep() {
	n.hello = false
	n.preferences.Enabled = true
	n.panel.SavePreferences(n.preferences)
	n.setEnabled(true)
}

// target returns the size the shape goes to.
func (n *notch) target() (w, h float32) {
	tr := n.home.host.Playback().Current()
	s := n.preferences
	switch {
	case n.open && tr == nil:
		return notchIdleW, n.h0 + 72
	case n.open:
		return notchOpenW, n.h0 + 178
	case n.hello && tr != nil:
		return notchHelloW, n.h0 + 58
	case !n.real && !s.Virtual:
		// Over a menu bar without a notch, it would hide the menus and
		// icons there: nothing shows until the pointer asks for it.
		return 0, 0
	case tr != nil && s.Activity && s.Enabled:
		// Wings just wide enough for the cover and the levels, so they
		// hide no more of the menu bar than they must.
		return n.w0 + 2*max(0, n.h0-12) + 20, n.h0
	default:
		return n.w0, n.h0
	}
}

// poll follows the pointer, which the window cannot always see: it lets
// clicks through where it draws nothing, and opens while over it.
func (n *notch) poll() {
	if !n.preferences.Enabled && !n.hello {
		return
	}
	pt := mygo.Screen.CursorScreenPoint()
	w, h := n.target()
	x := n.screenX + (n.winW-w)/2
	px, py := float32(pt.X), float32(pt.Y)
	slack := float32(6)
	inside := px >= x-slack && px <= x+w+slack && py >= n.screenY-slack && py <= n.screenY+h+slack
	if !n.real && !n.preferences.Virtual && !n.open && !n.hello {
		// Without a notch, the pointer pressed against the top edge of
		// the screen, about its middle, opens it.
		cx := n.screenX + n.winW/2
		inside = px >= cx-n.w0/2 && px <= cx+n.w0/2 && py <= n.screenY+1
	}
	now := time.Now()
	changed := false
	if inside {
		if n.lastInside.IsZero() || now.Sub(n.lastInside) > 100*time.Millisecond {
			n.enteredAt = now
		}
		n.lastInside = now
		// A pointer passing by to the menu bar does not open it: one that
		// rests there does. While it greets it stays as it is, the pointer
		// on its button.
		if !n.open && !n.hello && now.Sub(n.enteredAt) >= 300*time.Millisecond {
			n.open, changed = true, true
		}
	} else if n.open && now.Sub(n.lastInside) > 300*time.Millisecond {
		n.open, changed = false, true
	}
	pass := !inside
	if n.win == nil {
		n.passing = pass
		return // as in tests
	}
	if pass != n.passing {
		n.passing = pass
		n.win.SetIgnoreMouseEvents(pass)
	}
	if changed {
		n.win.Invalidate()
	}
	n.pollAgain(inside)
}

// pollAgain polls again soon while the pointer is over the notch or it is
// open, for the times it rests and leaves; and, more slowly, while the app
// is active, when the pointer's moves outside its windows go unseen, or
// when they are never seen. Otherwise the next move polls.
func (n *notch) pollAgain(inside bool) {
	var d time.Duration
	switch {
	case inside || n.open:
		d = 33 * time.Millisecond
	case n.active || !n.watched:
		d = 100 * time.Millisecond
	}
	if d == 0 || n.polling {
		return
	}
	n.polling = true
	n.after(d, func() {
		n.polling = false
		n.poll()
	})
}

// shape returns the outline of the housing grown to w×h: straight sides
// that flare into the top edge, and a rounded foot.
func shape(cx, w, h, flare, foot float32) *ui.Path {
	const k = 0.5523 // a quarter circle's control points
	l, r := cx-w/2, cx+w/2
	p := &ui.Path{}
	p.MoveTo(l-flare, 0)
	p.CubeTo(l-flare+flare*k, 0, l, flare-flare*k, l, flare)
	p.LineTo(l, h-foot)
	p.CubeTo(l, h-foot+foot*k, l+foot-foot*k, h, l+foot, h)
	p.LineTo(r-foot, h)
	p.CubeTo(r-foot+foot*k, h, r, h-foot+foot*k, r, h-foot)
	p.LineTo(r, flare)
	p.CubeTo(r, flare-flare*k, r+flare-flare*k, 0, r+flare, 0)
	p.Close()
	return p
}

func (n *notch) view(c *ui.Context) {
	home := n.home
	pl := home.host.Playback()
	tr := pl.Current()
	c.Root().Background(ui.Transparent)

	tw, th := n.target()
	spring := ui.Animation{Spring: ui.SpringOptions{Duration: 520 * time.Millisecond, Bounce: 0.28}}
	if !n.open {
		spring.Spring.Bounce = 0.12
	}
	stage := ui.Box(c).Fill()
	// A spring overshoots: never below nothing.
	w := max(0, stage.AnimateTo("w", tw, spring))
	h := max(0, stage.AnimateTo("h", th, spring))
	opened := float32(0)
	if n.open {
		opened = 1
	}
	open := stage.AnimateTo("open", opened, spring)
	var greeting float32
	if n.hello {
		greeting = 1
	}
	hello := stage.AnimateTo("hello", greeting, spring)
	foot := 10 + 18*open
	flare := 6 + 8*open
	cx := n.winW / 2

	stage.Draw(func(p *ui.Painter, r ui.Rect) {
		if w < 1 || h < 1 {
			return
		}
		path := shape(r.X+cx, w, h, flare, foot)
		if open > 0.05 {
			p.Shadow(ui.Rect{X: r.X + cx - w/2, Y: r.Y + h - 40, W: w, H: 40}, foot, 0, 14, 30, -6, ui.RGBA(0, 0, 0, 0.45*open))
		}
		p.FillPath(path, ui.RGB(0, 0, 0))
	})
	stage.Children(func() {
		body := ui.Box(c).Absolute().Top(0).Left(cx-w/2).Size(w, h).Radius(0, 0, foot, foot).Clip().TextColor(white)
		body.Children(func() {
			if tr == nil {
				n.idle(c, open, (w-notchIdleW)/2)
				return
			}
			if n.preferences.Activity || !n.preferences.Enabled {
				n.compact(c, tr, 1-open)
			}
			n.helloRow(c, hello*(1-open), (w-notchHelloW)/2)
			if open > 0.02 {
				n.expanded(c, tr, open, (w-notchOpenW)/2)
			}
		})
	})
}

// compact shows the cover on the housing's left and the levels on its right.
func (n *notch) compact(c *ui.Context, tr *pluginapi.Track, shown float32) {
	if shown < 0.02 {
		return
	}
	art := n.home.host.DecodedArtwork(tr.Album.PicURL, 96, false)
	s := n.h0 - 12
	ui.Row(c).Absolute().Top(0).Left(0).Right(0).Height(n.h0).PaddingX(7).AlignItems(ui.Center).Opacity(shown).Children(func() {
		cov := ui.Box(c).Size(s, s).Radius(s * 0.28)
		in := artIn(cov, bitmap(art))
		if in < 1 {
			cov.Background(white.Alpha(0.12))
		}
		cov.Children(func() { artImage(c, bitmap(art), s*0.28, in) })
		ui.Spacer(c)
		n.levels(c, tr, 16, s*0.7, 4)
	})
}

// levels draws the music's levels in the cover's color.
func (n *notch) levels(c *ui.Context, tr *pluginapi.Track, w, h float32, count int) {
	home := n.home
	color := ui.RGB(255, 255, 255)
	if art := home.host.DecodedArtwork(tr.Album.PicURL, 96, false); art != nil && !art.Failed() {
		color = art.Tint().Mix(white, 0.35)
	}
	ui.Box(c).Size(w, h).Draw(func(p *ui.Painter, r ui.Rect) {
		lv, live := home.host.Playback().Levels()
		n.bars.step(p.Now(), lv, live)
		drawBars(p, r, &n.bars, count, color, 2.5)
		if live || n.bars.group(0, pluginapi.SpectrumBands) > 0.01 {
			p.AnimationFrame()
		}
	})
}

func (n *notch) expanded(c *ui.Context, tr *pluginapi.Track, open, left float32) {
	home := n.home
	pl := home.host.Playback()
	art := home.host.DecodedArtwork(tr.Album.PicURL, 200, false)
	home.tickTimes(c)
	col := ui.Column(c).Absolute().Top(n.h0+6).Left(left).Width(notchOpenW).Padding(8, 22, 16, 22).Gap(12).Opacity(max(0, open*1.4-0.4))
	col.Translate(0, (1-open)*-10)
	col.Children(func() {
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			cov := ui.ButtonBase(c.Key("cover")).Size(60, 60).Radius(12).FocusRing(false).Label("打开正在播放")
			lift := ui.Pose{}
			if cov.Hovered() {
				lift.Scale = 1.05
			}
			cov.AnimatePose(lift, ui.Snappy)
			in := artIn(cov, bitmap(art))
			if in < 1 {
				cov.Background(white.Alpha(0.12))
			}
			cov.Children(func() { artImage(c, bitmap(art), 12, in) })
			if cov.Clicked() {
				home.nowPlaying = true
				home.host.ShowMain()
				home.host.Invalidate()
			}
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				ui.Text(c, tr.Name).FontSize(15).FontWeight(700).SingleLine()
				if pl.Loading() {
					ui.Text(c, "正在缓冲…").FontSize(12.5).TextColor(white.Alpha(0.55)).SingleLine()
				} else {
					home.artistLinks(c, tr, white.Alpha(0.55), white).FontSize(12.5).SingleLine()
				}
			})
			home.likeButton(c.Key("like"), tr, 17, white.Alpha(0.6))
			n.levels(c, tr, 26, 22, 5)
		})
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, clock(pl.Position())).FontSize(10.5).FontFeatures("tnum").TextColor(white.Alpha(0.5)).Width(30)
			home.scrubber(c.Key("notch-scrub"), &n.scrub, white.Alpha(0.18), white.Alpha(0.9), 14, ui.Color{}).Grow(1)
			ui.Text(c, "-"+clock(pl.Duration()-pl.Position())).FontSize(10.5).FontFeatures("tnum").TextColor(white.Alpha(0.5)).Width(34).TextAlign(ui.End)
		})
		ui.Box(c).Margin(-6, 0, 0, 0).Children(func() {
			home.transport(c, white, white.Alpha(0.4), white, 20)
		})
	})
}

// helloRow says, under the housing, what the notch is, while it greets.
func (n *notch) helloRow(c *ui.Context, shown, left float32) {
	if shown < 0.02 {
		return
	}
	home := n.home
	row := ui.Row(c).Absolute().Top(n.h0+2).Left(left).Width(notchHelloW).Padding(6, 14, 14, 20).Gap(12).AlignItems(ui.Center).
		Opacity(max(0, shown*1.4-0.4))
	row.Translate(0, (1-shown)*-8)
	row.Children(func() {
		title, hint := "刘海里也能控制播放", "指针停在这里，即可暂停、切歌"
		if !n.real {
			title, hint = "屏幕顶部也能控制播放", "指针移到顶部中央，即可暂停、切歌"
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(13.5).FontWeight(700).SingleLine()
			ui.Text(c, hint).FontSize(11.5).TextColor(white.Alpha(0.55)).SingleLine()
		})
		b := ui.ButtonBase(c.Key("keep")).Padding(6, 14).Radius(999).Background(home.pal.accent).FocusRing(false).Label("开启刘海播放器")
		b.Children(func() { ui.Text(c, "开启").FontSize(12.5).FontWeight(600) })
		if b.Clicked() {
			n.keep()
		}
	})
}

// idle is the notch with nothing to play: it opens on a way to the app.
func (n *notch) idle(c *ui.Context, open, left float32) {
	if open < 0.02 {
		return
	}
	home := n.home
	ui.Row(c).Absolute().Top(n.h0+4).Left(left).Width(notchIdleW).Padding(10, 20, 14, 20).Gap(12).AlignItems(ui.Center).
		Opacity(max(0, open*1.4-0.4)).Children(func() {
		ui.Box(c).Size(36, 36).Radius(10).Background(home.pal.accent).Center().Children(func() {
			ui.Icon(c, iconNote).Size(18, 18).TextColor(white)
		})
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, appName).FontSize(14).FontWeight(700)
			ui.Text(c, "未在播放").FontSize(12).TextColor(white.Alpha(0.5))
		})
		b := ui.ButtonBase(c.Key("open")).Padding(6, 14).Radius(999).Background(white.Alpha(0.14)).FocusRing(false)
		b.Children(func() { ui.Text(c, "打开").FontSize(12.5).FontWeight(600) })
		if b.Clicked() {
			home.host.ShowMain()
		}
	})
}

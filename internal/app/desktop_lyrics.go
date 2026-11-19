package app

// The desktop lyrics window keeps its controls independent from Now Playing.

import (
	"time"

	"github.com/egoist/mygo"

	"waygu-music/internal/pluginapi"
)

// newDesktopLyrics creates the hidden, click-through window kept
// separate from Now Playing.
func (a *app) newDesktopLyrics() {
	if a.desktopLyricsWin != nil || a.platformWindows[a.platformID][pluginapi.DesktopLyricsWindow].Root == nil {
		return
	}
	options := a.windowOptions(pluginapi.DesktopLyricsWindow)
	prefs := a.desktopLyricsPrefs()
	displays := mygo.Screen.Displays()
	d := desktopLyricsDisplayArea(mygo.Screen.PrimaryDisplay())
	x, y := desktopLyricsDefaultPosition(d, options.Width, options.Height)
	if prefs.PositionSet {
		position := desktopLyricsPosition(mygo.Rectangle{
			X: prefs.X, Y: prefs.Y, Width: options.Width, Height: options.Height,
		}, displays)
		x, y = position.X, position.Y
		if prefs.X != x || prefs.Y != y {
			// A disconnected display can leave the saved window completely off
			// screen. Persist the fallback so every later launch uses it too.
			prefs.X, prefs.Y = x, y
			_ = a.settings.save()
		}
	}
	a.desktopLyricsLocked = prefs.Locked
	a.desktopLyricsShowTranslation = prefs.ShowTranslation
	a.desktopLyricsFontSize = prefs.FontSize
	a.desktopLyricsColor = prefs.Color
	options.X, options.Y = x, y
	win := mygo.NewWindow(options)
	a.desktopLyricsWin = win
	// MyGo uses the material at creation time to enable DirectComposition.
	// Drop the Acrylic backdrop immediately, keeping the composed surface so
	// transparent pixels reveal the desktop instead of a grey material.
	win.SetVibrancy(mygo.VibrancyNone)
	setDesktopLyricsMouseIgnored(win, true)
	a.desktopLyricsIgnoresMouse = true
	win.SetVisibleOnAllWorkspaces(true)
	win.OnClose(func(e *mygo.CloseEvent) {
		if !a.quitting {
			e.PreventDefault()
			win.Hide()
		}
	})
	win.OnHide(func() {
		if a.desktopLyricsWin != win {
			return
		}
		a.desktopLyricsPointerInside = false
		a.saveDesktopLyricsState()
		a.stopDesktopLyricsPolling()
	})
	a.desktopLyricsOffDisplays = mygo.Screen.OnDisplaysChanged(func() {
		if a.desktopLyricsWin != win {
			return
		}
		placeDesktopLyricsWindow(win, options.Width, options.Height)
		a.saveDesktopLyricsState()
	})
}

func (a *app) destroyDesktopLyrics() {
	win := a.desktopLyricsWin
	if win == nil {
		return
	}
	a.saveDesktopLyricsState()
	a.desktopLyricsWin = nil
	a.desktopLyricsPointerInside = false
	a.stopDesktopLyricsPolling()
	a.desktopLyricsOffDisplays()
	a.desktopLyricsOffDisplays = nil
	win.Destroy()
}

func placeDesktopLyricsWindow(w *mygo.Window, width, height int) {
	r := w.Bounds()
	if r.Width <= 0 {
		r.Width = width
	}
	if r.Height <= 0 {
		r.Height = height
	}
	position := desktopLyricsPosition(r, mygo.Screen.Displays())
	if position.X == r.X && position.Y == r.Y {
		return
	}
	w.SetBounds(position)
}

const (
	desktopLyricsMinVisibleWidth  = 100
	desktopLyricsMinVisibleHeight = 50
)

func desktopLyricsDisplayArea(d mygo.Display) mygo.Rectangle {
	if d.WorkArea.Width > 0 && d.WorkArea.Height > 0 {
		return d.WorkArea
	}
	return d.Bounds
}

func desktopLyricsDefaultPosition(workArea mygo.Rectangle, width, height int) (int, int) {
	if workArea.Width <= 0 || workArea.Height <= 0 {
		return 0, 0
	}
	return workArea.X + max(0, (workArea.Width-width)/2),
		workArea.Y + max(0, workArea.Height-height-24)
}

// desktopLyricsPosition keeps a saved window visible on the current displays.
// It preserves valid negative coordinates for displays placed left or above
// the primary display, while recovering positions left by disconnected ones.
func desktopLyricsPosition(r mygo.Rectangle, displays []mygo.Display) mygo.Rectangle {
	best := -1
	bestArea := 0
	for i, d := range displays {
		a := desktopLyricsDisplayArea(d)
		if a.Width <= 0 || a.Height <= 0 {
			continue
		}
		if r.X >= a.X && r.Y >= a.Y && r.X+r.Width <= a.X+a.Width && r.Y+r.Height <= a.Y+a.Height {
			return r
		}
		w := min(a.X+a.Width, r.X+r.Width) - max(a.X, r.X)
		h := min(a.Y+a.Height, r.Y+r.Height) - max(a.Y, r.Y)
		area := max(w, 0) * max(h, 0)
		if area > bestArea {
			best, bestArea = i, area
		}
	}

	if best >= 0 {
		a := desktopLyricsDisplayArea(displays[best])
		visibleW := min(a.X+a.Width, r.X+r.Width) - max(a.X, r.X)
		visibleH := min(a.Y+a.Height, r.Y+r.Height) - max(a.Y, r.Y)
		if visibleW >= min(desktopLyricsMinVisibleWidth, r.Width) && visibleH >= min(desktopLyricsMinVisibleHeight, r.Height) {
			r.X = min(max(r.X, a.X), a.X+max(0, a.Width-r.Width))
			r.Y = min(max(r.Y, a.Y), a.Y+max(0, a.Height-r.Height))
			return r
		}
	}

	// No current display contains enough of the saved window. Prefer the
	// primary display, then the first usable display in case enumeration is
	// temporarily incomplete during a display change.
	for _, d := range displays {
		a := desktopLyricsDisplayArea(d)
		if d.Primary && a.Width > 0 && a.Height > 0 {
			r.X, r.Y = desktopLyricsDefaultPosition(a, r.Width, r.Height)
			return r
		}
	}
	for _, d := range displays {
		a := desktopLyricsDisplayArea(d)
		if a.Width > 0 && a.Height > 0 {
			r.X, r.Y = desktopLyricsDefaultPosition(a, r.Width, r.Height)
			return r
		}
	}
	return r
}

func (a *app) toggleDesktopLyrics() {
	if a.desktopLyricsWin == nil {
		return
	}
	if a.desktopLyricsWin.IsVisible() {
		a.saveDesktopLyricsState()
		a.desktopLyricsWin.Hide()
		return
	}
	a.desktopLyricsWin.ShowInactive()
	a.updateDesktopLyricsInput()
	a.desktopLyricsWin.SetVisibleOnAllWorkspaces(true)
	a.startDesktopLyricsPolling()
	a.redraw()
}

func (a *app) toggleDesktopLyricsLock() {
	a.desktopLyricsLocked = !a.desktopLyricsLocked
	a.settings.DesktopLyricsLocked = a.desktopLyricsLocked
	a.saveDesktopLyricsState()
	if a.tray != nil {
		a.tray.SetMenu(a.trayMenu())
	}
	a.updateDesktopLyricsInput()
	a.redraw()
}

func (a *app) desktopLyricsPrefs() *desktopLyricsSettings {
	if a.settings.DesktopLyrics == nil {
		prefs := defaultDesktopLyricsSettings()
		prefs.Locked = a.settings.DesktopLyricsLocked
		a.settings.DesktopLyrics = &prefs
	}
	prefs := a.settings.DesktopLyrics
	if prefs.FontSize <= 0 {
		prefs.FontSize = 30
	}
	return prefs
}

func (a *app) saveDesktopLyricsState() {
	prefs := a.desktopLyricsPrefs()
	prefs.Locked = a.desktopLyricsLocked
	prefs.ShowTranslation = a.desktopLyricsShowTranslation
	prefs.FontSize = a.desktopLyricsFontSize
	prefs.Color = a.desktopLyricsColor
	if a.desktopLyricsWin != nil {
		b := a.desktopLyricsWin.Bounds()
		if b.Width > 0 && b.Height > 0 {
			prefs.X, prefs.Y, prefs.PositionSet = b.X, b.Y, true
		}
	}
	a.settings.DesktopLyricsLocked = a.desktopLyricsLocked
	_ = a.settings.save()
}

// startDesktopLyricsPolling tracks the pointer even while a locked window is
// click-through. Native pointer events cannot wake a window in that state.
func (a *app) startDesktopLyricsPolling() {
	a.stopDesktopLyricsPolling()
	var poll *time.Timer
	poll = time.AfterFunc(33*time.Millisecond, func() {
		mygo.RunOnMain(func() {
			if a.desktopLyricsPoll != poll {
				return
			}
			a.desktopLyricsPoll = nil
			if a.quitting || a.desktopLyricsWin == nil || !a.desktopLyricsWin.IsVisible() {
				return
			}
			a.updateDesktopLyricsInput()
			a.startDesktopLyricsPolling()
		})
	})
	a.desktopLyricsPoll = poll
}

func (a *app) stopDesktopLyricsPolling() {
	if a.desktopLyricsPoll != nil {
		a.desktopLyricsPoll.Stop()
		a.desktopLyricsPoll = nil
	}
}

func (a *app) updateDesktopLyricsInput() {
	win := a.desktopLyricsWin
	if win == nil || !win.IsVisible() {
		return
	}
	b := win.Bounds()
	p := mygo.Screen.CursorScreenPoint()
	inside := p.X >= b.X && p.X < b.X+b.Width && p.Y >= b.Y && p.Y < b.Y+b.Height
	if inside != a.desktopLyricsPointerInside {
		a.desktopLyricsPointerInside = inside
		win.Invalidate()
	}
	// A locked window is click-through even when the pointer is inside.
	if ignore := a.desktopLyricsLocked || !inside; ignore != a.desktopLyricsIgnoresMouse {
		setDesktopLyricsMouseIgnored(win, ignore)
		a.desktopLyricsIgnoresMouse = ignore
	}
}

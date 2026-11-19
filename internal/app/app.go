package app

import (
	"context"
	"errors"
	"os"
	"runtime"
	"time"

	"github.com/egoist/mygo"

	"waygu-music/internal/player"
	appplatform "waygu-music/internal/pluginapi"
)

// app owns the native windows and the services shared by platform plugins.
type app struct {
	win, settingsWin, desktopLyricsWin              *mygo.Window
	desktopLyricsLocked, desktopLyricsPointerInside bool
	desktopLyricsShowTranslation                    bool
	desktopLyricsFontSize                           float32
	desktopLyricsColor                              int
	desktopLyricsPoll                               *time.Timer
	desktopLyricsOffDisplays                        func()
	desktopLyricsIgnoresMouse                       bool
	tray                                            *mygo.Tray
	art                                             *artCache
	pl                                              *player.Player
	toast                                           string
	volume                                          float64
	dlnaScanning                                    bool
	mpvDownloading                                  bool
	mpvDone, mpvTotal                               int64
	mpvCancel                                       context.CancelFunc
	quitting                                        bool

	settings     settings
	cacheUsed    int64
	updates      bool
	devClicks    int
	devClickTime time.Time

	appPlatformState

	restored     bool
	savedSession string
	sessionDue   bool
}

// post runs fn on the main thread and redraws both windows. Without a
// window, as in tests, it drops fn.
func (a *app) post(fn func()) {
	if a.win == nil {
		return
	}
	a.win.Update(func() {
		fn()
		a.redraw()
	})
}

func (a *app) redraw() {
	if a.win != nil {
		a.win.Invalidate()
	}
	if a.desktopLyricsWin != nil {
		a.desktopLyricsWin.Invalidate()
	}
	for _, window := range a.mountedWindows {
		if window.updated != nil {
			window.updated()
		}
		window.native.Invalidate()
	}
}

func (a *app) showMain() {
	if a.win != nil {
		if a.win.IsMinimized() {
			a.win.Restore()
		}
		a.win.Show()
		a.win.Focus()
		// From the menu bar alone the app may be inactive.
		mygo.App.Focus()
	}
}

// Run runs the app until it quits.
func Run(registrations []appplatform.Registration) error {
	// Before the lock and anything that reads them: the app's folders are
	// named in ASCII, though it calls itself 肥牛音乐.
	useDataPaths()
	// Starting the app again, as from its shortcut while it plays from the
	// tray, hands the launch over to the instance already running, which
	// opens its window again, instead of running a second app beside it.
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	a := &app{cacheUsed: -1, appPlatformState: appPlatformState{platformRegistrations: registrations}}
	var startupErr error
	moveOldData()
	a.useUpdater()
	mygo.App.WhenReady(func() {
		a.settings = loadSettings(a.platformRegistrations...)
		if os.Getenv("NM_SHOTS") != "" {
			a.settings.path = ""
		}
		a.settings.applyAppearance()
		// Before the window: on Windows it lays out its content as it is
		// made, and the view asks the player what plays.
		a.art = newArtCache(a.post)
		a.pl = player.New(a.post)
		a.pl.Cache = newSongCache(a.settings.CacheMB)
		a.syncMPVSettings()
		a.pl.Changed = func() {
			a.keepSession()
			if changed := a.platformLifecycles[a.platformID].PlaybackChanged; changed != nil {
				changed()
			}
			a.redraw()
		}
		a.volume = a.settings.Volume
		a.pl.SetVolume(a.volume)
		// Load the selected platform before the first window frame.
		if !a.initPlatformState() {
			startupErr = errors.New("no platform plugin registered a main window")
			mygo.App.Quit()
			return
		}
		a.configurePlatformPlayer()
		a.activatePlatformView()
		options := a.windowOptions(appplatform.MainWindow)
		a.win = mygo.NewWindow(options)
		a.syncPlatformWindows()
		a.setMenu()
		a.mountPlatformWindows()
		a.platformReady(true)
		// From the menu bar alone, the Dock icon shows while the window
		// does.
		a.syncDock()
		a.win.OnShow(a.syncDock)
		a.win.OnHide(a.syncDock)
		a.win.OnFocus(func() {
			if focused := a.platformLifecycles[a.platformID].MainFocused; focused != nil {
				focused()
			}
		})
		a.applyTray()
		// Closing the window hides it: music plays on, and the Dock icon
		// or the tray's brings it back. A Linux desktop without a tray
		// has neither, so there closing it quits.
		a.win.OnClose(func(e *mygo.CloseEvent) {
			if a.quitting {
				return
			}
			e.PreventDefault()
			if runtime.GOOS == "darwin" || a.tray != nil {
				a.win.Hide()
			} else {
				mygo.App.Quit()
			}
		})
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.quitting = true
		if startupErr != nil {
			return
		}
		a.deactivatePlatformView(false)
		a.unmountPlatformWindows()
		a.destroyDesktopLyrics()
		a.settings.Volume = a.volume
		a.settings.save()
		a.saveSession()
	})
	// Windows leaves an icon in the notification area after its app until
	// the pointer passes over it.
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		if a.tray != nil {
			a.tray.Destroy()
		}
	})
	mygo.App.OnWindowAllClosed(func() {})
	// The Dock icon's click, and the launch another instance handed over
	// above, as the shortcut's while the window hides in the tray.
	mygo.App.OnActivate(func(bool) { a.showMain() })
	mygo.App.OnSecondInstance(func([]string, string) { a.showMain() })
	if err := mygo.App.Run(); err != nil {
		return err
	}
	return startupErr
}

func (a *app) onVersionClicked() {
	now := time.Now()
	if now.Sub(a.devClickTime) > 3*time.Second {
		a.devClicks = 0
	}
	a.devClickTime = now
	a.devClicks++
	if a.devClicks >= 5 {
		a.devClicks = 0
		a.settings.DeveloperMode = !a.settings.DeveloperMode
		a.settings.save()
		if a.settings.DeveloperMode {
			a.toast = "已开启开发者模式"
		} else {
			a.toast = "已关闭开发者模式"
		}
		a.redraw()
		if a.settingsWin != nil {
			a.settingsWin.Invalidate()
		}
	} else if a.devClicks >= 3 {
		a.redraw()
		if a.settingsWin != nil {
			a.settingsWin.Invalidate()
		}
	}
}

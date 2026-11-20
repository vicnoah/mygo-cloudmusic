package app

import (
	"context"
	"errors"
	"log"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/waygu-music-plugin/sdk/desktop"
	appplatform "github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/pluginapi/artwork"

	"waygu-music/internal/player"
)

// app owns the native windows and the services shared by platform plugins.
type app struct {
	win, settingsWin  *mygo.Window
	pluginsWin        *mygo.Window
	desktopLyrics     desktop.LyricsWindow
	tray              *mygo.Tray
	art               *artwork.Cache
	pl                *player.Player
	toast             string
	volume            float64
	desktopTasks      desktopTasks
	mpvDone, mpvTotal int64
	mpvCancel         context.CancelFunc
	cacheMeasure      uint64
	cacheLimitRev     atomic.Uint64
	cacheLimitMu      sync.Mutex
	quitting          bool

	settings  settings
	cacheUsed int64
	updates   bool

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
		if a.settingsWin != nil {
			a.settingsWin.Invalidate()
		}
	})
}

func (a *app) redraw() {
	if a.win != nil {
		a.win.Invalidate()
	}
	a.desktopLyrics.Invalidate()
	if a.pluginsWin != nil {
		a.pluginsWin.Invalidate()
	}
	for _, window := range a.mountedWindows {
		if window.updated != nil {
			window.updated()
		}
		window.native.Invalidate()
	}
}

func (a *app) showMain() {
	if remote := a.remoteActive(); remote != nil {
		go func() { _, _ = remote.action("show") }()
		return
	}
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
	return run(registrations, false)
}

func run(registrations []appplatform.Registration, managed bool) error {
	// Before the lock and anything that reads them: the app's folders are
	// named in ASCII, though it calls itself 肥牛音乐.
	useDataPaths()
	// Starting the app again, as from its shortcut while it plays from the
	// tray, hands the launch over to the instance already running, which
	// opens its window again, instead of running a second app beside it.
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	if err := desktop.SetApplicationIdentity(desktopApplicationID()); err != nil {
		return err
	}
	if runtime.GOOS == "windows" && !mygo.IsDev() {
		if err := PrepareInstallerShortcuts(); err != nil {
			log.Print(err)
		}
	}
	a := &app{cacheUsed: -1, appPlatformState: appPlatformState{platformRegistrations: registrations}}
	if managed {
		a.managedPlugins = make(map[string]*managedPlugin)
		a.localPlugins = make(map[appplatform.ID]*localPlugin)
		a.pluginContext, a.cancelPlugins = context.WithCancel(context.Background())
	}
	var startupErr error
	moveOldData()
	a.useUpdater()
	mygo.App.WhenReady(func() {
		a.settings = loadSettings(a.platformRegistrations...)
		a.loadPluginRegistry()
		a.settings.applyAppearance()
		// Before the window: on Windows it lays out its content as it is
		// made, and the view asks the player what plays.
		a.art = artwork.NewCache(a.post)
		a.pl = player.New(a.post)
		a.pl.Cache = newSongCache(a.settings.CacheMB)
		a.syncMPVSettings()
		a.pl.Changed = func() {
			a.keepSession()
			if remote := a.remoteActive(); remote != nil {
				remote.publishPlayback(a, false)
			}
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
		_ = a.win.SetIcon(desktopApplicationIcon())
		a.measureCache()
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
		if id := appplatform.ID(a.settings.Platform); a.localPlugins[id] != nil && a.localPlugins[id].program.Path != "" {
			a.startLocalPlugin(id)
		}
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
		if a.cancelPlugins != nil {
			a.cancelPlugins()
		}
		if startupErr != nil {
			return
		}
		if a.mpvCancel != nil {
			a.mpvCancel()
		}
		a.deactivatePlatformView(false)
		a.cancelLocalRequest()
		a.revokeLocalPlugin()
		a.unmountPlatformWindows()
		a.desktopLyrics.Destroy()
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
	err := mygo.App.Run()
	if a.pl != nil {
		a.pl.Close()
	}
	// Reap both the active process and any launch canceled during shutdown.
	a.pluginWorkers.Wait()
	if err != nil {
		return err
	}
	return startupErr
}

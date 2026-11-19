package pluginapi

import (
	"context"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type WindowID string

const (
	MainWindow          WindowID = "main"
	SettingsWindow      WindowID = "settings"
	DesktopLyricsWindow WindowID = "desktop-lyrics"
	QuickPlayerWindow   WindowID = "quick-player"
)

type MenuID string

const (
	ApplicationMenu MenuID = "application"
	TrayMenu        MenuID = "tray"
)

// Window is a complete native window supplied by a plugin.
type Window struct {
	Options mygo.WindowOptions
	Root    func(*ui.Context)
	// Mount connects a separately managed window to its controller.
	// The returned function releases subscriptions before the window closes.
	Mount   func(*mygo.Window) func()
	Updated func()
}

type QuickPlayerPreferences struct {
	Enabled, Activity, Virtual, Greeted bool
}

// MenuBarPanel provides access to native display geometry and pointer events.
type MenuBarPanel struct {
	CameraHousing   func() (width, height float64, present bool)
	MenuBarHeight   func() float64
	Raise           func(handle uintptr)
	Pin             func(handle uintptr, width, height float64)
	WatchPointer    func(func()) (stop func(), global bool)
	Preferences     func() QuickPlayerPreferences
	SavePreferences func(QuickPlayerPreferences)
}

// Activation is sent when the plugin's native windows are ready.
type Activation struct {
	Context   context.Context
	Main      *mygo.Window
	ColdStart bool
}

// Lifecycle handles entry to and departure from the active platform.
type Lifecycle struct {
	// Activate supplies the context and playback session access for this
	// activation. The host cancels the context before calling Deactivate.
	Activate        func(context.Context, PlaybackContinuity)
	Ready           func(Activation)
	Deactivate      func(switchingPlatform bool)
	MainFocused     func()
	PlaybackChanged func()
}

// Startup hands the host services to a platform's entry point.
type Startup struct {
	Host              Services
	Settings          SettingsStore
	RegisterWindow    func(WindowID, Window)
	RegisterMenu      func(MenuID, func() *mygo.Menu)
	RegisterLifecycle func(Lifecycle)
	MenuBarPanel      *MenuBarPanel
}

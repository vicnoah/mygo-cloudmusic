package app

import (
	"testing"

	"github.com/egoist/mygo"

	"waygu-music/internal/pluginapi"
)

// Queued app and tray menu clicks from a previous activation cannot change playback or desktop lyrics after switching back.
func TestNativeMenuActionsFollowActivation(t *testing.T) {
	a := newTestApp()
	owner := testRegistration("owner", func(s pluginapi.Startup) pluginapi.Adapter {
		s.RegisterMenu(pluginapi.ApplicationMenu, func() *mygo.Menu {
			return mygo.NewMenu([]*mygo.MenuItem{{Label: "Controls", Submenu: []*mygo.MenuItem{
				{Label: "Shuffle", Click: func(*mygo.MenuItem, *mygo.Window) { s.Host.Playback().ToggleShuffle() }},
			}}})
		})
		s.RegisterMenu(pluginapi.TrayMenu, func() *mygo.Menu {
			return mygo.NewMenu([]*mygo.MenuItem{
				{Label: "Lock lyrics", Click: func(*mygo.MenuItem, *mygo.Window) { s.Host.ToggleDesktopLyricsLock() }},
			})
		})
		return testPluginAdapter{}
	})
	a.platformRegistrations = []pluginapi.Registration{owner, testRegistration("other", nil)}
	a.initPlatformState()
	a.activatePlatformView()
	defer a.deactivatePlatformView(false)
	oldApp := a.platformMenu(pluginapi.ApplicationMenu)
	oldTray := a.trayMenu()
	a.setPlatform("other")
	a.setPlatform("owner")

	clickMenuAction(t, oldApp.Items(), "Shuffle")
	clickMenuAction(t, oldTray.Items(), "Lock lyrics")
	if a.pl.Shuffle() || a.desktopLyricsLocked {
		t.Fatal("an older activation's menu changed the selected platform")
	}
	clickMenuAction(t, a.platformMenu(pluginapi.ApplicationMenu).Items(), "Shuffle")
	clickMenuAction(t, a.trayMenu().Items(), "Lock lyrics")
	if !a.pl.Shuffle() || !a.desktopLyricsLocked {
		t.Fatal("the selected platform's menu actions did not take effect")
	}
}

func clickMenuAction(t *testing.T, items []*mygo.MenuItem, label string) {
	t.Helper()
	var find func([]*mygo.MenuItem) *mygo.MenuItem
	find = func(items []*mygo.MenuItem) *mygo.MenuItem {
		for _, item := range items {
			if item.Label == label && item.Click != nil {
				return item
			}
			if child := find(item.Submenu); child != nil {
				return child
			}
		}
		return nil
	}
	item := find(items)
	if item == nil {
		t.Fatalf("menu has no %q command", label)
	}
	item.Click(item, nil)
}

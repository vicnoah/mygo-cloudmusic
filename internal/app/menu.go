package app

import (
	"context"

	"github.com/egoist/mygo"

	"waygu-music/internal/pluginapi"
)

// setMenu gives the app its menu bar, in Chinese and named as the app
// is, with the settings in the app menu and the player's controls in a
// menu of their own.
func (a *app) setMenu() {
	mygo.App.SetMenu(a.platformMenu(pluginapi.ApplicationMenu))
}

func (a *app) platformMenu(id pluginapi.MenuID) *mygo.Menu {
	build := a.platformMenus[a.platformID][id]
	if build == nil {
		return nil
	}
	menu := build()
	if menu != nil && a.platformContext != nil {
		bindMenuActions(menu.Items(), a.platformContext)
	}
	return menu
}

// A native menu command that arrives after switching platforms belongs to the old activation.
func bindMenuActions(items []*mygo.MenuItem, activation context.Context) {
	for _, item := range items {
		if click := item.Click; click != nil {
			item.Click = func(item *mygo.MenuItem, window *mygo.Window) {
				if activation.Err() == nil {
					click(item, window)
				}
			}
		}
		bindMenuActions(item.Submenu, activation)
	}
}

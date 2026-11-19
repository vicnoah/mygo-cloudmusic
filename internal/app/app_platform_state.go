package app

import (
	"context"
	"github.com/egoist/mygo"
	appplatform "waygu-music/internal/pluginapi"
)

// Plugin registration and native window state.
type appPlatformState struct {
	platformID            appplatform.ID
	platformRegistrations []appplatform.Registration
	platforms             map[appplatform.ID]appplatform.Adapter
	platformWindows       map[appplatform.ID]map[appplatform.WindowID]appplatform.Window
	platformMenus         map[appplatform.ID]map[appplatform.MenuID]func() *mygo.Menu
	platformLifecycles    map[appplatform.ID]appplatform.Lifecycle
	mountedWindows        map[appplatform.WindowID]mountedWindow
	platformGeneration    uint64
	platformContext       context.Context
	cancelPlatform        context.CancelFunc
}

package app

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/waygu-music-plugin/sdk/platformrpc"
	appplatform "github.com/waygu-music-plugin/sdk/pluginapi"

	"waygu-music/internal/pluginregistry"
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
	pluginActivations     platformrpc.Activations
	platformScope         platformrpc.Scope
	platformAuthority     atomic.Pointer[platformAuthority]
	localPlugins          map[appplatform.ID]*localPlugin
	activeRemote          atomic.Pointer[pluginProcess]
	pluginRegistry        *pluginregistry.Registry
	pluginContext         context.Context
	cancelPlugins         context.CancelFunc
	managedPlugins        map[string]*managedPlugin
	pluginCatalogBusy     bool
	pluginCatalogRequest  uint64
	pluginCatalogError    string
	pluginWorkers         sync.WaitGroup
	catalogPackage        string
	pluginSource          pluginSourceEditor
	pluginLink            pluginLinkInstaller
}

type platformAuthority struct {
	id  appplatform.ID
	ctx context.Context
}

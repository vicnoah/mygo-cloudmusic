package app

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/waygu-music-plugin/sdk/pluginapi"
)

// A saved external plugin or a launch URL keeps the built-in root hidden and inactive on startup.
func TestStartupDefersBuiltinWindow(t *testing.T) {
	for _, tc := range []struct{ name, saved, repo string }{
		{name: "saved plugin", saved: "subsonic"},
		{name: "install link", saved: "fnmusic", repo: "waygu-music-plugin/subsonic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestApp()
			activated := false
			a.platformRegistrations = []pluginapi.Registration{testRegistration("fnmusic", func(s pluginapi.Startup) pluginapi.Adapter {
				s.RegisterLifecycle(pluginapi.Lifecycle{Activate: func(context.Context, pluginapi.PlaybackContinuity) {
					activated = true
				}})
				return testPluginAdapter{}
			})}
			a.managedPlugins = make(map[string]*managedPlugin)
			a.settings.Platform = tc.saved
			a.pluginLink.repo = tc.repo
			if !a.initPlatformState() {
				t.Fatal("missing built-in recovery window")
			}
			if options := a.startupWindowOptions(); !options.Hidden || activated || a.platformID != "" {
				t.Fatal("the built-in platform was shown or activated before startup was resolved")
			}
			if a.settings.Platform != tc.saved {
				t.Fatal("startup replaced the saved platform")
			}
		})
	}
}

// Explicitly choosing the built-in plugin from recovery replaces the saved external selection.
func TestStartupRecoveryUseBuiltin(t *testing.T) {
	a := newTestApp()
	a.platformRegistrations = []pluginapi.Registration{testRegistration("fnmusic", nil)}
	a.managedPlugins = make(map[string]*managedPlugin)
	a.settings.Platform = "subsonic"
	if !a.initPlatformState() {
		t.Fatal("missing built-in recovery window")
	}
	a.startupWindowOptions()
	a.pluginStartup.failed = "subsonic"
	a.pluginStartup.failure = "无法启动 Subsonic"
	view := ui.NewTester(a.pluginManagerView, 660, 485)
	if err := view.Click("使用飞牛音乐"); err != nil {
		t.Fatal(err)
	}
	if a.platformID != "fnmusic" || a.settings.Platform != "fnmusic" || a.pluginStartup.failure != "" {
		t.Fatal("the explicit fallback did not activate and save the built-in platform")
	}
}

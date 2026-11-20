package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/waygu-music-plugin/sdk/pluginapi"

	"waygu-music/internal/pluginregistry"
)

// Choosing a platform in the manager activates it and saves the selection for the next launch.
func TestPluginManagerSwitchPersistsSelection(t *testing.T) {
	a := newTestApp()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.platformRegistrations = []pluginapi.Registration{testRegistration("first", nil), testRegistration("second", nil)}
	if !a.initPlatformState() {
		t.Fatal("no platform registered")
	}
	a.activatePlatformView()
	t.Cleanup(func() { a.deactivatePlatformView(false) })

	tt := ui.NewTester(a.pluginManagerView, 520, 560)
	if err := tt.Click("启用 second"); err != nil {
		t.Fatal(err)
	}
	if a.platformID != "second" {
		t.Fatalf("the manager did not activate the selected platform: %q", a.platformID)
	}
	data, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved platformSettings
	if err := json.Unmarshal(data, &saved); err != nil || saved.Platform != "second" {
		t.Fatalf("the manager did not save the selected platform: %q, %v", saved.Platform, err)
	}
}

// Cancelling the source editor discards its draft, and saving adds the directory without changing existing sources.
func TestPluginManagerSavesAdditionalCatalog(t *testing.T) {
	a := newTestApp()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.managedPlugins = make(map[string]*managedPlugin)
	a.settings.PluginCatalogs = map[string]pluginCatalogSettings{"existing": {URL: "https://original.example/catalog.json"}}
	view := ui.NewTester(a.pluginManagerView, 810, 650)
	click := func(label string) {
		t.Helper()
		if err := view.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click("来源设置…")
	view.Type("https://cancel.example/catalog.json")
	click("取消")
	if a.settings.PluginCatalogURL != "" {
		t.Fatal("cancelling saved an unfinished source")
	}
	click("来源设置…")
	view.Type("https://repo.example/catalog.json")
	click("保存")
	data, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.PluginCatalogURL != "https://repo.example/catalog.json" || saved.PluginCatalogs["existing"].URL != "https://original.example/catalog.json" {
		t.Fatalf("adding a catalog changed the existing source: %+v", saved.platformSettings)
	}
}

// Enabling a package reuses the last connection and preserves the entry ID saved by Alpha.
func TestPluginPackageReusesLegacyConnection(t *testing.T) {
	a := newTestApp()
	a.localPlugins = make(map[pluginapi.ID]*localPlugin)
	a.settings.Platform = "jellyfin"
	pkg := &managedPlugin{id: "subsonic"}
	a.setInstalled(pkg, pluginregistry.Installed{Program: pluginregistry.Program{Path: "program", Version: pluginregistry.Version{
		Platforms: []string{"navidrome", "jellyfin", "gonic", "subsonic"},
	}}})
	if active := a.preferredPluginEntry(pkg); active != "jellyfin" || a.localPlugins[active].entry.LegacySettingsKey != "jellyfin" {
		t.Fatalf("saved profile was replaced: %q", active)
	}
	a.settings.Platform = "fnmusic"
	a.settings.LastPluginEntries = map[string]string{"subsonic": "navidrome"}
	if active := a.preferredPluginEntry(pkg); active != "navidrome" {
		t.Fatalf("activating Subsonic again did not restore its connection: %q", active)
	}
}

// Saving the proxy switch applies to every plugin, and cancelling leaves the stored choice intact.
func TestPluginManagerProxyPreference(t *testing.T) {
	a := newTestApp()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.managedPlugins = make(map[string]*managedPlugin)
	view := ui.NewTester(a.pluginManagerView, 810, 650)
	click := func(label string) {
		t.Helper()
		if err := view.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click("来源设置…")
	click("启用 GitHub 下载加速")
	click("保存")
	data, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.DisablePluginGitHubProxy || len(saved.PluginCatalogs) != 0 {
		t.Fatal("saving the switch did not disable the proxy or overwrote a catalog")
	}
	click("来源设置…")
	if a.pluginSource.useGitHubProxy {
		t.Fatal("the other plugin did not inherit the disabled proxy preference")
	}
	click("启用 GitHub 下载加速")
	click("取消")
	if !a.settings.DisablePluginGitHubProxy {
		t.Fatal("cancelling restored the proxy")
	}
}

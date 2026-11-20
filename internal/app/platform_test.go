package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/waygu-music-plugin/sdk/platformrpc"
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
	if err := tt.Click("使用 second"); err != nil {
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

// Editing a package source saves only that package and discards edits when cancelled.
func TestPluginManagerSourceEditorSavesSelectedPackage(t *testing.T) {
	a := newTestApp()
	dir := t.TempDir()
	a.settings.path = filepath.Join(dir, "settings.json")
	registry, err := pluginregistry.New(filepath.Join(dir, "registry"), nil)
	if err != nil {
		t.Fatal(err)
	}
	a.localPlugins = make(map[pluginapi.ID]*localPlugin)
	for _, id := range []string{"first", "second"} {
		entry := testRegistration(pluginapi.ID(id), nil)
		a.platformRegistrations = append(a.platformRegistrations, entry)
		a.localPlugins[entry.ID] = &localPlugin{entry: entry, registry: registry,
			program: platformrpc.Program{Identity: platformrpc.Identity{Package: id, Entry: id}}}
	}
	view := ui.NewTester(a.pluginManagerView, 810, 650)
	click := func(label string) {
		t.Helper()
		if err := view.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click("管理 second")
	click("来源设置…")
	view.Type("https://cancel.example/catalog.json")
	click("取消")
	if len(a.settings.PluginCatalogs) != 0 || a.localPlugins["second"].catalog != "" {
		t.Fatal("cancelling saved an unfinished source")
	}
	click("来源设置…")
	if !view.Focused("插件目录 URL") {
		t.Fatal("the new source editor did not focus its URL field")
	}
	view.Type("https://repo.example/catalog.json")
	click("保存来源")
	data, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatalf("source was not saved: %v (%s)", err, a.pluginSource.message)
	}
	var saved settings
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.PluginCatalogs) != 1 || saved.PluginCatalogs["second"] != (pluginCatalogSettings{URL: "https://repo.example/catalog.json"}) || a.pluginSource.open {
		t.Fatalf("source edit did not save the selected package and close the dialog: %+v", saved.PluginCatalogs)
	}
}

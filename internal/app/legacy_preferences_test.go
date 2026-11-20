package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
	"github.com/waygu-music-plugin/sdk/pluginapi"
)

// Plugins can opt into an old settings location and update it without changing each other's preferences.
func TestRegisteredLegacySettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	file := filepath.Join(dir, "settings.json")
	old := []byte(`{"platform":"named-service","volume":0.65,"quality":"lossless","normalize":false,"captions":false,"old-service-key":{"config":{"address":"https://example.test","username":"saved-user"},"quality":"standard"},"plugins":{"new-service":{"quality":"higher"}}}`)
	if err := os.WriteFile(file, old, 0o600); err != nil {
		t.Fatal(err)
	}

	var named, flat, modern pluginapi.Startup
	namedRegistration := testRegistration("named-service", func(start pluginapi.Startup) pluginapi.Adapter {
		named = start
		return testPluginAdapter{}
	})
	namedRegistration.LegacySettingsKey = "old-service-key"
	flatRegistration := testRegistration("flat-service", func(start pluginapi.Startup) pluginapi.Adapter {
		flat = start
		return testPluginAdapter{}
	})
	flatRegistration.LegacySettingsFields = []string{"quality", "normalize", "captions"}
	modernRegistration := testRegistration("new-service", func(start pluginapi.Startup) pluginapi.Adapter {
		modern = start
		return testPluginAdapter{}
	})

	a := newTestApp()
	a.platformRegistrations = []pluginapi.Registration{flatRegistration, namedRegistration, modernRegistration}
	a.settings = loadSettings(a.platformRegistrations...)
	if !a.initPlatformState() || a.platformID != "named-service" {
		t.Fatalf("startup selected %q", a.platformID)
	}
	type savedPreferences struct {
		Config struct {
			Address  string `json:"address"`
			Username string `json:"username"`
		} `json:"config,omitempty"`
		Quality string `json:"quality"`
	}
	type flatPreferences struct {
		Quality   string  `json:"quality"`
		Normalize bool    `json:"normalize"`
		Captions  bool    `json:"captions"`
		Volume    float64 `json:"volume"`
	}
	var namedPrefs, modernPrefs savedPreferences
	flatPrefs := flatPreferences{Normalize: true, Captions: true}
	for _, read := range []struct {
		store pluginapi.SettingsStore
		dst   any
	}{
		{named.Settings, &namedPrefs}, {flat.Settings, &flatPrefs}, {modern.Settings, &modernPrefs},
	} {
		if err := read.store.Load(read.dst); err != nil {
			t.Fatal(err)
		}
	}
	if namedPrefs.Quality != "standard" || namedPrefs.Config.Username != "saved-user" || modernPrefs.Quality != "higher" {
		t.Fatalf("loaded named=%+v, modern=%+v", namedPrefs, modernPrefs)
	}
	if flatPrefs.Quality != "lossless" || flatPrefs.Normalize || flatPrefs.Captions || flatPrefs.Volume != 0 {
		t.Fatalf("flat preferences unexpectedly loaded %+v", flatPrefs)
	}

	namedPrefs.Quality = "exhigh"
	if err := named.Settings.Save(namedPrefs); err != nil {
		t.Fatal(err)
	}
	flatPrefs.Normalize = true
	flatPrefs.Volume = 0.01
	if err := flat.Settings.Save(flatPrefs); err != nil {
		t.Fatal(err)
	}
	if a.settings.Volume != 0.65 || !a.settings.Normalize {
		t.Fatalf("unexpected shared audio settings: volume=%v normalize=%v", a.settings.Volume, a.settings.Normalize)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Named     savedPreferences                  `json:"old-service-key"`
		Plugins   map[pluginapi.ID]savedPreferences `json:"plugins"`
		Quality   string                            `json:"quality"`
		Normalize bool                              `json:"normalize"`
		Captions  bool                              `json:"captions"`
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Named.Quality != "exhigh" || saved.Named.Config.Address != "https://example.test" ||
		saved.Plugins["new-service"].Quality != "higher" || saved.Quality != "lossless" || !saved.Normalize || saved.Captions {
		t.Fatalf("saving touched the wrong settings or lost legacy settings: %+v", saved)
	}
}

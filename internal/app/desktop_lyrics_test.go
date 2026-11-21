package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
	"github.com/waygu-music-plugin/sdk/bridge"
	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/protocol"
)

// A legacy taskbar font becomes the shared font and stays cleared after a reset.
func TestSharedLyricsFontMigratesAndResets(t *testing.T) {
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"taskbarLyricsFontFamily":"Noto Sans"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	a := newTestApp()
	a.settings = loadSettings()
	h := platformHost{a: a}
	if got := h.DesktopLyricsState().FontFamily; got != "Noto Sans" {
		t.Fatalf("desktop lyrics use %q after migration", got)
	}
	if got := h.DesktopSettings().TaskbarLyricsFontFamily; got != "Noto Sans" {
		t.Fatalf("legacy plugin snapshot contains %q after migration", got)
	}
	if err := a.setLyricsFontFamily(""); err != nil {
		t.Fatal(err)
	}
	if got := loadSettings().LyricsFontFamily; got != "" {
		t.Fatalf("the old font %q came back after resetting", got)
	}
}

// Desktop lyrics keep style, lock state, and the dragged position in settings across runs.
func TestDesktopLyricsSettingsRememberStyle(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	prefs := pluginapi.DefaultDesktopLyricsPreferences()
	prefs.PositionSet, prefs.X, prefs.Y = true, 123, 456
	a.settings.DesktopLyrics = &prefs
	h := platformHost{a: a}
	h.ToggleDesktopLyricsLock()
	h.SetDesktopLyricsStyle(false, 36, 5)

	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var got settings
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.DesktopLyrics == nil {
		t.Fatal("desktop lyric settings were not saved")
	}
	if got.DesktopLyrics.Locked != true || got.DesktopLyrics.ShowTranslation || got.DesktopLyrics.FontSize != 36 || got.DesktopLyrics.Color != 5 {
		t.Fatalf("desktop lyric style was not saved: %+v", *got.DesktopLyrics)
	}
	if !got.DesktopLyrics.PositionSet || got.DesktopLyrics.X != 123 || got.DesktopLyrics.Y != 456 {
		t.Fatalf("desktop lyric position was not saved: %+v", *got.DesktopLyrics)
	}
}

// A late lyric preference write from a previous plugin activation cannot replace the host settings.
func TestDesktopLyricsPreferencesFollowActivation(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	previous := a.pluginActivations.Prepare("old connection")
	a.pluginActivations.Commit(previous)
	current := a.pluginActivations.Prepare("current connection")
	a.pluginActivations.Commit(current)
	prefs := desktopLyricsSettings{
		Locked: true, FontSize: 38, ShowTranslation: false, Color: 5,
		X: 120, Y: 210, PositionSet: true,
	}
	in := PluginHostCall{
		Scope: current, Action: protocol.HostSaveDesktopLyrics, DesktopLyrics: bridge.WireDesktopLyricsPrefs(prefs),
	}
	reverse := pluginReverse{app: a, connection: current.Connection}
	if err := reverse.apply(bridge.LocalHostCall(in.Wire()), &PluginHostReply{}); err != nil {
		t.Fatal(err)
	}
	stale := pluginReverse{app: a, connection: previous.Connection}
	in.Scope, in.DesktopLyrics = previous, bridge.WireDesktopLyricsPrefs(pluginapi.DefaultDesktopLyricsPreferences())
	err := stale.apply(bridge.LocalHostCall(in.Wire()), &PluginHostReply{})
	var failure protocol.Failure
	if !errors.As(err, &failure) || failure.Code != "stale-activation" {
		t.Fatalf("a late preference write returned %v", err)
	}
	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.DesktopLyrics == nil || *saved.DesktopLyrics != prefs || !saved.DesktopLyricsLocked {
		t.Fatalf("saved settings did not retain the current activation's changes: %+v", saved.DesktopLyrics)
	}
}

// Legacy plugin preference writes preserve the custom color until it is explicitly reset.
func TestLegacyDesktopLyricsPreferencesPreserveCustomColor(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	scope := a.pluginActivations.Prepare("current connection")
	a.pluginActivations.Commit(scope)
	reverse := pluginReverse{app: a, connection: scope.Connection}
	apply := func(in PluginHostCall) error {
		in.Scope = scope
		return reverse.apply(bridge.LocalHostCall(in.Wire()), &PluginHostReply{})
	}
	if err := apply(PluginHostCall{Action: protocol.HostSetDesktopLyricsColor, Text: "#3AD2EF"}); err != nil {
		t.Fatal(err)
	}
	prefs := pluginapi.DefaultDesktopLyricsPreferences()
	prefs.Locked, prefs.FontSize, prefs.ShowTranslation = true, 38, false
	if err := apply(PluginHostCall{Action: protocol.HostSaveDesktopLyrics, DesktopLyrics: bridge.WireDesktopLyricsPrefs(prefs)}); err != nil {
		t.Fatal(err)
	}
	readSaved := func() settings {
		t.Helper()
		data, err := os.ReadFile(a.settings.path)
		if err != nil {
			t.Fatal(err)
		}
		var saved settings
		if err := json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		return saved
	}
	if saved := readSaved(); saved.DesktopLyricsColor != "#3ad2ef" {
		t.Fatalf("legacy preferences replaced the custom color: %q", saved.DesktopLyricsColor)
	}
	if err := apply(PluginHostCall{Action: protocol.HostSetDesktopLyricsColor, Text: "#f00"}); err == nil {
		t.Fatal("the host accepted an invalid color")
	}
	if err := apply(PluginHostCall{Action: protocol.HostSetDesktopLyricsColor}); err != nil {
		t.Fatal(err)
	}
	if saved := readSaved(); saved.DesktopLyricsColor != "" || saved.DesktopLyrics == nil || *saved.DesktopLyrics != prefs {
		t.Fatalf("reset did not preserve the other lyric settings: %+v", saved.DesktopLyrics)
	}
}

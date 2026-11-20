package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/waygu-music-plugin/sdk/bridge"
	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/protocol"
)

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

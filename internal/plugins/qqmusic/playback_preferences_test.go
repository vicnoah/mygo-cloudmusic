package qqmusic

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/plugins/internal/plugintest"
)

// Changing QQ normalization updates only its plugin preference and playback.
func TestQQSettingsStayInPlatformNamespace(t *testing.T) {
	h := plugintest.NewHost(ID)
	prefs := &platformPreferences{Quality: "standard"}
	saves := 0
	h.Save = func() { saves++ }
	adapter := h.Start(Registration(), prefs)
	audio := adapter.PlaybackMedia().Preferences
	tt := ui.NewTester(h.Windows[pluginapi.SettingsWindow].Root, 560, 1200)
	tt.Frame()
	label, haveLabel := tt.Find("音量均衡")
	// Both the row title and the switch expose this name.
	if !haveLabel {
		t.Fatal("cannot locate the normalization control")
	}
	tt.ClickAt(560-28-16-12, label.Y+label.H)
	tt.Frame()
	if err := h.Preferences.Load(prefs); err != nil {
		t.Fatal(err)
	}
	if prefs.Normalize == nil || *prefs.Normalize || audio().Normalize || saves != 1 {
		t.Fatal("the setting did not disable normalization and save QQ preferences")
	}
	if audio().Quality != "standard" {
		t.Fatal("updating normalization changed the audio quality")
	}
}

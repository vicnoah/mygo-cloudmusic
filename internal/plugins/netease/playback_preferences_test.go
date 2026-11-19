package netease

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/plugins/internal/plugintest"
)

// The NetEase normalization switch updates playback and the plugin's save callback.
func TestNetEaseUpdatesPlaybackPreferences(t *testing.T) {
	h := plugintest.NewHost(ID)
	prefs := &platformPreferences{Quality: "lossless", Normalize: false, Captions: false}
	saves := 0
	h.Save = func() { saves++ }
	adapter := h.Start(Registration(), prefs)
	h.Activate(t)
	audio := adapter.PlaybackMedia().Preferences
	if settings := audio(); settings.Quality != "lossless" || settings.Normalize {
		t.Fatal("the player did not receive the saved audio preferences")
	}
	tt := ui.NewTester(h.Windows[pluginapi.SettingsWindow].Root, 560, 1200)
	tt.Frame()
	label, foundLabel := tt.Find("音量均衡")
	description, foundDescription := tt.Find("按网易云音乐测得的响度调整每首歌的音量，让歌曲听起来一样响")
	if !foundLabel || !foundDescription {
		t.Fatal("cannot locate the normalization control")
	}
	tt.ClickAt(560-28-16-12, (label.Y+description.Y+description.H)/2)
	tt.Frame()
	if !audio().Normalize || saves != 1 {
		t.Fatal("the settings switch did not enable normalization")
	}
	if err := h.Preferences.Load(prefs); err != nil {
		t.Fatal(err)
	}
	if !prefs.Normalize || prefs.Quality != "lossless" || prefs.Captions {
		t.Fatalf("the updated preferences were not scoped to normalization: %+v", prefs)
	}
}

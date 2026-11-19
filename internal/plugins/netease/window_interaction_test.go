package netease

import (
	"waygu-music/internal/plugins/internal/plugintest"
	"testing"
)

// Font and lock controls affect only an unlocked desktop lyrics window under the pointer.
func TestDesktopLyricsToolbar(t *testing.T) {
	plugintest.CheckDesktopLyricsToolbar(t, plugintest.Start(t, Registration()))
}

// Clicking the settings version sends each click to the developer-mode service.
func TestSettingsVersionClicks(t *testing.T) {
	plugintest.CheckSettingsVersionClicks(t, plugintest.Start(t, Registration()))
}

package songloft

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

// Space controls playback when the Now Playing button still has focus.
func TestSpaceAfterOpeningNowPlaying(t *testing.T) {
	plugintest.CheckSpaceAfterOpeningNowPlaying(t, plugintest.Start(t, Registration()))
}

// Dragging the progress bar defers seeking until the pointer is released.
func TestScrubSeeksOnRelease(t *testing.T) {
	plugintest.CheckScrubSeeksOnRelease(t, plugintest.Start(t, Registration()))
}

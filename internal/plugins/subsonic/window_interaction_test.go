package subsonic

import (
	"waygu-music/internal/plugins/internal/plugintest"
	"testing"
)

// Font and lock controls affect only an unlocked desktop lyrics window under the pointer.
func TestDesktopLyricsToolbar(t *testing.T) {
	for _, entry := range Platforms() {
		t.Run(string(entry.ID), func(t *testing.T) {
			plugintest.CheckDesktopLyricsToolbar(t, plugintest.Start(t, entry))
		})
	}
}

// Clicking the settings version sends each click to the developer-mode service.
func TestSettingsVersionClicks(t *testing.T) {
	for _, entry := range Platforms() {
		t.Run(string(entry.ID), func(t *testing.T) {
			plugintest.CheckSettingsVersionClicks(t, plugintest.Start(t, entry))
		})
	}
}

// Space controls playback when the Now Playing button still has focus.
func TestSpaceAfterOpeningNowPlaying(t *testing.T) {
	for _, entry := range Platforms() {
		t.Run(string(entry.ID), func(t *testing.T) {
			plugintest.CheckSpaceAfterOpeningNowPlaying(t, plugintest.Start(t, entry))
		})
	}
}

// Dragging the progress bar defers seeking until the pointer is released.
func TestScrubSeeksOnRelease(t *testing.T) {
	for _, entry := range Platforms() {
		t.Run(string(entry.ID), func(t *testing.T) {
			plugintest.CheckScrubSeeksOnRelease(t, plugintest.Start(t, entry))
		})
	}
}

// Activating the plugin again resets navigation to its landing page.
func TestActivationRestoresLandingPage(t *testing.T) {
	for _, entry := range Platforms() {
		t.Run(string(entry.ID), func(t *testing.T) {
			h := plugintest.NewHost(entry.ID)
			adapter := h.Start(entry, nil).(*Adapter)
			h.Activate(t)
			adapter.Home().Open(Route{Page: PageSearch, Query: "example"})
			if got := adapter.Home().Route(); got.Page != PageSearch {
				t.Fatalf("plugin did not navigate to search: %+v", got)
			}
			h.Lifecycle.Deactivate(true)
			h.Activate(t)
			if got := adapter.Home().Route(); got.Page != PageLibrary {
				t.Fatalf("activated with route %+v", got)
			}
		})
	}
}

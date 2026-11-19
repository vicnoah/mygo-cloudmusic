package builtin

import (
	"testing"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/plugins/fnmusic"
	"waygu-music/internal/plugins/netease"
	"waygu-music/internal/plugins/qqmusic"
	"waygu-music/internal/plugins/songloft"
	"waygu-music/internal/plugins/subsonic"
)

// The built-in selector starts on Feiniu and preserves the original switch order and developer-mode entries.
func TestBuiltinPlatformOrder(t *testing.T) {
	entries := Platforms()
	want := [...]pluginapi.ID{fnmusic.ID, subsonic.NavidromeID, subsonic.JellyfinID, subsonic.GonicID, subsonic.ID, songloft.ID, netease.ID, qqmusic.ID}
	if len(entries) != len(want) {
		t.Fatalf("built-in entries = %d, want %d", len(entries), len(want))
	}
	for i, entry := range entries {
		if entry.ID != want[i] {
			t.Fatalf("entry %d = %q, want %q", i, entry.ID, want[i])
		}
		if entry.DeveloperOnly != (entry.ID == netease.ID || entry.ID == qqmusic.ID) {
			t.Fatalf("developer mode requirement changed for %q", entry.ID)
		}
	}
}

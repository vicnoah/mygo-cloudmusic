package fnmusic

import (
	"encoding/json"
	"strings"
	"testing"
)

// The generic runtime seam round-trips connection and transcode settings without exposing backend types.
func TestRuntimeGenericSettings(t *testing.T) {
	settings := &platformPreferences{TranscodeMode: transcodeAuto}
	rt := NewRuntime(NewState(), settings)
	want := connectionConfig{Address: "music.local", Username: "alice", Mode: "address", HTTPS: true}
	rt.SetConfig(want)
	if got := rt.Config(); got != want {
		t.Fatalf("config = %#v, want %#v", got, want)
	}
	rt.SetTranscode(transcodeSettings{Mode: transcodeAlways, Bitrate: 192})
	if got := rt.Transcode(); got.Mode != transcodeAlways || got.Bitrate != 192 {
		t.Fatalf("transcode = %#v", got)
	}
	b, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) == "" || string(b) == "{}" {
		t.Fatalf("settings did not serialize: %s", b)
	}
	if strings.Contains(string(b), "passHash") || strings.Contains(string(b), "accessCode") {
		t.Fatalf("secret connection fields leaked into settings: %s", b)
	}
}

// Sort preferences are persisted per page and survive a reload.
func TestRuntimeSortPreferences(t *testing.T) {
	settings := &platformPreferences{}
	rt := NewRuntime(NewState(), settings)
	rt.SetSort(sortKeyAlbums, "releaseYear,desc")
	if got := rt.Sorts()[sortKeyAlbums]; got != "releaseYear,desc" {
		t.Fatalf("stored sort = %q", got)
	}
	next := NewRuntime(NewState(), settings)
	if got := next.Sorts()[sortKeyAlbums]; got != "releaseYear,desc" {
		t.Fatalf("reloaded sort = %q", got)
	}
}

package fnmusic

import (
	"testing"

	backend "netease-music-mygo/internal/platform/fnmusic"
)

// Attaching and detaching a session clears account state instead of leaking it to the next login.
func TestStateSessionLifecycle(t *testing.T) {
	s := NewState()
	c, err := backend.New(backend.Config{Address: "127.0.0.1:1", Username: "u", PassHash: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Attach(c); err != nil {
		t.Fatal(err)
	}
	s.SetLiked(7, true)
	s.SetPlaylists(nil)
	s.SetRoamID("r1")
	if !s.Connected() || !s.Liked(7) || s.RoamID() != "r1" || s.Provider() == nil {
		t.Fatal("attached state was not published")
	}
	if got := s.Detach(); got != c {
		t.Fatal("Detach returned a different client")
	}
	if s.Connected() || s.Liked(7) || s.RoamID() != "" || s.Provider() != nil {
		t.Fatal("detached state retained session data")
	}
}

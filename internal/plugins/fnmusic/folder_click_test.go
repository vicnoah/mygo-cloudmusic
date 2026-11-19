package fnmusic

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	backend "waygu-music/internal/plugins/fnmusic/internal/client"
)

// Clicking a folder row opens it as its own page, and Back returns to the folder it was in.
func TestFolderRowOpensFolderAndBackReturns(t *testing.T) {
	s := NewState()
	client, err := backend.New(backend.Config{Address: "127.0.0.1:1", Username: "u", PassHash: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Attach(client); err != nil {
		t.Fatal(err)
	}
	s.capabilities, s.capabilitiesKnown = backend.FolderCapabilities{FolderView: true}, true
	h := NewHome(s, nopHost{})
	f := h.syncFolders()
	f.roots = []backend.SharedLibrary{{Name: "音乐", Path: "/music"}}
	f.pages["/music"] = backend.FolderPage{Name: "music", Path: "/music", Folders: []backend.FolderNode{{Name: "Adele", Path: "/music/Adele", ItemCount: 2}}}
	f.pages["/music/Adele"] = backend.FolderPage{Name: "Adele", Path: "/music/Adele", Tracks: []backend.FolderTrack{{Track: pluginapi.Track{Name: "Hello"}}}}
	h.ResetNavigation(Route{Page: PageFolders, Query: "/music"})

	tt := ui.NewTester(h.Content, 1000, 900)
	tt.Frame()
	if err := tt.Click("Adele"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := h.Route(); got.Page != PageFolders || got.Query != "/music/Adele" {
		t.Fatalf("after click: route = %#v", got)
	}
	if _, ok := tt.Find("Hello"); !ok {
		t.Fatal("the opened folder does not list its song")
	}
	h.Back()
	tt.Frame()
	if got := h.Route(); got.Query != "/music" {
		t.Fatalf("after back: route = %#v", got)
	}
}

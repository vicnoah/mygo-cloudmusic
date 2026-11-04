package subsonic

import (
	"encoding/json"
	"strings"
	"testing"

	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/subsonic"
)

// Tests that the Subsonic runtime configures and persists settings without leaking passwords to json.
func TestSubsonicRuntimeSettings(t *testing.T) {
	settings := &appplatform.PlatformSettings{}
	state := NewState()
	rt := NewRuntime(state, settings)

	want := appplatform.ConnectionConfig{
		Address:  "http://172.16.1.10:4533",
		Username: "wwg",
		PassHash: "secret123",
	}
	rt.SetConfig(want)

	got := rt.Config()
	if got.Address != want.Address || got.Username != want.Username {
		t.Fatalf("config = %#v, want %#v", got, want)
	}

	b, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "secret123") {
		t.Fatalf("password leaked into JSON: %s", b)
	}
}

// Tests that Adapter.OwnsTrack correctly claims only Subsonic tracks.
func TestSubsonicAdapterOwnsTrack(t *testing.T) {
	adapter := NewAdapter(nil, backend.ProfileSubsonic, nil)
	naviAdapter := NewAdapter(nil, backend.ProfileNavidrome, nil)

	subTrack := &netease.Track{Source: backend.Source, ID: -3123}
	naviTrack := &netease.Track{Source: "navidrome", ID: -3124}
	neteaseTrack := &netease.Track{Source: "", ID: 12345}
	fnTrack := &netease.Track{Source: "fn", ID: -1234}

	if !adapter.OwnsTrack(subTrack) {
		t.Errorf("expected OwnsTrack(subTrack) to be true")
	}
	if !naviAdapter.OwnsTrack(naviTrack) {
		t.Errorf("expected naviAdapter.OwnsTrack(naviTrack) to be true")
	}
	if !naviAdapter.OwnsTrack(subTrack) {
		t.Errorf("expected naviAdapter.OwnsTrack(subTrack) to be true")
	}
	if adapter.OwnsTrack(neteaseTrack) {
		t.Errorf("expected OwnsTrack(neteaseTrack) to be false")
	}
	if adapter.OwnsTrack(fnTrack) {
		t.Errorf("expected OwnsTrack(fnTrack) to be false")
	}
}

// Tests navigation history and back/forward stack in Home.
func TestSubsonicNavigation(t *testing.T) {
	home := NewHome(NewState(), nil)

	if home.CanBack() || home.CanForward() {
		t.Fatalf("expected empty history at init")
	}
	if home.Route().Page != PageLibrary {
		t.Fatalf("expected initial page %q, got %q", PageLibrary, home.Route().Page)
	}

	home.Open(Route{Page: PageAlbums})
	if home.Route().Page != PageAlbums {
		t.Fatalf("expected page %q, got %q", PageAlbums, home.Route().Page)
	}
	if !home.CanBack() {
		t.Fatalf("expected CanBack() to be true")
	}

	home.Back()
	if home.Route().Page != PageLibrary {
		t.Fatalf("expected back to %q, got %q", PageLibrary, home.Route().Page)
	}
	if !home.CanForward() {
		t.Fatalf("expected CanForward() to be true")
	}

	home.Forward()
	if home.Route().Page != PageAlbums {
		t.Fatalf("expected forward to %q, got %q", PageAlbums, home.Route().Page)
	}

	home.ResetNavigation(Route{Page: PageArtists})
	if home.CanBack() || home.CanForward() {
		t.Fatalf("expected history to be cleared after ResetNavigation")
	}
	if home.Route().Page != PageArtists {
		t.Fatalf("expected page %q, got %q", PageArtists, home.Route().Page)
	}
}

// Tests that account menu offers Settings in both disconnected and connected states.
func TestSubsonicAccountActions(t *testing.T) {
	state := NewState()
	home := NewHome(state, nil)

	actions := home.accountActions()
	hasSettings := false
	for _, a := range actions {
		if a.Label == "设置…" {
			hasSettings = true
			break
		}
	}
	if !hasSettings {
		t.Fatalf("disconnected accountActions missing '设置…': %#v", actions)
	}

	client := backend.NewClient(backend.Config{})
	client.SetConnectedForTest(true)
	state.SetClient(client)
	actions = home.accountActions()
	hasSettings = false
	for _, a := range actions {
		if a.Label == "设置…" {
			hasSettings = true
			break
		}
	}
	if !hasSettings {
		t.Fatalf("connected accountActions missing '设置…': %#v", actions)
	}
}

// Tests that ListForRoute resolves artist and album links when opened from a track row.
func TestSubsonicArtistAlbumResolution(t *testing.T) {
	state := NewState()
	catalog := state.Catalog()

	// Simulate track with album and artist
	artistID := backend.RegisterMeta(backend.ProfileNavidrome.ID, "artist-guid-1", "artist", "Taylor Swift", "")
	albumID := backend.RegisterMeta(backend.ProfileNavidrome.ID, "album-guid-1", "album", "1989", "cover-1")

	// 1. Artist link click in track row (opened via route with Page: "artist", ID: artistID, Kind: "")
	artistList := catalog.ListForRoute(artistID, "", "")
	if artistList == nil {
		t.Fatalf("ListForRoute(artistID) returned nil")
	}
	if artistList.Spec.Kind != ListArtist || artistList.Spec.Ref != "artist-guid-1" || artistList.Name != "Taylor Swift" {
		t.Fatalf("resolved artist list = %#v, want Kind=ListArtist, Ref=artist-guid-1, Name=Taylor Swift", artistList)
	}

	// 2. Album link click in track row (opened via route with Page: "album", ID: albumID, Kind: "")
	albumList := catalog.ListForRoute(albumID, "", "")
	if albumList == nil {
		t.Fatalf("ListForRoute(albumID) returned nil")
	}
	if albumList.Spec.Kind != ListAlbum || albumList.Spec.Ref != "album-guid-1" || albumList.Name != "1989" {
		t.Fatalf("resolved album list = %#v, want Kind=ListAlbum, Ref=album-guid-1, Name=1989", albumList)
	}
}

// Tests that album sort choices include all expected sorting policies.
func TestSubsonicAlbumSortOptions(t *testing.T) {
	choices := albumSortChoices()
	var foundStarred, foundYear bool
	for _, c := range choices {
		if c.Value == "starred" {
			foundStarred = true
		}
		if c.Value == "byYear" {
			foundYear = true
		}
	}
	if !foundStarred || !foundYear {
		t.Fatalf("expected starred and byYear in album sort choices, got: %#v", choices)
	}
}

// Tests that State records album liked state in memory and handles toggling correctly.
func TestStateAlbumFavorite(t *testing.T) {
	state := NewState()
	if state.AlbumLiked("al-test-1") {
		t.Fatalf("expected al-test-1 not to be liked initially")
	}

	state.SetAlbumLikedMemory("al-test-1", true)
	if !state.AlbumLiked("al-test-1") {
		t.Fatalf("expected al-test-1 to be liked after SetAlbumLikedMemory")
	}

	state.SetAlbumLikedMemory("al-test-1", false)
	if state.AlbumLiked("al-test-1") {
		t.Fatalf("expected al-test-1 not to be liked after unliking")
	}
}

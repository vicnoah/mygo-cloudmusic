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
		Address:  "http://subsonic.example.com",
		Username: "testuser",
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

// Tests that Subsonic RecentStore records played songs up to 200 and supports removal and clear.
func TestSubsonicRecentStore(t *testing.T) {
	store := &RecentStore{
		path: t.TempDir() + "/test_subsonic_recent.json",
	}

	tr1 := netease.Track{ID: 201, Name: "Sub Song 1"}
	tr2 := netease.Track{ID: 202, Name: "Sub Song 2"}
	store.Add(tr1)
	store.Add(tr2)

	if store.Total() != 2 {
		t.Fatalf("expected 2 tracks, got %d", store.Total())
	}
	tracks, total := store.Slice(0, 10)
	if total != 2 || len(tracks) != 2 {
		t.Fatalf("expected slice length 2, got %d", len(tracks))
	}
	if tracks[0].ID != 202 || tracks[1].ID != 201 {
		t.Fatalf("unexpected order: %v, %v", tracks[0].ID, tracks[1].ID)
	}

	// Move to top
	store.Add(tr1)
	tracks, _ = store.Slice(0, 10)
	if tracks[0].ID != 201 || tracks[1].ID != 202 {
		t.Fatalf("expected tr1 at top, got %v", tracks[0].ID)
	}

	// Remove
	store.Remove(201)
	if store.Total() != 1 {
		t.Fatalf("expected 1 track, got %d", store.Total())
	}

	// Clear
	store.Clear()
	if store.Total() != 0 {
		t.Fatalf("expected 0 tracks, got %d", store.Total())
	}
}

// Tests that Catalog resolves recent list for backend.RecentID in Subsonic.
func TestSubsonicRecentCatalog(t *testing.T) {
	cat := NewCatalog(NewState(), nil)
	list := cat.ListForRoute(backend.RecentID, "", "")
	if list == nil {
		t.Fatal("expected recent list not nil")
	}
	if list.Spec.Kind != ListRecent || list.Name != "最近播放" {
		t.Fatalf("unexpected recent list: kind=%s, name=%s", list.Spec.Kind, list.Name)
	}
}

// Tests that RecentStore uses separate storage files for distinct platform profiles.
func TestSubsonicProfileRecentFiles(t *testing.T) {
	navStore := NewRecentStore("navidrome")
	jelStore := NewRecentStore("jellyfin")
	gonStore := NewRecentStore("gonic")
	subStore := NewRecentStore("subsonic")

	if !strings.HasSuffix(navStore.path, "navidrome_recent.json") {
		t.Fatalf("unexpected navStore path: %s", navStore.path)
	}
	if !strings.HasSuffix(jelStore.path, "jellyfin_recent.json") {
		t.Fatalf("unexpected jelStore path: %s", jelStore.path)
	}
	if !strings.HasSuffix(gonStore.path, "gonic_recent.json") {
		t.Fatalf("unexpected gonStore path: %s", gonStore.path)
	}
	if !strings.HasSuffix(subStore.path, "subsonic_recent.json") {
		t.Fatalf("unexpected subStore path: %s", subStore.path)
	}

	// Verify catalog initialized with profile state gets corresponding recent file
	navState := NewState(backend.ProfileNavidrome)
	if navState.Catalog() == nil || navState.Catalog().Recent == nil {
		t.Fatalf("expected navState to have catalog and recent store")
	}
	if !strings.HasSuffix(navState.Catalog().Recent.path, "navidrome_recent.json") {
		t.Fatalf("expected navidrome_recent.json, got: %s", navState.Catalog().Recent.path)
	}
}

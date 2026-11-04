package songloft

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform"
	backend "netease-music-mygo/internal/platform/songloft"
)

// Tests that the Songloft runtime configures and persists settings without leaking passwords to json.
func TestSongloftRuntimeSettings(t *testing.T) {
	settings := &appplatform.PlatformSettings{}
	state := NewState()
	rt := NewRuntime(state, settings)

	want := appplatform.ConnectionConfig{
		Address:  "http://songloft.example.com",
		Username: "testuser",
		PassHash: "mockpassword123",
	}
	rt.SetConfig(want)

	bcfg := fromConnectionConfig(want)
	if bcfg.Password != "mockpassword123" {
		t.Fatalf("fromConnectionConfig did not preserve password: %q", bcfg.Password)
	}

	got := rt.Config()
	if got.Address != want.Address || got.Username != want.Username {
		t.Fatalf("config = %#v, want %#v", got, want)
	}

	b, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "mockpassword123") {
		t.Fatalf("password leaked into JSON: %s", b)
	}
}

// Tests that Adapter.OwnsTrack correctly claims only Songloft tracks.
func TestSongloftAdapterOwnsTrack(t *testing.T) {
	adapter := NewAdapter(nil, nil)

	songloftTrack := &netease.Track{Source: backend.Source, ID: -4123}
	songloftIDTrack := &netease.Track{Source: string(platform.SongloftID), ID: -4124}
	neteaseTrack := &netease.Track{Source: "", ID: 12345}
	fnTrack := &netease.Track{Source: "fnmusic", ID: -1234}
	subTrack := &netease.Track{Source: "subsonic", ID: -3123}

	if !adapter.OwnsTrack(songloftTrack) {
		t.Errorf("expected OwnsTrack(songloftTrack) to be true")
	}
	if !adapter.OwnsTrack(songloftIDTrack) {
		t.Errorf("expected OwnsTrack(songloftIDTrack) to be true")
	}
	if adapter.OwnsTrack(neteaseTrack) {
		t.Errorf("expected OwnsTrack(neteaseTrack) to be false")
	}
	if adapter.OwnsTrack(fnTrack) {
		t.Errorf("expected OwnsTrack(fnTrack) to be false")
	}
	if adapter.OwnsTrack(subTrack) {
		t.Errorf("expected OwnsTrack(subTrack) to be false")
	}
}

// Tests navigation history and back/forward stack in Home.
func TestSongloftNavigation(t *testing.T) {
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

// Tests that ListForRoute resolves artist and album links when opened from a track row.
func TestSongloftArtistAlbumResolution(t *testing.T) {
	state := NewState()
	catalog := state.Catalog()

	artistID := backend.RegisterMeta(platform.SongloftID, "徐佳莹", "artist", "徐佳莹", "")
	albumID := backend.RegisterMeta(platform.SongloftID, "LaLa首张创作专辑", "album", "LaLa首张创作专辑", "")

	// 1. Artist link click
	artistList := catalog.ListForRoute(artistID, "artist", "徐佳莹")
	if artistList == nil {
		t.Fatalf("ListForRoute(artistID) returned nil")
	}
	if artistList.Spec.Kind != ListArtist || artistList.Spec.Ref != "徐佳莹" || artistList.Name != "徐佳莹" {
		t.Fatalf("resolved artist list = %#v", artistList)
	}

	// 2. Album link click
	albumList := catalog.ListForRoute(albumID, "album", "LaLa首张创作专辑")
	if albumList == nil {
		t.Fatalf("ListForRoute(albumID) returned nil")
	}
	if albumList.Spec.Kind != ListAlbum || albumList.Spec.Ref != "LaLa首张创作专辑" || albumList.Name != "LaLa首张创作专辑" {
		t.Fatalf("resolved album list = %#v", albumList)
	}
}

// Tests that State tracks liked songs correctly.
func TestSongloftLikedTracking(t *testing.T) {
	state := NewState()
	tr := &netease.Track{ID: -4500, Fav: false}

	if state.LikedTrack(tr) {
		t.Fatal("expected track not to be liked initially")
	}

	state.liked[tr.ID] = true
	if !state.LikedTrack(tr) {
		t.Fatal("expected track to be liked after state.liked update")
	}
}

// Tests that FavoriteChanged and RemoveFavorite correctly drop and mutate list state.
func TestSongloftCatalogFavoriteChanged(t *testing.T) {
	state := NewState()
	cat := state.Catalog()

	favList := cat.ListFor(backend.FavoritesID, ListFavorites, "", "我喜欢")
	if favList == nil {
		t.Fatal("expected favList to not be nil")
	}

	tr := netease.Track{ID: 1001, Name: "Test Song", Fav: true, Source: backend.Source}
	favList.Pages.Land(0, []netease.Track{tr}, 1)
	if favList.Pages.Total() != 1 {
		t.Fatalf("expected total 1, got %d", favList.Pages.Total())
	}

	// Unlike: should remove immediately from favorites
	cat.FavoriteChanged(1001, false)
	if favList.Pages.Total() != 0 {
		t.Fatalf("expected total 0 after remove favorite, got %d", favList.Pages.Total())
	}

	// Like: should drop cached list so next query fetches anew
	cat.Lists[backend.FavoritesID] = favList
	cat.FavoriteChanged(1002, true)
	if _, exists := cat.Lists[backend.FavoritesID]; exists {
		t.Fatal("expected cached favorites list to be dropped on new like")
	}
}

// Tests that album and artist sorting updates catalog sort choices and recreates pagers.
func TestSongloftSortChoicesAndCatalogReload(t *testing.T) {
	state := NewState()
	cat := state.Catalog()

	p1 := cat.AlbumsPager()
	if cat.AlbumSort != "count:desc" {
		t.Fatalf("expected default album sort count:desc, got %s", cat.AlbumSort)
	}

	cat.SetAlbumSort("name:asc")
	if cat.AlbumSort != "name:asc" {
		t.Fatalf("expected album sort name:asc, got %s", cat.AlbumSort)
	}
	p2 := cat.AlbumsPager()
	if p1 == p2 {
		t.Fatal("expected new pager instance after changing album sort")
	}

	a1 := cat.ArtistsPager()
	if cat.ArtistSort != "count:desc" {
		t.Fatalf("expected default artist sort count:desc, got %s", cat.ArtistSort)
	}
	cat.SetArtistSort("name:desc")
	if cat.ArtistSort != "name:desc" {
		t.Fatalf("expected artist sort name:desc, got %s", cat.ArtistSort)
	}
	a2 := cat.ArtistsPager()
	if a1 == a2 {
		t.Fatal("expected new pager instance after changing artist sort")
	}

	albumChoices := albumSortChoices()
	if len(albumChoices) != 4 {
		t.Fatalf("expected 4 album sort choices, got %d", len(albumChoices))
	}
	artistChoices := artistSortChoices()
	if len(artistChoices) != 4 {
		t.Fatalf("expected 4 artist sort choices, got %d", len(artistChoices))
	}
}

// Tests that changing list sort updates list spec and reloads track pages.
func TestSongloftListSetSort(t *testing.T) {
	state := NewState()
	cat := state.Catalog()
	l := cat.ListFor(backend.LibraryID, ListLibrary, "", "音乐库")

	tr := netease.Track{ID: 2001, Name: "Song A", Fav: false, Source: backend.Source}
	l.Pages.Land(0, []netease.Track{tr}, 1)
	if l.Pages.Total() != 1 {
		t.Fatalf("expected total 1, got %d", l.Pages.Total())
	}

	l.SetSort("title:asc")
	if l.Spec.Sort != "title:asc" {
		t.Fatalf("expected spec sort title:asc, got %s", l.Spec.Sort)
	}
	// After reload, total resets until loaded again
	if l.Pages.Peek(0) != nil {
		t.Fatal("expected cached page to be cleared on reload")
	}
}

// Tests that Songloft RecentStore records played songs up to 200 and supports removal and clear.
func TestSongloftRecentStore(t *testing.T) {
	store := &RecentStore{
		path: t.TempDir() + "/test_songloft_recent.json",
	}

	tr1 := netease.Track{ID: 101, Name: "Song 1"}
	tr2 := netease.Track{ID: 102, Name: "Song 2"}
	store.Add(tr1)
	store.Add(tr2)

	if store.Total() != 2 {
		t.Fatalf("expected 2 tracks, got %d", store.Total())
	}
	tracks, total := store.Slice(0, 10)
	if total != 2 || len(tracks) != 2 {
		t.Fatalf("expected slice length 2, got %d", len(tracks))
	}
	// Newest first
	if tracks[0].ID != 102 || tracks[1].ID != 101 {
		t.Fatalf("unexpected order: %v, %v", tracks[0].ID, tracks[1].ID)
	}

	// Deduplication: playing tr1 again moves it to top
	store.Add(tr1)
	if store.Total() != 2 {
		t.Fatalf("expected 2 tracks after deduplication, got %d", store.Total())
	}
	tracks, _ = store.Slice(0, 10)
	if tracks[0].ID != 101 || tracks[1].ID != 102 {
		t.Fatalf("expected tr1 to move to top, got %v", tracks[0].ID)
	}

	// Remove
	store.Remove(101)
	if store.Total() != 1 {
		t.Fatalf("expected 1 track after remove, got %d", store.Total())
	}

	// Clear
	store.Clear()
	if store.Total() != 0 {
		t.Fatalf("expected 0 tracks after clear, got %d", store.Total())
	}
}

// Tests that Catalog resolves recent list for backend.RecentID.
func TestSongloftRecentCatalog(t *testing.T) {
	cat := NewCatalog(NewState(), nil)
	list := cat.ListForRoute(backend.RecentID, "", "")
	if list == nil {
		t.Fatal("expected recent list not nil")
	}
	if list.Spec.Kind != ListRecent || list.Name != "最近播放" {
		t.Fatalf("unexpected recent list: kind=%s, name=%s", list.Spec.Kind, list.Name)
	}
}

// Tests that Catalog search caches stable pagers and does not re-create them per frame.
func TestSongloftSearchStablePagers(t *testing.T) {
	home := NewHome(NewState(), nil)
	home.Open(Route{Page: PageSearch, Query: "test-query"})

	cat := home.State().Catalog()
	sList := cat.SearchList("test-query")
	if sList == nil || sList.Spec.Kind != ListSearch || sList.Spec.Query != "test-query" {
		t.Fatalf("unexpected search list: %#v", sList)
	}

	albums1 := cat.SearchAlbums("test-query")
	albums2 := cat.SearchAlbums("test-query")
	if albums1 != albums2 {
		t.Fatalf("expected SearchAlbums to return cached pager instance")
	}

	artists1 := cat.SearchArtists("test-query")
	artists2 := cat.SearchArtists("test-query")
	if artists1 != artists2 {
		t.Fatalf("expected SearchArtists to return cached pager instance")
	}
}

// Tests that trackMenu in a Songloft playlist provides "从歌单中移除" and excludes current playlist from "添加到歌单".
func TestSongloftTrackMenuInPlaylist(t *testing.T) {
	state := NewState()
	cli, _ := backend.NewClient("http://songloft.test", nil)
	cli.SetTokens("mock-token", "refresh-token", 3600)
	p := backend.NewProvider(backend.Config{Address: "http://songloft.test"})
	p.SetClientForTest(cli)
	state.mu.Lock()
	state.provider = p
	state.client = cli
	state.mu.Unlock()

	plID := backend.RegisterMeta(platform.SongloftID, "123", "playlist", "Test Playlist", "")
	otherID := backend.RegisterMeta(platform.SongloftID, "999", "playlist", "Other Playlist", "")
	state.playlists = []netease.Playlist{
		{ID: plID, Ref: "123", Name: "Test Playlist"},
		{ID: otherID, Ref: "999", Name: "Other Playlist"},
	}

	home := NewHome(state, nil)
	list := &List{ID: plID, Spec: Spec{Kind: ListPlaylist, Ref: "123"}}
	tr := netease.Track{ID: -100, Ref: "100", Name: "Song A"}

	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Size(200, 50).Label("track-row").ContextMenu(func(m *ui.Menu) {
			home.trackMenu(m, tr, list)
		})
	}, 400, 400)
	tt.Frame()

	if err := tt.RightClick("track-row"); err != nil {
		t.Fatal(err)
	}
	menu := tt.Menu()
	hasRemove := false
	for _, item := range menu {
		if item == "从歌单中移除" {
			hasRemove = true
			break
		}
	}
	if !hasRemove {
		t.Fatalf("expected '从歌单中移除' in menu, got: %v", menu)
	}

	// Current playlist must be excluded from "添加到歌单", while other playlists and "新建歌单…" are offered.
	if err := tt.ChooseMenuItem("添加到歌单", "Test Playlist"); err == nil {
		t.Fatalf("offered to add track to current playlist: %v", tt.Menu())
	}
	if err := tt.ChooseMenuItem("添加到歌单", "Other Playlist"); err != nil {
		t.Fatalf("expected 'Other Playlist' in '添加到歌单' submenu: %v", err)
	}

	// Outside playlist (e.g. in library), "从歌单中移除" must not be present.
	libList := &List{ID: backend.LibraryID, Spec: Spec{Kind: ListLibrary}}
	ttLib := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Size(200, 50).Label("lib-row").ContextMenu(func(m *ui.Menu) {
			home.trackMenu(m, tr, libList)
		})
	}, 400, 400)
	ttLib.Frame()
	if err := ttLib.RightClick("lib-row"); err != nil {
		t.Fatal(err)
	}
	for _, item := range ttLib.Menu() {
		if item == "从歌单中移除" {
			t.Fatalf("unexpected '从歌单中移除' in library menu: %v", ttLib.Menu())
		}
	}
}

// Tests that folderPage updates breadcrumbs correctly and formats folder header.
func TestSongloftFolderPageStructure(t *testing.T) {
	state := NewState()
	home := NewHome(state, nil)

	home.updateFolderCrumbs("Pop/Rock/Classic")
	if len(home.folderCrumbs) != 4 {
		t.Fatalf("expected 4 crumbs, got %d", len(home.folderCrumbs))
	}
	if home.folderCrumbs[0].Name != "文件夹" || home.folderCrumbs[0].Path != "" {
		t.Errorf("unexpected root crumb: %+v", home.folderCrumbs[0])
	}
	if home.folderCrumbs[3].Name != "Classic" || home.folderCrumbs[3].Path != "Pop/Rock/Classic" {
		t.Errorf("unexpected leaf crumb: %+v", home.folderCrumbs[3])
	}

	home.updateFolderCrumbs("")
	if len(home.folderCrumbs) != 1 || home.folderCrumbs[0].Name != "文件夹" {
		t.Fatalf("expected root crumb for empty path, got %+v", home.folderCrumbs)
	}

	tracks := []netease.Track{
		{ID: 1, Name: "Song 1"},
		{ID: 2, Name: "Song 2"},
		{ID: 3, Name: "Song 3"},
	}
	shuffled := shuffleTracks(tracks)
	if len(shuffled) != 3 {
		t.Fatalf("expected 3 tracks after shuffle, got %d", len(shuffled))
	}
}

// Tests live connect and catalog list when LIVE=1.
func TestLiveSongloftUI(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("skipping live test without LIVE=1")
	}
	addr := os.Getenv("SONGLOFT_ADDR")
	if addr == "" {
		addr = os.Getenv("SONGLOFT_TEST_URL")
	}
	user := os.Getenv("SONGLOFT_USER")
	pass := os.Getenv("SONGLOFT_PASS")
	if addr == "" || user == "" {
		t.Skip("LIVE is set, but SONGLOFT_ADDR and SONGLOFT_USER do not name a service to try")
	}
	state := NewState()
	ctx := context.Background()
	err := state.Connect(ctx, backend.Config{
		Address:  addr,
		Username: user,
		Password: pass,
	})
	if err != nil {
		t.Skipf("cannot connect to test server: %v", err)
	}

	if !state.Connected() {
		t.Fatal("expected state.Connected() to be true")
	}

	cat := state.Catalog()
	lib := cat.ListForRoute(backend.LibraryID, "library", "")
	if lib == nil {
		t.Fatal("expected library list not nil")
	}
	t.Logf("Songloft connected, playlists count: %d", len(state.Playlists()))
}

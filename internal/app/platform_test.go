package app

import (
	"errors"
	"testing"

	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform"
	"netease-music-mygo/internal/platform/paging"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func newPlatformTestApp() *app {
	a := newTestApp()
	a.platforms = map[platform.ID]appplatform.Adapter{}
	a.platformID = platform.NeteaseID
	a.settings = defaultSettings()
	newPlatformAdapters(a)
	a.configurePlatformPlayer()
	return a
}

// Switching platforms changes only the platform-owned context and restores the upstream home when switched back.
func TestPlatformContextSwitch(t *testing.T) {
	a := newPlatformTestApp()
	if a.platformID != platform.NeteaseID {
		t.Fatalf("default platform = %q", a.platformID)
	}
	a.pl.Cue([]netease.Track{{ID: 1, Name: "网易云歌曲"}})
	a.setPlatform(platform.FnmusicID)
	if a.pl.Current() != nil || len(a.pl.Queue()) != 0 || a.pl.Radio() != "" {
		t.Fatal("platform switch retained playback context")
	}
	if a.platformID != platform.FnmusicID || a.settings.Platform != string(platform.FnmusicID) {
		t.Fatalf("Feiniu platform was not selected: id=%q setting=%q", a.platformID, a.settings.Platform)
	}
	if a.pl.ResolveStream == nil {
		t.Fatal("Feiniu player hooks were not installed")
	}
	if got := a.activeHome().Route().Page; got != "library" {
		t.Fatalf("Feiniu home route = %q, want library", got)
	}
	a.setPlatform(platform.NeteaseID)
	if a.platformID != platform.NeteaseID || a.settings.Platform != string(platform.NeteaseID) || a.route.page != pageHome {
		t.Fatalf("NetEase platform was not restored: id=%q setting=%q route=%v", a.platformID, a.settings.Platform, a.route)
	}
	if a.pl.ResolveStream != nil || a.pl.Artwork != nil || a.pl.ResolveLyrics != nil {
		t.Fatal("provider player hooks survived switching back to NetEase")
	}
}

// Selecting QQ Music installs its player hooks and starts on its home page.
func TestQQMusicPlatformContextSwitch(t *testing.T) {
	a := newPlatformTestApp()
	a.setPlatform(platform.QQMusicID)
	if a.platformID != platform.QQMusicID || a.settings.Platform != string(platform.QQMusicID) {
		t.Fatalf("QQ Music was not selected: id=%q setting=%q", a.platformID, a.settings.Platform)
	}
	if got := a.activeHome().Route().Page; got != "home" {
		t.Fatalf("QQ Music home route = %q", got)
	}
	if a.pl.ResolveStream == nil || a.pl.Artwork == nil || a.pl.ResolveLyrics == nil {
		t.Fatal("QQ Music player hooks were not installed")
	}
	a.setPlatform(platform.NeteaseID)
	if a.pl.ResolveStream != nil {
		t.Fatal("QQ Music player hooks survived switching back to NetEase")
	}
}

// Selecting Subsonic family platforms installs player hooks and switches home route.
func TestSubsonicPlatformsContextSwitch(t *testing.T) {
	for _, id := range []platform.ID{platform.NavidromeID, platform.JellyfinID, platform.GonicID, platform.SubsonicID} {
		a := newPlatformTestApp()
		a.setPlatform(id)
		if a.platformID != id || a.settings.Platform != string(id) {
			t.Fatalf("%s was not selected: id=%q setting=%q", id, a.platformID, a.settings.Platform)
		}
		if got := a.activeHome().Route().Page; got != "library" {
			t.Fatalf("%s home route = %q", id, got)
		}
		if a.pl.ResolveStream == nil || a.pl.Artwork == nil || a.pl.ResolveLyrics == nil {
			t.Fatalf("%s player hooks were not installed", id)
		}
		a.setPlatform(platform.NeteaseID)
		if a.pl.ResolveStream != nil {
			t.Fatalf("%s player hooks survived switching back to NetEase", id)
		}
	}
}

// Shared platform settings enumerate adapter identities instead of hardcoding provider names.
// In default mode, NetEase and QQ Music are hidden and Feiniu is first.
// In developer mode, NetEase and QQ Music are placed at the bottom.
func TestPlatformInfosComeFromAdapters(t *testing.T) {
	a := newPlatformTestApp()

	// Default mode: DeveloperMode == false
	infos := a.platformInfos()
	if len(infos) != 6 {
		t.Fatalf("default platform count = %d, want 6", len(infos))
	}
	if infos[0].ID != platform.FnmusicID || infos[0].Name != "飞牛音乐" {
		t.Fatalf("first platform in default mode = %#v", infos[0])
	}
	for _, info := range infos {
		if info.ID == platform.NeteaseID || info.ID == platform.QQMusicID {
			t.Fatalf("commercial platform %s visible in default mode", info.ID)
		}
	}

	// Developer mode: DeveloperMode == true
	a.settings.DeveloperMode = true
	infos = a.platformInfos()
	if len(infos) != 8 {
		t.Fatalf("developer mode platform count = %d, want 8", len(infos))
	}
	expectedOrder := []platform.ID{
		platform.FnmusicID,
		platform.NavidromeID,
		platform.JellyfinID,
		platform.GonicID,
		platform.SubsonicID,
		platform.SongloftID,
		platform.NeteaseID,
		platform.QQMusicID,
	}
	for i, wantID := range expectedOrder {
		if infos[i].ID != wantID {
			t.Fatalf("infos[%d].ID = %q, want %q", i, infos[i].ID, wantID)
		}
	}
	if infos[6].Name != "网易云音乐" {
		t.Fatalf("NetEase platform = %#v", infos[6])
	}
	if infos[7].Name != "QQ 音乐" {
		t.Fatalf("QQ Music platform = %#v", infos[7])
	}
}

// Platform capabilities keep NetEase-only settings out of adapters that do not
// support them, while QQ Music keeps the playback rows it does share.
func TestPlatformCapabilities(t *testing.T) {
	a := newPlatformTestApp()
	a.settings.DeveloperMode = true
	infos := a.platformInfos()
	var neteaseInfo *appplatform.Info
	for _, info := range infos {
		if info.ID == platform.NeteaseID {
			neteaseInfo = &info
			break
		}
	}
	if neteaseInfo == nil || !neteaseInfo.Quality || !neteaseInfo.Normalize || !neteaseInfo.Podcasts || !neteaseInfo.Comments {
		t.Fatalf("NetEase capabilities = %#v", neteaseInfo)
	}
	for _, info := range infos {
		if info.ID == platform.NeteaseID {
			continue
		}
		if info.Podcasts {
			t.Fatalf("NetEase-only capability leaked into adapter %#v", info)
		}
		if info.ID == platform.FnmusicID && (info.Quality || info.Normalize || info.Comments) {
			t.Fatalf("optional capabilities leaked into Feiniu %#v", info)
		}
		if info.ID == platform.QQMusicID && (!info.Quality || !info.Normalize || !info.Comments) {
			t.Fatalf("QQ Music lost its playback or comment settings %#v", info)
		}
	}
}

// Clicking version 5 times activates developer mode, and 5 more times deactivates it.
func TestDeveloperModeToggle(t *testing.T) {
	a := newPlatformTestApp()
	if a.settings.DeveloperMode {
		t.Fatal("developer mode enabled by default")
	}

	for i := 1; i <= 4; i++ {
		a.onVersionClicked()
		if a.settings.DeveloperMode {
			t.Fatalf("developer mode enabled prematurely at click %d", i)
		}
		if a.toast != "" {
			t.Fatalf("unexpected toast on intermediate click %d: %q", i, a.toast)
		}
		if i == 3 && a.versionDesc() != "再点击 2 次开启开发者模式" {
			t.Fatalf("unexpected desc on click 3: %q", a.versionDesc())
		}
		if i == 4 && a.versionDesc() != "再点击 1 次开启开发者模式" {
			t.Fatalf("unexpected desc on click 4: %q", a.versionDesc())
		}
	}
	a.onVersionClicked() // 5th click
	if !a.settings.DeveloperMode {
		t.Fatal("developer mode not enabled after 5 clicks")
	}
	if a.toast != "已开启开发者模式" {
		t.Fatalf("unexpected toast: %q", a.toast)
	}
	if a.versionDesc() != "开发者模式已开启" {
		t.Fatalf("unexpected desc after enabling: %q", a.versionDesc())
	}

	// 5 more clicks to disable
	a.toast = ""
	for i := 1; i <= 4; i++ {
		a.onVersionClicked()
		if !a.settings.DeveloperMode {
			t.Fatalf("developer mode disabled prematurely at click %d", i)
		}
		if a.toast != "" {
			t.Fatalf("unexpected toast on disable click %d: %q", i, a.toast)
		}
		if i == 3 && a.versionDesc() != "再点击 2 次关闭开发者模式" {
			t.Fatalf("unexpected desc on disable click 3: %q", a.versionDesc())
		}
	}
	a.onVersionClicked() // 5th click
	if a.settings.DeveloperMode {
		t.Fatal("developer mode not disabled after 5 clicks")
	}
	if a.toast != "已关闭开发者模式" {
		t.Fatalf("unexpected toast: %q", a.toast)
	}
}

// Clicking the version row 5 times in the settings view activates developer mode.
func TestSettingsDeveloperModeClickUI(t *testing.T) {
	a := newPlatformTestApp()
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	verLabel := "当前版本 " + mygo.App.Version()
	for i := 0; i < 5; i++ {
		if err := tt.Click(verLabel); err != nil {
			t.Fatalf("click %d failed: %v", i+1, err)
		}
		tt.Frame()
	}
	if !a.settings.DeveloperMode {
		t.Fatal("developer mode not enabled after 5 clicks in settingsView")
	}

	// Also test SharedSettings in platform host
	a.settings.DeveloperMode = false
	h := platformHost{a: a}
	ttHost := ui.NewTester(h.SharedSettings, 560, 2000)
	ttHost.Frame()
	for i := 0; i < 5; i++ {
		if err := ttHost.Click(verLabel); err != nil {
			t.Fatalf("host click %d failed: %v", i+1, err)
		}
		ttHost.Frame()
	}
	if !a.settings.DeveloperMode {
		t.Fatal("developer mode not enabled after 5 clicks in SharedSettings")
	}
}

// A detail page waiting for its hero keeps every real label behind the same full skeleton.
func TestPlatformHeroWaitKeepsFullSkeleton(t *testing.T) {
	a := newTestApp()
	a.art.fetch = func(string) ([]byte, error) { return nil, errors.New("not ready") }
	track := netease.Track{ID: 1, Name: "真实歌曲"}
	list := &appplatform.TrackList{
		Total: func() int { return 1 },
		Len:   func() int { return 1 },
		At:    func(int) *netease.Track { return &track },
	}
	view := appplatform.TrackView{
		Cover: "fnmusic://cover",
		Header: func(c *ui.Context) {
			ui.Text(c, "真实标题")
		},
		HeaderSkeleton: func(c *ui.Context) {
			a.playlistHeaderSkeleton(c, a.pageShimmer(c))
		},
		ShowAlbum: true,
	}
	h := platformHost{a: a}
	tt := ui.NewTester(func(c *ui.Context) {
		t := *c.Theme()
		a.pal = paletteFor(&t)
		h.RenderTracks(c, list, view)
	}, 1200, 800)
	tt.Frame()
	if tt.HasText("真实标题") || tt.HasText("真实歌曲") {
		t.Fatal("real content showed through the hero-loading skeleton")
	}
}

// Search keeps loaded songs behind its full skeleton until album and artist results settle.
func TestPlatformSearchWaitsForAllFirstPages(t *testing.T) {
	a := newTestApp()
	track := netease.Track{ID: 1, Name: "真实歌曲"}
	songs := &appplatform.TrackList{
		Total: func() int { return 1 },
		Len:   func() int { return 1 },
		At:    func(int) *netease.Track { return &track },
	}
	albums := paging.New[netease.Playlist](60, 0, nil, nil)
	artists := paging.New[netease.Playlist](60, 0, nil, nil)
	h := platformHost{a: a}
	results := appplatform.SearchResults{Query: "春日影", Songs: songs, Albums: albums, Artists: artists}
	tt := ui.NewTester(func(c *ui.Context) {
		t := *c.Theme()
		a.pal = paletteFor(&t)
		h.RenderSearch(c, results, appplatform.SearchView{})
	}, 1200, 800)
	tt.Frame()
	if tt.HasText("真实歌曲") || tt.HasText("“春日影”") {
		t.Fatal("search revealed content before its first pages settled")
	}
	albums.Land(0, nil, 0)
	tt.Frame()
	if tt.HasText("真实歌曲") {
		t.Fatal("search revealed songs while artists were still loading")
	}
	artists.Land(0, nil, 0)
	tt.Frame()
	if !tt.HasText("真实歌曲") || !tt.HasText("“春日影”") {
		t.Fatal("search did not reveal its results after every first page settled")
	}
}

// A failed first list page shows its retry state instead of a permanent skeleton.
func TestPlatformListFailureLeavesSkeleton(t *testing.T) {
	a := newTestApp()
	list := &appplatform.TrackList{
		Total:  func() int { return -1 },
		Len:    func() int { return 0 },
		At:     func(int) *netease.Track { return nil },
		Failed: func() bool { return true },
	}
	h := platformHost{a: a}
	tt := ui.NewTester(func(c *ui.Context) {
		t := *c.Theme()
		a.pal = paletteFor(&t)
		h.RenderTracks(c, list, appplatform.TrackView{ShowAlbum: true})
	}, 1200, 800)
	tt.Frame()
	if !tt.HasText("加载失败，请稍后重试") {
		t.Fatal("failed list remained behind its loading skeleton")
	}
}

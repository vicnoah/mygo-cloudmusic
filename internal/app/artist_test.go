package app

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// Each artist's name opens their page: from Now Playing, which closes, and
// from a song's row, which doesn't play the song as the row's click would.
func TestArtistNameOpensTheirPage(t *testing.T) {
	artistDetail = func(int64) (*netease.ArtistProfile, error) { return nil, errors.New("offline") }
	defer func() { artistDetail = netease.ArtistDetail }()
	a := newTestApp()
	a.settings = defaultSettings()
	duet := netease.Track{ID: 1, Name: "广寒宫", Artists: []netease.Artist{{ID: 11, Name: "甲"}, {ID: 12, Name: "乙"}}, Duration: 200000}
	a.pl.Cue([]netease.Track{duet})
	a.pl.SetLyrics(nil)
	a.artists[12] = &artistData{profile: &netease.ArtistProfile{
		Artist: netease.Artist{ID: 12, Name: "乙"},
		Songs:  []netease.Track{{ID: 2, Name: "起风了", Artists: []netease.Artist{{ID: 13, Name: "丙"}}, Duration: 200000}},
	}}
	tt := ui.NewTester(a.view, 1200, 800)
	a.nowPlaying = true
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()

	// The second of two artists credited.
	if err := tt.Click("乙"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.nowPlaying || a.route != (route{page: pageArtist, id: 12}) {
		t.Fatalf("after the click: Now Playing open %v, page %+v", a.nowPlaying, a.route)
	}

	tt.Advance(time.Second)
	tt.Frame()
	// The name sits inside the song row's button, which the tester takes
	// for the target of its center; click where it is, as the row gets it,
	// and check the link opened instead of the song playing.
	if r, ok := tt.Find("丙"); !ok {
		t.Fatal("no artist name 丙")
	} else {
		tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	}
	tt.Frame()
	if a.route != (route{page: pageArtist, id: 13}) {
		t.Fatalf("a row's artist opened %+v", a.route)
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != duet.ID {
		t.Errorf("a click on a row's artist played %+v", cur)
	}

	// Back goes to the artist before.
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.route.id != 12 {
		t.Errorf("back went to %+v", a.route)
	}
}

// 最新 lists an artist's newest songs, asked of NetEase once, and a click
// on one plays the list shown.
func TestArtistSongsByLatest(t *testing.T) {
	asked := make(chan int64, 4)
	artistLatestSongs = func(id int64, _ int) ([]netease.Track, error) { asked <- id; return nil, errors.New("offline") }
	defer func() { artistLatestSongs = netease.ArtistLatestSongs }()
	a := newTestApp()
	a.settings = defaultSettings()
	song := func(id int64, name string) netease.Track {
		return netease.Track{ID: id, Name: name, Artists: []netease.Artist{{ID: 12, Name: "乙"}}, Duration: 200000}
	}
	d := &artistData{profile: &netease.ArtistProfile{Artist: netease.Artist{ID: 12, Name: "乙"}, Songs: []netease.Track{song(1, "老歌")}}}
	a.artists[12] = d
	a.open(route{page: pageArtist, id: 12})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	if err := tt.Click("最新"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	select {
	case id := <-asked:
		if id != 12 {
			t.Fatalf("asked for the songs of %d", id)
		}
	case <-time.After(time.Second):
		t.Fatal("the newest songs weren't asked for")
	}
	// As the answer lands, which post drops without a window.
	d.latest, d.latestLoading = []netease.Track{song(2, "新歌"), song(3, "更新的歌")}, false
	tt.Frame()
	if err := tt.Click("新歌"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if cur := a.pl.Current(); cur == nil || cur.ID != 2 || len(a.pl.Queue()) != 2 {
		t.Fatalf("playing %+v of %d", cur, len(a.pl.Queue()))
	}

	// Back and forth, the songs loaded stay.
	tt.Click("热门")
	tt.Frame()
	tt.Click("最新")
	tt.Frame()
	if len(asked) > 0 {
		t.Error("the newest songs were asked for again")
	}
}

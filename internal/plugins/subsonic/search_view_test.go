package subsonic

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/paging"
)

type searchTestHost struct{ Host }

func (searchTestHost) Player() PlayerPort                     { return nil }
func (searchTestHost) Artwork(string, int) (*ui.Bitmap, bool) { return nil, true }

// A search result can be played after the songs, albums and artists have all loaded.
func TestSubsonicSearchWaitsForAllFirstPages(t *testing.T) {
	home := NewHome(NewState(), searchTestHost{})
	songs := paging.New[pluginapi.Track](20, 0, nil, nil)
	songs.Land(0, []pluginapi.Track{{ID: 1, Name: "Sample song"}}, 1)
	albums := paging.New[pluginapi.Playlist](20, 0, nil, nil)
	artists := paging.New[pluginapi.Playlist](20, 0, nil, nil)
	results := SearchResults{Query: "sample", Songs: songs, Albums: albums, Artists: artists}
	played := 0
	view := SearchView{Play: func(int) { played++ }}
	tt := ui.NewTester(func(c *ui.Context) { home.renderSearch(c, results, view) }, 1200, 800)
	albums.Land(0, nil, 0)
	tt.Frame()
	_ = tt.Click("Sample song")
	if played != 0 {
		t.Fatal("a song played before the artist results landed")
	}
	artists.Land(0, nil, 0)
	tt.Frame()
	if err := tt.Click("Sample song"); err != nil {
		t.Fatal(err)
	}
	if played != 1 {
		t.Fatalf("after all first pages loaded, clicks started the song %d times", played)
	}
}

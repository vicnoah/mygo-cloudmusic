package qqmusic

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
)

type listTestHost struct {
	Host
	ready bool
}

func (h *listTestHost) Artwork(string, int) (*ui.Bitmap, bool) { return nil, h.ready }
func (h *listTestHost) Player() PlayerPort                     { return nil }

// A detail page accepts song clicks once the hero that gates its content is ready.
func TestQQHeroLoadingGatesSongClicks(t *testing.T) {
	host := &listTestHost{}
	home := NewHome(NewState(), host)
	songs := staticTracks([]pluginapi.Track{{ID: 1, Name: "Sample song"}})
	played := 0
	view := TrackView{Cover: "sample-cover", OnPlay: func(int) { played++ }}
	tt := ui.NewTester(func(c *ui.Context) { home.renderTracks(c, songs, view) }, 1200, 800)
	_ = tt.Click("Sample song")
	if played != 0 {
		t.Fatal("the user could start the song while the page was loading")
	}
	host.ready = true
	tt.Frame()
	if err := tt.Click("Sample song"); err != nil {
		t.Fatal(err)
	}
	if played != 1 {
		t.Fatalf("after loading, clicks started the song %d times", played)
	}
}

// A failed first page can be retried from the QQ track list.
func TestQQTrackListRetriesFailedPage(t *testing.T) {
	home := NewHome(NewState(), &listTestHost{})
	songs := staticTracks(nil)
	songs.Failed = func() bool { return true }
	retries := 0
	songs.Retry = func() { retries++ }
	tt := ui.NewTester(func(c *ui.Context) { home.renderTracks(c, songs, TrackView{}) }, 1200, 800)
	if err := tt.Click("重试"); err != nil {
		t.Fatal(err)
	}
	if retries != 1 {
		t.Fatalf("retry clicked once, loads restarted %d times", retries)
	}
}

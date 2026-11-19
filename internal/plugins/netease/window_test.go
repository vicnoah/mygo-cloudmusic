package netease

import (
	"context"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
)

// A result queued by an old activation cannot update a newly active NetEase window.
func TestPlaybackPlatformDiscardsLateResults(t *testing.T) {
	a := newTestApp()
	h := a.hostTest
	h.collectPosts = true
	a.scopedPost()(func() { a.toast = "old" })
	a.deactivate()
	a.activate(context.Background(), h)
	a.scopedPost()(func() { a.toast = "new" })
	if len(h.posted) != 2 {
		t.Fatalf("queued %d results, want 2", len(h.posted))
	}
	h.posted[0]()
	if a.toast != "" {
		t.Fatal("a result from the previous activation wrote into the current window")
	}
	h.posted[1]()
	if a.toast != "new" {
		t.Fatal("the current window did not receive its result")
	}
}

// Switching away from Now Playing restores the button theme of the reused window.
func TestPlaybackWindowRestoresButtonsOnDeactivate(t *testing.T) {
	a := newTestApp()
	a.pl.Cue([]pluginapi.Track{{ID: 1, Name: "宝船", Duration: 180000}})
	a.pl.SetLyrics(nil)
	tt := ui.NewTester(a.view, 1180, 780)
	if err := tt.Click("打开正在播放"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.hostTest.windowButtonsTheme != mygo.ThemeDark {
		t.Fatal("Now Playing did not theme the window buttons")
	}
	a.deactivate()
	if a.hostTest.windowButtonsTheme != mygo.ThemeSystem {
		t.Fatal("the reused window kept the theme of Now Playing after switching plugins")
	}
}

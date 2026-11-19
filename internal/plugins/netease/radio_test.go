package netease

import (
	"context"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/plugins/netease/internal/client"
)

// Restoring a paused radio queue reattaches its feed at the saved track and position.
func TestNetEaseRestoresPausedRadio(t *testing.T) {
	a := newTestApp()
	t.Cleanup(a.pl.Reset)
	a.pl.ResolveStream = func(ctx context.Context, _ *pluginapi.Track, _ string) (*pluginapi.Stream, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	a.hostTest.savedSession = &pluginapi.PlaybackSession{
		Queue: []pluginapi.Track{{ID: 7}, {ID: 8, Duration: 200000}, {ID: 9}, {ID: 10}},
		Order: []int{0, 1, 2, 3}, At: 1, Position: 48000, Radio: "fm:DEFAULT",
	}
	a.restoreSession()
	current := a.pl.Current()
	if current == nil || current.ID != 8 || a.pl.Playing() || a.pl.Position() != 48*time.Second || a.pl.Radio() != "fm:DEFAULT" {
		t.Fatalf("restored track %v at %v, playing=%v, radio=%q", current, a.pl.Position(), a.pl.Playing(), a.pl.Radio())
	}
}

func TestCommentTime(t *testing.T) {
	now := time.Date(2026, 10, 25, 22, 0, 0, 0, time.Local)
	for _, c := range []struct {
		t    time.Time
		want string
	}{
		{now.Add(-20 * time.Second), "刚刚"},
		{now.Add(-42 * time.Minute), "42分钟前"},
		{now.Add(-5 * time.Hour), "17:00"},
		{now.Add(-23 * time.Hour), "昨天 23:00"},
		{time.Date(2026, 3, 8, 9, 0, 0, 0, time.Local), "3月8日"},
		{time.Date(2021, 4, 10, 9, 0, 0, 0, time.Local), "2021年4月10日"},
	} {
		if got := commentTime(c.t, now); got != c.want {
			t.Errorf("%v: %q, want %q", c.t, got, c.want)
		}
	}
}

// Heart mode playing, its card and Now Playing offer to leave it, which
// keeps the song playing.
func TestExitHeartMode(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &netease.Profile{UserID: 1, Nickname: "听众"}
	a.pl.Cue([]netease.Track{{ID: 1, Name: "晚风"}, {ID: 2, Name: "推荐的歌"}})
	a.pl.AttachRadio(heartRadio, func(got func([]netease.Track)) {})
	a.route = route{page: pageFM}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("退出心动模式"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.pl.Radio() != "" || a.pl.Current().ID != 1 || len(a.pl.Queue()) != 1 {
		t.Fatalf("radio %q, current %v, queue %d", a.pl.Radio(), a.pl.Current(), len(a.pl.Queue()))
	}
	if !tt.HasText("开启心动模式") {
		t.Errorf("card still on: %q", tt.Texts())
	}

	a.pl.AttachRadio(heartRadio, func(got func([]netease.Track)) {})
	a.nowPlaying = true
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("退出心动模式"); err != nil {
		t.Fatal(err)
	}
	if a.pl.Radio() != "" || a.pl.Current().ID != 1 {
		t.Fatalf("from Now Playing: radio %q, current %v", a.pl.Radio(), a.pl.Current())
	}
}

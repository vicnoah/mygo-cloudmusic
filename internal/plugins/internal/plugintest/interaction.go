package plugintest

import (
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
)

// silence is a song of silence, to play without a stream.
type silence struct{ pos int64 }

func (s *silence) SampleRate() int           { return 48000 }
func (s *silence) Channels() int             { return 2 }
func (s *silence) Len() int64                { return 48000 * 200 }
func (s *silence) SetPosition(f int64) error { s.pos = f; return nil }
func (s *silence) Read(buf []float32) (int, error) {
	clear(buf)
	s.pos += int64(len(buf) / 2)
	return len(buf), nil
}

// AttachSilence attaches a controllable audio source without fetching a stream.
func (h *Host) AttachSilence(t *testing.T, playing bool) {
	t.Helper()
	out := audio.NewPlayer(&silence{})
	t.Cleanup(func() { out.Close() })
	out.SetVolume(0)
	if playing {
		out.Play()
	}
	h.Engine.Attach(out)
}

func CheckSpaceAfterOpeningNowPlaying(t *testing.T, h *Host) {
	t.Helper()
	tr := pluginapi.Track{ID: 7, Name: "宝船", Artists: []pluginapi.Artist{{Name: "DOUDOU"}}, Duration: 200000}
	h.Engine.Cue([]pluginapi.Track{tr})
	h.Engine.SetLyrics(nil)
	h.AttachSilence(t, true)

	tt := ui.NewTester(h.Windows[pluginapi.MainWindow].Root, 1200, 800)
	if err := tt.Click("打开正在播放"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if _, ok := tt.Find("关闭"); !ok {
		t.Fatal("now playing did not open")
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if h.Engine.Playing() {
		t.Error("Space did not pause")
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if !h.Engine.Playing() {
		t.Error("Space did not play again")
	}
}

func CheckScrubSeeksOnRelease(t *testing.T, h *Host) {
	t.Helper()
	tr := pluginapi.Track{ID: 7, Name: "宝船", Duration: 200000}
	h.Engine.Cue([]pluginapi.Track{tr, {ID: 8, Name: "下一首"}})
	h.Engine.SetLyrics(nil)
	h.AttachSilence(t, true)
	tt := ui.NewTester(h.Windows[pluginapi.MainWindow].Root, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	bar, ok := tt.Find("播放进度")
	if !ok {
		t.Fatal("no progress bar")
	}
	y := bar.Y + bar.H/2
	at := func(f float32) float32 { return bar.X + bar.W*f }

	tt.Press(at(0.25), y)
	tt.Frame()
	tt.Move(at(0.6), y-40) // off the bar, above it
	tt.Frame()
	tt.Move(at(1)+30, y+30) // past its end, below it
	tt.Frame()
	if pos := h.Engine.Position(); pos > 10*time.Second {
		t.Fatalf("seeked to %v while dragging", pos)
	}
	if h.Engine.Current().ID != 7 {
		t.Fatal("went to the next song while dragging")
	}
	tt.Move(at(0.6), y-40)
	tt.Frame()
	tt.Release(at(0.6), y-40)
	tt.Frame()
	tt.Frame()
	if pos := h.Engine.Position(); pos < 110*time.Second || pos > 130*time.Second {
		t.Errorf("let go at 60%%, at %v", pos)
	}
}

func CheckDesktopLyricsToolbar(t *testing.T, h *Host) {
	t.Helper()
	h.Lyrics = pluginapi.DesktopLyricsState{PointerInside: true, FontSize: 30}
	window := h.Windows[pluginapi.DesktopLyricsWindow]
	tt := ui.NewTester(window.Root, window.Options.Width, window.Options.Height)
	tt.Frame()
	plus, ok := tt.Find("增大字号")
	if !ok {
		t.Fatal("the font control is unavailable under the pointer")
	}
	if err := tt.Click("增大字号"); err != nil {
		t.Fatal(err)
	}
	if h.Lyrics.FontSize != 32 {
		t.Fatal("increasing the font size did not update lyrics preferences")
	}
	if err := tt.Click("锁定桌面歌词"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.ClickAt(plus.X+plus.W/2, plus.Y+plus.H/2)
	if !h.Lyrics.Locked || h.Lyrics.FontSize != 32 {
		t.Fatal("the old toolbar location accepted input while the lyrics were locked")
	}
	h.Lyrics.Locked, h.Lyrics.PointerInside = false, false
	tt.Frame()
	tt.ClickAt(plus.X+plus.W/2, plus.Y+plus.H/2)
	if h.Lyrics.FontSize != 32 {
		t.Fatal("the toolbar accepted input outside of the lyrics window")
	}
}

func CheckSettingsVersionClicks(t *testing.T, h *Host) {
	t.Helper()
	tt := ui.NewTester(h.Windows[pluginapi.SettingsWindow].Root, 560, 2100)
	version := "当前版本 " + mygo.App.Version()
	for i := 0; i < 5; i++ {
		if err := tt.Click(version); err != nil {
			t.Fatalf("click %d failed: %v", i+1, err)
		}
		tt.Frame()
	}
	if h.VersionClicks != 5 {
		t.Fatalf("reported %d of 5 clicks to the host", h.VersionClicks)
	}
}

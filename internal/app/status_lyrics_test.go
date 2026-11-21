package app

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/waygu-music-plugin/sdk/pluginapi/lyrics"
)

// Status lyrics advance at line boundaries and clear whenever the new track has no lines.
func TestStatusLyricFollowsPlaybackPosition(t *testing.T) {
	lines := []lyrics.Line{
		{At: time.Second, Text: "  first\n line\t", Translation: " 第一行\r\n歌词 "},
		{At: 3 * time.Second, Text: strings.Repeat("歌", 20)},
	}
	cases := []struct {
		lines []lyrics.Line
		at    time.Duration
		line  statusLyricsLine
		after time.Duration
	}{
		{lines, 0, statusLyricsLine{}, time.Second},
		{lines, time.Second, statusLyricsLine{"first line", "第一行 歌词"}, 2 * time.Second},
		{lines, 3 * time.Second, statusLyricsLine{original: strings.Repeat("歌", 20)}, 0},
		{nil, 3 * time.Second, statusLyricsLine{}, 0},
	}
	for _, tc := range cases {
		line, _, after := statusLyricAt(tc.lines, tc.at)
		if line != tc.line || after != tc.after {
			t.Errorf("at %v with %d lines: got (%+v, %v), want (%+v, %v)", tc.at, len(tc.lines), line, after, tc.line, tc.after)
		}
	}
}

// Two rows sharing a timestamp in a bilingual LRC appear together instead of losing the original.
func TestStatusLyricUsesBilingualLRC(t *testing.T) {
	lines := lyrics.Build("[00:01]Shine\n[00:01]闪耀\n[00:03]Next", "", "")
	got, timing, delay := statusLyricAt(lines, time.Second)
	if got.original != "Shine" || got.translation != "闪耀" || delay != 2*time.Second {
		t.Fatalf("got %+v and %v", got, delay)
	}
	if timing != (statusLyricsTiming{time.Second, 3 * time.Second}) {
		t.Fatalf("the original and translation did not share a scroll interval: %+v", timing)
	}
}

// The native label advances through the entire line and keeps joined emoji together at page breaks.
func TestStatusLyricsTrayPagesFollowPlayback(t *testing.T) {
	first, last := strings.Repeat("歌", 16), "👩🏽‍🎤 落幕"
	pages := statusLyricsTrayPages(first + last)
	if len(pages) != 2 || pages[0] != first || pages[1] != last {
		t.Fatalf("split into %q", pages)
	}
	timing := statusLyricsTiming{2 * time.Second, 6 * time.Second}
	title, next := statusLyricsTrayPageAt(pages, timing, 3*time.Second)
	if title != first || next != time.Second {
		t.Fatalf("first page got %q, advancing in %v", title, next)
	}
	title, next = statusLyricsTrayPageAt(pages, timing, 4*time.Second)
	if title != last || next != 0 {
		t.Fatalf("last page got %q, advancing in %v", title, next)
	}
	title, _ = statusLyricsTrayPageAt(pages, timing, 2*time.Second)
	if title != first {
		t.Fatalf("seeking back left %q on screen", title)
	}
}

// Taskbar scrolling reaches the end before the next lyric, and seeking back restores the beginning.
func TestTaskbarLyricsMotionFollowsPlayback(t *testing.T) {
	timing := statusLyricsTiming{time.Second, 5 * time.Second}
	for _, tc := range []struct {
		position time.Duration
		progress float32
		wake     time.Duration
	}{
		{time.Second, 0, 350 * time.Millisecond},
		{3 * time.Second, 0.5, time.Second / 30},
		{4700 * time.Millisecond, 1, 0},
		{time.Second, 0, 350 * time.Millisecond},
	} {
		motion, wake := taskbarLyricsMotion(timing, tc.position)
		if motion != tc.progress || wake != tc.wake {
			t.Errorf("at %v got progress %v, wake %v", tc.position, motion, wake)
		}
	}
}

// Both measured text rows fit a short Windows taskbar at a large requested size.
func TestTaskbarLyricRowsFit(t *testing.T) {
	const height, requested = 32, 20
	rows := shapeTaskbarLyricRows(taskbarLyricStyle{
		original: "Deep in the night", translation: "Lắng nghe những giấc mơ",
		font: "sans-serif", size: requested, height: height,
	})
	if rows.original == nil || rows.translation == nil {
		t.Fatal("a line was dropped instead of fitting both in the taskbar")
	}
	_, original := rows.original.Size()
	_, translation := rows.translation.Size()
	if original+translation+4*taskbarLyricRowPadding > height {
		t.Fatalf("the shaped lines do not fit: original=%v translation=%v height=%v", original, translation, height)
	}
}

// The scrolling viewport leaves room for glyph descenders that a fixed line height cropped.
func TestTaskbarLyricViewportFitsGlyphHeight(t *testing.T) {
	frame := statusLyricsFrame{
		statusLyricsLine: statusLyricsLine{original: "Dropping gypsy letters", translation: "Lắng nghe những giấc mơ"},
		font:             "sans-serif", fontSize: 12,
	}
	tester := ui.NewTester(func(c *ui.Context) { taskbarLyricsView(c, frame, true) }, 170, 48)
	tester.Frame()
	first, ok := tester.Find(frame.original)
	if !ok {
		t.Fatal("missing original lyric viewport")
	}
	second, ok := tester.Find(frame.translation)
	if !ok {
		t.Fatal("missing translated lyric viewport")
	}
	_, originalHeight := ui.ShapeText(frame.original, ui.Font{Family: frame.font, Size: 12, Weight: 600}, 0).Size()
	_, translationHeight := ui.ShapeText(frame.translation, ui.Font{Family: frame.font, Size: 9, Weight: 400}, 0).Size()
	if first.H < originalHeight || second.H < translationHeight || first.Y+first.H > second.Y || second.Y+second.H > 48 {
		t.Fatalf("viewports do not leave room for the shaped text: original=%+v (%v) translation=%+v (%v)",
			first, originalHeight, second, translationHeight)
	}
}

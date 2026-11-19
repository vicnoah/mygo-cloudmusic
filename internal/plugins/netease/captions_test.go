package netease

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi/speech"
	"waygu-music/internal/plugins/netease/internal/client"
)

const sec = time.Second

// Transcribing starts where the listener is, or past what is written down
// there already, and stops where what was written down before begins.
// With all from there on written down, it goes back for what is not:
// an episode played to its end, then given the model, wrote down nothing
// and said it had no speech.
func TestCaptionStart(t *testing.T) {
	spans := []span{{30 * sec, 60 * sec}, {300 * sec, 400 * sec}}
	for _, c := range []struct {
		pos, from, until time.Duration
		ok               bool
	}{
		{10 * sec, 10 * sec, 30 * sec, true},
		{40 * sec, 60 * sec, 300 * sec, true},
		{100 * sec, 100 * sec, 300 * sec, true},
		{350 * sec, 400 * sec, 0, true},
		{500 * sec, 500 * sec, 0, true},
		{599500 * time.Millisecond, 0, 30 * sec, true},
		{600 * sec, 0, 30 * sec, true},
	} {
		from, until, ok := captionStart(spans, c.pos, 600*sec)
		if from != c.from || until != c.until || ok != c.ok {
			t.Errorf("at %v: %v to %v, %v; want %v to %v, %v", c.pos, from, until, ok, c.from, c.until, c.ok)
		}
	}
	if from, until, ok := captionStart([]span{{0, 60 * sec}}, 600*sec, 600*sec); from != 60*sec || until != 0 || !ok {
		t.Errorf("at the end with the start written down: %v to %v, %v", from, until, ok)
	}
	for _, pos := range []time.Duration{0, 300 * sec, 600 * sec} {
		if _, _, ok := captionStart([]span{{0, 600 * sec}}, pos, 600*sec); ok {
			t.Errorf("at %v: an episode written down whole starts again", pos)
		}
	}
}

// Stretches written down merge where they meet, in order.
func TestAddSpan(t *testing.T) {
	spans := addSpan(nil, span{100 * sec, 200 * sec})
	spans = addSpan(spans, span{0, 50 * sec})
	spans = addSpan(spans, span{150 * sec, 250 * sec})
	if want := []span{{0, 50 * sec}, {100 * sec, 250 * sec}}; !slices.Equal(spans, want) {
		t.Fatalf("%v, want %v", spans, want)
	}
	if spans = addSpan(spans, span{50 * sec, 100 * sec}); !slices.Equal(spans, []span{{0, 250 * sec}}) {
		t.Errorf("%v: the gap filled should leave one span", spans)
	}
}

// A transcription started again where one was stopped writes down again
// what that one did last: the transcript keeps it once.
func TestAddSegmentOnce(t *testing.T) {
	segs := addSegment(nil, speech.Segment{Start: 10 * sec, End: 20 * sec, Text: "一"})
	segs = addSegment(segs, speech.Segment{Start: 30 * sec, End: 40 * sec, Text: "三"})
	segs = addSegment(segs, speech.Segment{Start: 21 * sec, End: 29 * sec, Text: "二"})
	segs = addSegment(segs, speech.Segment{Start: 10500 * time.Millisecond, End: 20 * sec, Text: "一"})
	var got []string
	for _, s := range segs {
		got = append(got, s.Text)
	}
	if !slices.Equal(got, []string{"一", "二", "三"}) {
		t.Errorf("%q", got)
	}
}

// captionsTestApp plays an episode in Now Playing, with the libraries in
// the app and the model installed or not.
func captionsTestApp(t *testing.T, installed bool) *testApp {
	oldAvail, oldInstalled := captionsAvailable, modelInstalled
	t.Cleanup(func() { captionsAvailable, modelInstalled = oldAvail, oldInstalled })
	captionsAvailable = func() bool { return true }
	modelInstalled = func(string) bool { return installed }
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 7, Name: "510 泰国侨批沉浮录", ProgramID: 42, Duration: 600000, Description: "下南洋"}})
	a.nowPlaying = true
	return a
}

// Without the model, the episode's captions offer to download it, and
// 下载模型 starts the download.
func TestDownloadModelFromCaptions(t *testing.T) {
	a := captionsTestApp(t, false)
	started := make(chan struct{}, 1)
	old := downloadModel
	defer func() { downloadModel = old }()
	downloadModel = func(ctx context.Context, dir string, progress func(int64, int64)) error {
		started <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Frame()
	if a.npPane != paneProgram {
		t.Fatalf("pane %d: an episode without captions opens on 简介", a.npPane)
	}
	if err := tt.Click("字幕"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("下载模型"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("the download did not start")
	}
	tt.Frame()
	if !a.captions.model.downloading {
		t.Fatal("not downloading")
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
}

// With the model, the episode opens on its captions, and a click on a
// line seeks to it, as a line of lyrics does.
func TestCaptionClickSeeks(t *testing.T) {
	a := captionsTestApp(t, true)
	segs := []speech.Segment{
		{Start: 12 * sec, End: 16 * sec, Text: "有些话题是绕不开的。"},
		{Start: 17 * sec, End: 20 * sec, Text: "比如说，我的婚姻结束了。"},
	}
	a.captions.program = 42
	a.captions.tr = &transcript{Model: speech.ModelName, Segments: segs, Spans: []span{{0, 20 * sec}}}
	a.captions.lines = speech.Lines(segs)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Frame()
	if a.npPane != paneCaptions {
		t.Fatalf("pane %d: an episode with captions opens on 字幕", a.npPane)
	}
	if err := tt.Click("比如说，我的婚姻结束了"); err != nil {
		t.Fatal(err)
	}
	if got := a.pl.Position(); got != 17*sec {
		t.Errorf("at %v, want 17s", got)
	}
}

// Until an episode's transcription starts, its captions say they are on
// their way. Its transcript loaded before its audio opened, they said
// there was no speech, until the transcription started; and before the
// player told the app the episode changed, they showed the last one's, or
// after a song, that they had failed.
func TestCaptionsOnTheirWay(t *testing.T) {
	a := captionsTestApp(t, true)
	tt := ui.NewTester(a.view, 1200, 800)
	for _, c := range []struct {
		name  string
		state captionState
	}{
		{"after a song", captionState{}},
		{"after another episode", captionState{program: 41, tr: &transcript{Spans: []span{{0, 600 * sec}}}, lines: speech.Lines([]speech.Segment{{Start: 0, End: sec, Text: "上一期"}})}},
		{"loaded, the audio not yet", captionState{program: 42, tr: &transcript{Model: speech.ModelName}}},
	} {
		a.captions = c.state
		a.captions.model = modelState{checked: true, installed: true}
		tt.Frame()
		tt.Frame()
		if !tt.HasText("正在识别…") || tt.HasText("上一期") || tt.HasText("这期节目没有识别到语音") || tt.HasText("字幕生成失败") {
			t.Errorf("%s: %q", c.name, tt.Texts())
		}
	}
	// Written down whole, with nothing said, there is no speech.
	a.captions = captionState{program: 42, tr: &transcript{Spans: []span{{0, 600 * sec}}}, model: modelState{checked: true, installed: true}}
	tt.Frame()
	if !tt.HasText("这期节目没有识别到语音") {
		t.Errorf("written down whole: %q", tt.Texts())
	}
}

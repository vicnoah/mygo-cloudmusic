package netease

import (
	"testing"
	"time"

	"waygu-music/internal/pluginapi/lyrics"
)

// Choosing a style from the stage's menu sets its lyrics in it, and
// choosing 流光 goes back to the classic.
func TestStageStyleMenu(t *testing.T) {
	a, tt, _ := playingApp()
	a.nowPlaying, a.npPane = true, paneStage
	a.pl.Seek(2 * time.Second)
	tt.Frame()
	for _, c := range []struct{ item, want string }{{"云阶", "partita"}, {"倾诉", "tilt"}, {"流光", ""}} {
		if err := tt.Click("律动效果"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(c.item); err != nil {
			t.Fatalf("%v; the menu shows %q", err, tt.Menu())
		}
		tt.Frame()
		if a.settings.StageStyle != c.want {
			t.Errorf("chose %s: style %q, want %q", c.item, a.settings.StageStyle, c.want)
		}
	}
}

// 火花 in the stage's menu turns the sparks on, and again off, whatever
// the style.
func TestStageSparksMenu(t *testing.T) {
	a, tt, _ := playingApp()
	a.nowPlaying, a.npPane = true, paneStage
	a.settings.StageStyle = "tilt"
	tt.Frame()
	for _, want := range []bool{true, false} {
		if err := tt.Click("律动效果"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem("火花"); err != nil {
			t.Fatalf("%v; the menu shows %q", err, tt.Menu())
		}
		tt.Frame()
		if a.settings.StageSparks != want || a.settings.StageStyle != "tilt" {
			t.Errorf("sparks %v in %q, want %v in tilt", a.settings.StageSparks, a.settings.StageStyle, want)
		}
	}
}

func words(texts ...string) []lyrics.Word {
	out := make([]lyrics.Word, len(texts))
	for i, s := range texts {
		out[i] = lyrics.Word{Text: s, Start: time.Duration(i) * time.Second, End: time.Duration(i+1) * time.Second}
	}
	return out
}

// A line breaks at its spaces and punctuation, punctuation staying with
// the word before it and a contraction whole, and a long run of
// characters in pieces.
func TestStageUnits(t *testing.T) {
	for _, c := range []struct {
		words []lyrics.Word
		want  [][2]int
	}{
		{words("从", "不 ", "主", "动 ", "示", "弱"), [][2]int{{0, 2}, {2, 4}, {4, 6}}},
		{words("Hello", ", ", "It", "’", "s ", "me"), [][2]int{{0, 2}, {2, 5}, {5, 6}}},
		{words("I ", "never ", "show"), [][2]int{{0, 1}, {1, 2}, {2, 3}}},
		{words("曾", "经", "的", "那", "些", "发", "生", "过"), [][2]int{{0, 4}, {4, 8}}},
	} {
		got := stageUnits(c.words)
		if len(got) != len(c.want) {
			t.Errorf("%v: units %v, want %v", c.words, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%v: units %v, want %v", c.words, got, c.want)
				break
			}
		}
	}
}

// 倾诉 splits a line into parts that follow one another, none empty, and
// rather after punctuation than nearest the middle.
func TestTiltSplitsAtPunctuation(t *testing.T) {
	ws := words("夜", "空", "中", "最", "亮", "的", "星，", "能", "否", "听", "清")
	units := stageUnits(ws)
	for n := 1; n <= min(4, len(units)); n++ {
		parts := splitParts(ws, units, n)
		if len(parts) != n || parts[0].from != 0 || parts[n-1].to != len(ws) {
			t.Fatalf("%d parts: %+v", n, parts)
		}
		for i := 1; i < n; i++ {
			if parts[i].from != parts[i-1].to || parts[i].from >= parts[i].to {
				t.Fatalf("%d parts: %+v", n, parts)
			}
		}
	}
	if parts := splitParts(ws, units, 2); parts[0].to != 7 {
		t.Errorf("split at %d, not after 星，", parts[0].to)
	}
}

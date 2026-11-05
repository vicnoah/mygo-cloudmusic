package netease

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Every level NetEase serves is known, ordered, and has the container the
// eapi expects.
func TestQualityLevels(t *testing.T) {
	want := []string{
		"standard", "higher", "exhigh", "lossless", "hires",
		"jyeffect", "sky", "jymaster", "dolby", "vivid",
	}
	if !reflect.DeepEqual(Levels, want) {
		t.Fatalf("levels %v, want %v", Levels, want)
	}
	for _, level := range Levels {
		q, ok := QualityOf(level)
		if !ok {
			t.Fatalf("%s has no quality", level)
		}
		if q.Label == "" || q.Short == "" && level != "standard" && level != "higher" && level != "exhigh" {
			t.Errorf("%s: label %q, short %q", level, q.Label, q.Short)
		}
	}
	if got := EncodeType("sky"); got != "flac" {
		t.Errorf("sky encode type %q", got)
	}
	if got := EncodeType("vivid"); got != "mp4" {
		t.Errorf("vivid encode type %q", got)
	}
	if !Spatial("sky") || !Spatial("vivid") || Spatial("jymaster") {
		t.Error("spatial flags are wrong")
	}
}

// A level falls back through lower levels, and a spatial level never falls
// straight to a compressed one.
func TestFallbackLevels(t *testing.T) {
	cases := map[string][]string{
		"standard": {"standard"},
		"higher":   {"higher", "standard"},
		"exhigh":   {"exhigh", "higher", "standard"},
		"lossless": {"lossless", "exhigh", "higher", "standard"},
		"hires":    {"hires", "lossless", "exhigh", "higher", "standard"},
		"jymaster": {"jymaster", "hires", "lossless", "exhigh", "higher", "standard"},
		"sky":      {"sky", "jyeffect", "hires", "lossless", "exhigh", "higher", "standard"},
		"vivid":    {"vivid", "dolby", "sky", "jyeffect", "hires", "lossless", "exhigh", "higher", "standard"},
	}
	for level, want := range cases {
		if got := FallbackLevels(level); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %v, want %v", level, got, want)
		}
	}
}

// AV3A is a valid NetEase stream type: it is Audio Vivid in an MP4 wrapper,
// decoded by the bundled helper. A previous parser rejected it as
// "unsupported format av3a" and silently fell back to a lower quality.
func TestAV3AStreamType(t *testing.T) {
	if !streamTypeSupported("av3a") {
		t.Fatal("av3a should be supported")
	}
	if streamTypeSupported("exe") {
		t.Fatal("exe should not be supported")
	}
}

// The response parser keeps the fields that decide the container, channel
// layout and whether the server capped the level.
func TestSongURLResponseFields(t *testing.T) {
	const body = `{"code":200,"data":[{"id":1,"url":"http://m8.music.126.net/a.flac","type":"flac","level":"sky","br":2139873,"sr":44100,"channelLayout":"5.1(side)","fee":0,"code":200}]}`
	var res songURLResponse
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Data) != 1 {
		t.Fatalf("data %d", len(res.Data))
	}
	d := res.Data[0]
	if d.Level != "sky" || d.Type != "flac" || d.BR != 2139873 || d.SR != 44100 || d.ChannelLayout != "5.1(side)" {
		t.Fatalf("parsed %+v", d.Stream)
	}
	if d.URL != "http://m8.music.126.net/a.flac" {
		t.Fatalf("url %q", d.URL)
	}
}

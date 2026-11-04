package subsonic

import (
	"strings"
	"testing"
	"time"

	gosubsonic "github.com/supersonic-app/go-subsonic/subsonic"
)

// Tests that NormalizeAddress handles schemes and trailing slashes correctly.
func TestNormalizeAddress(t *testing.T) {
	cases := []struct {
		input  string
		https  bool
		expect string
	}{
		{"http://127.0.0.1:4533/", false, "http://127.0.0.1:4533"},
		{"https://music.example.com///", false, "https://music.example.com"},
		{"192.168.1.50:4533", false, "http://192.168.1.50:4533"},
		{"192.168.1.50:4533", true, "https://192.168.1.50:4533"},
		{"", false, ""},
	}
	for _, tc := range cases {
		got := NormalizeAddress(tc.input, tc.https)
		if got != tc.expect {
			t.Errorf("NormalizeAddress(%q, %v) = %q, want %q", tc.input, tc.https, got, tc.expect)
		}
	}
}

// Tests that ID generates negative, non-zero IDs from GUIDs deterministically.
func TestIDGeneration(t *testing.T) {
	id1 := ID("song-guid-1")
	id2 := ID("song-guid-2")
	id1Again := ID("song-guid-1")

	if id1 >= 0 || id2 >= 0 {
		t.Fatalf("expected negative IDs, got %d and %d", id1, id2)
	}
	if id1 != id1Again {
		t.Fatalf("expected deterministic ID, got %d != %d", id1, id1Again)
	}
	if id1 == id2 {
		t.Fatalf("expected distinct IDs, got identical %d", id1)
	}
}

// Tests that ToTrack converts Subsonic Child metadata into netease.Track.
func TestToTrack(t *testing.T) {
	child := &gosubsonic.Child{
		ID:          "sub-song-123",
		Title:       "Test Track",
		Album:       "Test Album",
		AlbumID:     "sub-album-456",
		Artist:      "Test Artist",
		ArtistID:    "sub-artist-789",
		Duration:    185, // seconds
		CoverArt:    "cover-123",
		Suffix:      "flac",
		ContentType: "audio/flac",
		Starred:     time.Now(),
		Track:       3,
		DiscNumber:  1,
	}

	tr, ok := ToTrack(child)
	if !ok {
		t.Fatal("expected ToTrack to return true")
	}
	if tr.Ref != "sub-song-123" {
		t.Errorf("expected Ref %q, got %q", "sub-song-123", tr.Ref)
	}
	if tr.Name != "Test Track" {
		t.Errorf("expected Name %q, got %q", "Test Track", tr.Name)
	}
	if tr.Duration != 185000 {
		t.Errorf("expected Duration 185000ms, got %d", tr.Duration)
	}
	if tr.Format != "flac" {
		t.Errorf("expected Format flac, got %q", tr.Format)
	}
	if !tr.Fav {
		t.Errorf("expected Fav true")
	}
	if tr.Source != Source {
		t.Errorf("expected Source %q, got %q", Source, tr.Source)
	}
	if !strings.HasPrefix(tr.Album.PicURL, CoverScheme) {
		t.Errorf("expected CoverScheme prefix in PicURL, got %q", tr.Album.PicURL)
	}
}

// Tests that NormalizeFormat maps common MIME types to short extensions.
func TestNormalizeFormat(t *testing.T) {
	tests := map[string]string{
		"audio/flac": "flac",
		"audio/mpeg": "mp3",
		".mp3":       "mp3",
		"audio/mp4":  "m4a",
		"audio/ogg":  "ogg",
		"audio/wav":  "wav",
		"dsf":        "dsf",
	}
	for in, want := range tests {
		if got := NormalizeFormat(in); got != want {
			t.Errorf("NormalizeFormat(%q) = %q, want %q", in, got, want)
		}
	}
}

// Tests that Client tracks starred albums and handles sort count invalidation correctly.
func TestAlbumStarredState(t *testing.T) {
	c := NewClient(Config{})
	if c.IsAlbumStarred("al-1") {
		t.Fatalf("expected al-1 not to be starred initially")
	}
	if _, known := c.AlbumStarredKnown("al-1"); known {
		t.Fatalf("expected al-1 starred status to be unknown initially")
	}

	c.SetAlbumStarred("al-1", true)
	if !c.IsAlbumStarred("al-1") {
		t.Fatalf("expected al-1 to be starred")
	}
	if st, known := c.AlbumStarredKnown("al-1"); !known || !st {
		t.Fatalf("expected al-1 starred known and true, got known=%v st=%v", known, st)
	}

	c.SetSortAlbumCount("starred", 42)
	if cnt, ok := c.SortAlbumCount("starred"); !ok || cnt != 42 {
		t.Fatalf("expected starred count 42, got %d (ok=%v)", cnt, ok)
	}

	c.InvalidateSortCount("starred")
	if _, ok := c.SortAlbumCount("starred"); ok {
		t.Fatalf("expected starred count to be invalidated")
	}
}

// Tests that formatStructuredLyrics unescapes HTML/XML character entities in line text.
func TestFormatStructuredLyricsUnescape(t *testing.T) {
	sl := &gosubsonic.StructuredLyrics{
		Lines: []gosubsonic.LyricLine{
			{
				Start: 10000,
				Text:  "Don&apos;t say you&quot;re &amp; I&#39;m &lt;fine&gt;",
			},
			{
				Start: 20000,
				Value: "Tom&nbsp;&amp;&nbsp;Jerry",
			},
		},
	}
	got := formatStructuredLyrics(sl)
	want := "[00:10.00]Don't say you\"re & I'm <fine>\n[00:20.00]Tom\u00a0&\u00a0Jerry\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

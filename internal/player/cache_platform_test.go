package player

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/waygu-music-plugin/sdk/pluginapi"
)

// Identical references from different instances keep their own audio and decoder paths.
func TestCacheScopesAudioByPlatform(t *testing.T) {
	c := NewCache(t.TempDir(), 400)
	p := &Player{Cache: c, PlatformPlayer: PlatformPlayer{
		ResolveStream: func(context.Context, *pluginapi.Track, string) (*pluginapi.Stream, error) {
			return nil, errors.New("the cached next song should be staged without a remote request")
		},
	}}
	track := &pluginapi.Track{ID: 17, PlatformTrack: pluginapi.PlatformTrack{Ref: "A/17", Format: "mp3"}}
	for _, fixture := range []struct {
		owner pluginapi.ID
		sound byte
	}{
		{"library-one", 11},
		{"library-two", 22},
	} {
		ref := trackCacheRef(fixture.owner, track)
		st := &pluginapi.Stream{Type: "mp3", Level: "exhigh"}
		f := c.part(st)
		if f == nil {
			t.Fatal("cache did not create a partial file")
		}
		if _, err := f.Write([]byte{fixture.sound}); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if _, ok := c.keep(ref, st, f.Name(), 1); !ok {
			t.Fatalf("cache did not keep %q", fixture.owner)
		}
	}
	for owner, sound := range map[pluginapi.ID]byte{"library-one": 11, "library-two": 22} {
		track.Source = string(owner)
		staged, ok := p.openNextOutput(t.Context(), *track, "exhigh")
		if !ok || staged.localFile == "" || staged.info.URL != "" || staged.info.Type != "mp3" {
			t.Fatalf("cached next song from %q was not prepared for external output", owner)
		}
		data, err := os.ReadFile(staged.location())
		if err != nil || len(data) != 1 || data[0] != sound {
			t.Fatalf("decoder file for %q contains %v, %v", owner, data, err)
		}
	}
	if got := c.filePath(trackCacheRef("library-three", track), "exhigh"); got != "" {
		t.Fatalf("a third instance matched %s", got)
	}
}

// Queue appends keep equal service IDs from two instances and deduplicate repeated lookups of one raw reference.
func TestAppendQueueKeepsTrackInstances(t *testing.T) {
	p := &Player{}
	from := pluginapi.Track{ID: 17, PlatformTrack: pluginapi.PlatformTrack{Source: "first", Ref: "shared/path"}}
	other := from
	other.Source = "second"
	alias := from
	alias.ID = 18
	p.AppendQueue([]pluginapi.Track{from, other, alias})
	queue := p.Queue()
	if len(queue) != 2 || queue[0].Source != from.Source || queue[1].Source != other.Source {
		t.Fatalf("queue did not preserve the two track owners: %+v", queue)
	}
}

// Numeric cache files without an owner are preserved without serving another platform's audio.
func TestCachePreservesUnownedLegacyFile(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir, 400)
	media := filepath.Join(dir, "17-exhigh.mp3")
	if err := os.WriteFile(media, []byte{4}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "17-exhigh.json"), []byte(`{"type":"mp3","level":"exhigh","size":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	track := &pluginapi.Track{ID: 17}
	if got := c.filePath(trackCacheRef("new", track), "exhigh"); got != "" {
		t.Fatalf("the decoder found unowned audio at %s", got)
	}
	if _, _, ok := c.lookup(trackCacheRef("new", track), "exhigh"); ok {
		t.Fatal("the current plugin received unowned audio")
	}
	if got, err := os.ReadFile(media); err != nil || len(got) != 1 || got[0] != 4 {
		t.Fatalf("the old audio changed: %v, %v", got, err)
	}
}

// A resolved stream can adopt matching old audio for the decoder and later reuse it offline.
func TestLegacyCacheVerifiesBeforeDecodingAndReuse(t *testing.T) {
	audio, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	legacyFile := writeLegacyCache(t, audio)
	c := NewCache(filepath.Dir(legacyFile), int64(len(audio))*8)
	p := &Player{Cache: c, PlatformPlayer: PlatformPlayer{
		ResolveStream: func(context.Context, *pluginapi.Track, string) (*pluginapi.Stream, error) {
			return &pluginapi.Stream{
				URL: "unreachable://no-audio-download", Type: "mp3", Level: "exhigh", Size: int64(len(audio)),
				VerifyCachedContent: func(r io.Reader) bool {
					got, err := io.ReadAll(r)
					return err == nil && bytes.Equal(got, audio)
				},
			}, nil
		},
	}}
	track := pluginapi.Track{ID: 17, Duration: 5000, PlatformTrack: pluginapi.PlatformTrack{Source: "library-one", Ref: "A/17"}}
	media, err := p.open(t.Context(), track, "exhigh", false)
	if err != nil {
		t.Fatal(err)
	}
	defer media.decoded.Close()
	samples := make([]float32, 64)
	if n, err := media.decoded.Read(samples); n == 0 || err != nil {
		t.Fatalf("verified old cache did not decode: n=%d err=%v", n, err)
	}
	for _, path := range []string{legacyFile, filepath.Join(c.dir, "17-exhigh.json")} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("the legacy cache entry at %s was not moved: %v", path, err)
		}
	}

	p.ResolveStream = func(context.Context, *pluginapi.Track, string) (*pluginapi.Stream, error) {
		return nil, errors.New("offline")
	}
	staged, ok := p.openNextOutput(t.Context(), track, "exhigh")
	if !ok || staged.localFile == "" {
		t.Fatal("external output could not stage the migrated file offline")
	}
	got, err := os.ReadFile(staged.localFile)
	if err != nil || !bytes.Equal(got, audio) {
		t.Fatalf("migrated audio differs from the original: %v", err)
	}
	track.Source = "library-two"
	if _, ok := p.openNextOutput(t.Context(), track, "exhigh"); ok {
		t.Fatal("another platform used the migrated audio")
	}
}

// A digest mismatch with the same numeric ID and byte count passes the resolved URL to external output.
func TestLegacyCacheMismatchKeepsStreamAndOldFile(t *testing.T) {
	legacyFile := writeLegacyCache(t, []byte("other audio"))
	c := NewCache(filepath.Dir(legacyFile), 1024)
	p := &Player{Cache: c, PlatformPlayer: PlatformPlayer{
		ResolveStream: func(context.Context, *pluginapi.Track, string) (*pluginapi.Stream, error) {
			return &pluginapi.Stream{
				URL: "https://example.test/record.mp3", Type: "mp3", Level: "exhigh", Size: int64(len("right audio")),
				VerifyCachedContent: func(r io.Reader) bool {
					b, err := io.ReadAll(r)
					return err == nil && string(b) == "right audio"
				},
			}, nil
		},
	}}
	track := pluginapi.Track{ID: 17, PlatformTrack: pluginapi.PlatformTrack{Source: "library-two"}}
	staged, ok := p.openNextOutput(t.Context(), track, "exhigh")
	if !ok || staged.localFile != "" || staged.location() != "https://example.test/record.mp3" {
		t.Fatalf("mismatched old audio replaced the resolved stream: %+v", staged)
	}
	b, err := os.ReadFile(legacyFile)
	if err != nil || string(b) != "other audio" {
		t.Fatalf("the legacy file was altered: %q %v", b, err)
	}
}

// A platform switch during verification leaves the legacy record available for later playback.
func TestLegacyCacheCancellationLeavesOldFile(t *testing.T) {
	legacyFile := writeLegacyCache(t, []byte("old audio"))
	c := NewCache(filepath.Dir(legacyFile), 1024)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	track := &pluginapi.Track{ID: 17, PlatformTrack: pluginapi.PlatformTrack{Source: "old"}}
	ref := trackCacheRef("old", track)
	info := &pluginapi.Stream{Type: "mp3", Level: "exhigh", Size: int64(len("old audio"))}
	info.VerifyCachedContent = func(r io.Reader) bool {
		b, err := io.ReadAll(r)
		cancel()
		return err == nil && string(b) == "old audio"
	}
	if c.promoteLegacy(ctx, ref, track.ID, info) || c.filePath(ref, "exhigh") != "" {
		t.Fatal("the canceled request published the migrated audio")
	}
	if _, err := os.Stat(legacyFile); err != nil {
		t.Fatalf("the canceled request removed the old audio: %v", err)
	}
}

func writeLegacyCache(t *testing.T, audio []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "17-exhigh.mp3")
	if err := os.WriteFile(path, audio, 0o600); err != nil {
		t.Fatal(err)
	}
	meta := fmt.Sprintf(`{"type":"mp3","level":"exhigh","size":%d}`, len(audio))
	if err := os.WriteFile(filepath.Join(dir, "17-exhigh.json"), []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

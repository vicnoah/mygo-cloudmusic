package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"waygu-music/internal/pluginapi"
)

// Configured artwork fetchers handle platform-private cover URLs before decoding.
func TestLoadArtWithPlatformFetcher(t *testing.T) {
	b, err := os.ReadFile("../pluginapi/artwork/testdata/animated.webp")
	if err != nil {
		t.Fatal(err)
	}
	called := ""
	_, img, _, err := loadArtWith("service://cover/abc", func(url string) ([]byte, error) { called = url; return b, nil })
	if err != nil {
		t.Fatal(err)
	}
	if called != "service://cover/abc" || img.Bounds().Dx() != 24 {
		t.Fatalf("fetch=%q image=%v", called, img.Bounds())
	}
}

// A cached legacy cover cannot satisfy the same cover URL requested by another plugin.
func TestArtworkCacheKeepsLegacyCoversByPlatform(t *testing.T) {
	b, err := os.ReadFile("../pluginapi/artwork/testdata/animated.webp")
	if err != nil {
		t.Fatal(err)
	}
	a := newTestApp()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	const cover = "service://cover/identical-id"
	fetched := make(chan pluginapi.ID, 2)
	for _, id := range []pluginapi.ID{"first", "second"} {
		a.platformRegistrations = append(a.platformRegistrations, testRegistration(id, func(pluginapi.Startup) pluginapi.Adapter {
			return testArtworkAdapter{Adapter: testPluginAdapter{}, fetch: func(string) ([]byte, bool, error) {
				fetched <- id
				return b, true, nil
			}}
		}))
	}
	posted := make(chan func(), 2)
	a.art = newArtCache(func(fn func()) { posted <- fn })
	a.initPlatformState()
	a.configurePlatformPlayer()
	a.activatePlatformView()
	defer a.deactivatePlatformView(false)
	host := platformHost{a: a}
	awaitCover := func(owner pluginapi.ID) {
		t.Helper()
		select {
		case fn := <-posted:
			fn()
		case <-time.After(2 * time.Second):
			t.Fatalf("%s cover did not load", owner)
		}
		select {
		case got := <-fetched:
			if got != owner {
				t.Fatalf("cover request went to %q instead of %q", got, owner)
			}
		default:
			t.Fatalf("no cover request reached %q", owner)
		}
	}

	host.Artwork(cover, 96)
	awaitCover("first")
	first, _ := host.Artwork(cover, 96)
	if first == nil {
		t.Fatal("the first cover could not be read from the cache")
	}
	a.setPlatform("second")
	if second := host.DecodedArtwork(cover, 96, false); second != nil {
		t.Fatal("the second platform received the first platform's cached cover")
	}
	awaitCover("second")
	second := host.DecodedArtwork(cover, 96, false)
	if second == nil || second.Bitmap() == nil || second.Bitmap() == first {
		t.Fatal("the second platform did not get its own cover")
	}
	a.setPlatform("first")
	if restored, _ := host.Artwork(cover, 96); restored != first {
		t.Fatal("returning to the first platform did not reuse its cover")
	}
}

// A cover response from the previous platform is discarded after a platform switch.
func TestArtCacheDropsLatePlatformResponse(t *testing.T) {
	b, err := os.ReadFile("../pluginapi/artwork/testdata/animated.webp")
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	posted := make(chan func(), 1)
	generation := uint64(1)
	c := newArtCache(func(fn func()) { posted <- fn })
	c.generation = func() uint64 { return generation }
	c.fetch = func() func(string) ([]byte, error) {
		return func(string) ([]byte, error) {
			close(started)
			<-release
			return b, nil
		}
	}
	c.get("service://cover/old", "service://cover/old", false)
	<-started
	generation = 2
	close(release)
	postedFn := <-posted
	postedFn()
	if _, ok := c.art["service://cover/old"]; ok {
		t.Fatal("late cover response populated the new platform cache")
	}
}

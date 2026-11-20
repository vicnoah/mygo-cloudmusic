package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/pluginapi/artwork"
)

// A cached legacy cover cannot satisfy the same cover URL requested by another plugin.
func TestArtworkCacheKeepsLegacyCoversByPlatform(t *testing.T) {
	b, err := os.ReadFile("testdata/animated.webp")
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
	a.art = artwork.NewCache(func(fn func()) { posted <- fn })
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

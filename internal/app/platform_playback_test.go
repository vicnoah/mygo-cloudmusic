package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/network"
)

type testStreamAdapter struct {
	pluginapi.Adapter
	id          pluginapi.ID
	preferences pluginapi.AudioPreferences
}

func (a testStreamAdapter) OwnsTrack(tr *pluginapi.Track) bool {
	return tr.Source == string(a.id) || tr.Source == "legacy-instance"
}

func (a testStreamAdapter) PlaybackMedia() pluginapi.PlaybackMedia {
	return pluginapi.PlaybackMedia{
		Preferences: func() pluginapi.AudioPreferences { return a.preferences },
		Stream: func(context.Context, *pluginapi.Track, string) (*pluginapi.Stream, error) {
			return &pluginapi.Stream{URL: string(a.id)}, nil
		},
	}
}

// Cutting to a track from a different instance uses that instance's quality while the active platform stays selected.
func TestPlaybackUsesTrackOwnerQuality(t *testing.T) {
	a := newTestApp()
	a.platformID = "first-instance"
	a.platforms = map[pluginapi.ID]pluginapi.Adapter{
		"first-instance":  testStreamAdapter{id: "first-instance", preferences: pluginapi.AudioPreferences{Quality: "standard"}},
		"second-instance": testStreamAdapter{id: "second-instance", preferences: pluginapi.AudioPreferences{Quality: "lossless", Normalize: true}},
	}
	a.configurePlatformPlayer()
	defer a.pl.DLNA().Close()
	defer a.pl.Reset()
	type streamRequest struct{ source, quality string }
	requests := make(chan streamRequest, 2)
	a.pl.ResolveStream = func(_ context.Context, tr *pluginapi.Track, level string) (*pluginapi.Stream, error) {
		requests <- streamRequest{tr.Source, level}
		return nil, errors.New("skip decoding synthetic test tracks")
	}
	a.pl.PlayQueue([]pluginapi.Track{
		{ID: 42, PlatformTrack: pluginapi.PlatformTrack{Source: "first-instance"}},
		{ID: 42, PlatformTrack: pluginapi.PlatformTrack{Source: "second-instance"}},
	}, 0)
	await := func(want streamRequest) {
		t.Helper()
		select {
		case got := <-requests:
			if got != want {
				t.Fatalf("stream request = %+v, want %+v", got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("stream resolver never received %+v", want)
		}
	}
	await(streamRequest{"first-instance", "standard"})
	a.pl.Next()
	await(streamRequest{"second-instance", "lossless"})
}

// A stream request still goes to its own instance when a different instance is active.
func TestStreamResolvesByTrackPlatform(t *testing.T) {
	a := newTestApp()
	a.platformID = "first-instance"
	a.platforms = map[pluginapi.ID]pluginapi.Adapter{
		"legacy-instance": testStreamAdapter{id: "legacy-instance"},
		"first-instance":  testStreamAdapter{id: "first-instance"},
		"second-instance": testStreamAdapter{id: "second-instance"},
	}
	a.configurePlatformPlayer()

	for _, id := range []pluginapi.ID{"legacy-instance", "second-instance"} {
		track := &pluginapi.Track{ID: 42, PlatformTrack: pluginapi.PlatformTrack{Source: string(id)}}
		stream, err := a.pl.ResolveStream(context.Background(), track, "lossless")
		if err != nil || stream == nil || stream.URL != string(id) {
			t.Fatalf("resolve %q with another instance active = %+v, %v", id, stream, err)
		}
	}
}

type testArtworkAdapter struct {
	pluginapi.Adapter
	fetch func(string) ([]byte, bool, error)
}

func (a testArtworkAdapter) FetchArtwork(_ context.Context, ref string, _ int) ([]byte, bool, error) {
	return a.fetch(ref)
}

// The same server-side cover ID loads separately from each of its owning instances.
func TestArtworkResolvesByPlatformInstance(t *testing.T) {
	a := newTestApp()
	a.platformID = "first-instance"
	a.platforms = make(map[pluginapi.ID]pluginapi.Adapter)
	host := platformHost{a: a}
	const sharedRef = "service://cover/local%2Fcover%3F1"
	for _, id := range []pluginapi.ID{"first-instance", "second-instance"} {
		a.platforms[id] = testArtworkAdapter{fetch: func(ref string) ([]byte, bool, error) {
			if ref != sharedRef {
				t.Errorf("%s received artwork reference %q, want %q", id, ref, sharedRef)
			}
			return []byte(id), true, nil
		}}
	}
	for _, id := range []pluginapi.ID{"second-instance", "first-instance"} {
		key := network.SizedArtworkURL(pluginapi.ArtworkURL(id, sharedRef), 96)
		got, err := host.fetchArtwork(context.Background(), a.platformID, key)
		if err != nil || string(got) != string(id) {
			t.Fatalf("artwork for %s with %s active = %q, %v", id, a.platformID, got, err)
		}
	}
	if _, err := host.fetchArtwork(context.Background(), a.platformID, pluginapi.ArtworkURL("unavailable-instance", sharedRef)); err == nil {
		t.Fatal("artwork for an unavailable instance used another instance")
	}
	if _, err := host.fetchArtwork(context.Background(), "unavailable-instance", sharedRef); err == nil {
		t.Fatal("unscoped artwork was fetched from an unrelated instance")
	}
}

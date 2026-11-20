package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/protocol"
)

// A queued write could clear the current search history after switching away and back.
func TestPlatformSharedWritesFollowActivation(t *testing.T) {
	a := newTestApp()
	if ctrl := a.pl.DLNA(); ctrl != nil {
		t.Cleanup(func() { _ = ctrl.Close() })
	}
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.platformRegistrations = []pluginapi.Registration{testRegistration("first", nil), testRegistration("second", nil)}
	if !a.initPlatformState() {
		t.Fatal("no platform was registered")
	}
	a.activatePlatformView()
	t.Cleanup(func() { a.deactivatePlatformView(false) })
	first := platformHost{a: a, id: "first"}
	second := platformHost{a: a, id: "second"}
	first.SetVolume(0.4)
	first.RememberGlobalSearch("saved search")
	previousPlayback := first.Playback()
	queued := first.activationCallback(func() {
		first.ClearSearchHistory()
		first.SetVolume(0.9)
	})
	a.setPlatform("second")
	second.SetVolume(0.3)
	second.Toast("active notification")
	first.SetVolume(0.1)
	first.SetDesktopLyricsStyle(false, 50, 1)
	preferences := first.DesktopSettings().Preferences
	preferences.Fog = true
	first.ApplyDesktopPreferences(preferences)
	first.ClearSearchHistory()
	first.Toast("inactive notification")
	if a.volume != 0.3 || a.settings.DesktopLyrics != nil || a.settings.Fog || first.TakeToast() != "" || second.TakeToast() != "active notification" {
		t.Fatal("the inactive instance changed the active instance's desktop state")
	}
	a.setPlatform("first")
	queued()
	previousPlayback.ToggleShuffle()
	if a.pl.Shuffle() {
		t.Fatal("a previous activation kept its playback controls")
	}
	if got := first.GlobalSearchHistory(); len(got) != 1 || got[0] != "saved search" || a.volume != 0.3 {
		t.Fatalf("an earlier activation changed the search history or volume: %v, %f", got, a.volume)
	}
	first.activationCallback(func() { first.SetVolume(0.6) })()
	if a.volume != 0.6 {
		t.Fatal("the current activation could not write")
	}
}

// The in-process player could enqueue another instance's tracks, which the RPC player already rejected.
func TestPlatformQueueCommandsRespectInstance(t *testing.T) {
	for _, mode := range []string{"in-process", "rpc"} {
		t.Run(mode, func(t *testing.T) {
			a := newTestApp()
			if ctrl := a.pl.DLNA(); ctrl != nil {
				t.Cleanup(func() { _ = ctrl.Close() })
			}
			a.platformRegistrations = []pluginapi.Registration{
				testRegistration("first", func(pluginapi.Startup) pluginapi.Adapter { return testStreamAdapter{id: "first"} }),
				testRegistration("second", func(pluginapi.Startup) pluginapi.Adapter { return testStreamAdapter{id: "second"} }),
			}
			if !a.initPlatformState() {
				t.Fatal("no platform was registered")
			}
			a.activatePlatformView()
			t.Cleanup(func() {
				a.deactivatePlatformView(false)
				a.pl.Reset()
			})
			port := (platformHost{a: a, id: "first"}).Player()
			scope := a.platformScope
			reverse := pluginReverse{app: a, connection: scope.Connection}
			apply := func(action string, track pluginapi.Track, index int, valid bool) {
				t.Helper()
				tracks := []pluginapi.Track{track}
				if mode == "rpc" {
					err := reverse.apply(PluginHostCall{Action: action, Tracks: tracks, Index: index, Scope: scope}, &PluginHostReply{})
					var failure protocol.Failure
					if (valid && err != nil) || (!valid && (!errors.As(err, &failure) || failure.Code != "invalid-queue")) {
						t.Fatalf("%s returned %v for %s", action, err, track.Source)
					}
				} else if action == "play" {
					port.PlayQueue(tracks, index)
				} else {
					port.AppendQueue(tracks)
				}
			}
			local := pluginapi.Track{ID: 10, PlatformTrack: pluginapi.PlatformTrack{Source: "first"}}
			foreign := pluginapi.Track{ID: 20, PlatformTrack: pluginapi.PlatformTrack{Source: "second"}}
			apply("play", local, 0, true)
			apply("append", foreign, 0, false)
			apply("play", foreign, 0, false)
			local.ID = 11
			apply("play", local, -1, false)
			current := a.pl.Current()
			if got := a.pl.Queue(); len(got) != 1 || got[0].ID != 10 || current == nil || current.ID != 10 {
				t.Fatalf("a rejected command changed playback: %+v", got)
			}
			apply("append", local, 0, true)
			if got := a.pl.Queue(); len(got) != 2 || got[1].ID != 11 {
				t.Fatalf("the active instance could not append its own song: %+v", got)
			}
			if mode == "in-process" {
				port.PlayList([]pluginapi.Track{local}, 0, func(_ context.Context, _ pluginapi.QueueFeedCursor, done func([]pluginapi.Track, error)) {
					done([]pluginapi.Track{foreign}, nil)
				})
				if got := a.pl.Queue(); len(got) != 1 || got[0].ID != local.ID {
					t.Fatalf("a foreign refill changed playback: %+v", got)
				}
			}
		})
	}
}

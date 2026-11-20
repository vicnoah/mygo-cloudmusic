package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"github.com/waygu-music-plugin/sdk/bridge"
	"github.com/waygu-music-plugin/sdk/platformrpc"
	"github.com/waygu-music-plugin/sdk/pluginapi"
)

// A shared Subsonic source used to be assigned to whichever server the listener switched to.
func TestLegacySubsonicSessionKeepsOriginalProfile(t *testing.T) {
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	old := []byte(`{"queue":[{"id":7,"name":"Saved on Jellyfin","dt":5000,"src":"subsonic","ref":"same-on-two-servers"}],"order":[0],"at":0,"position":1200}`)
	path := filepath.Join(dir, "session.json")
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"platform":"jellyfin"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	launch := func(entries ...pluginapi.ID) *app {
		t.Helper()
		a := newTestApp()
		for _, id := range entries {
			a.platformRegistrations = append(a.platformRegistrations, testRegistration(id, func(pluginapi.Startup) pluginapi.Adapter {
				return testStreamAdapter{id: id}
			}))
		}
		a.settings = loadSettings(a.platformRegistrations...)
		if !a.initPlatformState() {
			t.Fatal("no platform activated")
		}
		a.activatePlatformView()
		t.Cleanup(func() {
			a.deactivatePlatformView(false)
			_ = a.pl.DLNA().Close()
		})
		return a
	}
	load := func(a *app) (pluginapi.PlaybackSession, bool) {
		return (playbackContinuity{a: a, ctx: a.platformContext}).LoadSession()
	}
	reject := func(a *app) {
		t.Helper()
		if session, ok := load(a); ok {
			t.Fatalf("%s received Jellyfin's legacy session: %+v", a.platformID, session)
		}
	}

	fallback := launch("fnmusic")
	reject(fallback)
	fallback.settings.Appearance = "dark"
	if err := fallback.settings.save(); err != nil {
		t.Fatal(err)
	}
	a := launch("fnmusic", "navidrome", "jellyfin", "subsonic")
	a.setPlatform("navidrome")
	reject(a)
	a = launch("fnmusic", "navidrome", "jellyfin", "subsonic")
	reject(a)
	a.setPlatform("subsonic")
	reject(a)
	a.setPlatform("jellyfin")
	session, ok := load(a)
	if !ok || len(session.Queue) != 1 || session.Queue[0].Source != "jellyfin" || session.Position != 1200 {
		t.Fatalf("Jellyfin lost its original paused session: %+v", session)
	}
	if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, old) {
		t.Fatalf("the legacy session needed by older releases was changed: %v", err)
	}
	replacement := bytes.Replace(old, []byte("Saved on Jellyfin"), []byte("Unattributed replacement"), 1)
	if err := os.WriteFile(path, replacement, 0o600); err != nil {
		t.Fatal(err)
	}
	a = launch("fnmusic", "navidrome", "jellyfin", "subsonic")
	reject(a)
}

// A saved queue is offered once to its owner and restored paused, including old tracks without a source.
func TestLegacySessionOfferedToActiveOwner(t *testing.T) {
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	defer mygo.App.SetPath(mygo.PathUserData, "")
	old := []byte(`{"queue":[{"id":7,"name":"先前的歌","dt":180000},{"id":8,"name":"宝船","dt":200000}],"order":[0,1],"at":1,"position":48000,"radio":"radio:recent"}`)
	if err := os.WriteFile(filepath.Join(dir, "session.json"), old, 0o600); err != nil {
		t.Fatal(err)
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.Platform = "owner"
	var owner pluginapi.PlaybackContinuity
	a.platformRegistrations = []pluginapi.Registration{
		testRegistration("owner", func(s pluginapi.Startup) pluginapi.Adapter {
			s.RegisterLifecycle(pluginapi.Lifecycle{Activate: func(_ context.Context, continuity pluginapi.PlaybackContinuity) {
				owner = continuity
			}})
			return legacyTrackAdapter{}
		}),
		testRegistration("inactive", nil),
	}
	if !a.initPlatformState() {
		t.Fatal("no active plugin was loaded")
	}
	a.activatePlatformView()
	defer a.deactivatePlatformView(false)
	a.pl.ResolveStream = func(ctx context.Context, _ *pluginapi.Track, _ string) (*pluginapi.Stream, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	a.pl.ResolveLyrics, a.pl.Artwork = nil, nil
	defer a.pl.Reset()
	session, ok := owner.LoadSession()
	if !ok || session.Radio != "radio:recent" {
		t.Fatalf("legacy session was not offered to its owner: %+v", session)
	}
	owner.Restore(session)
	if current := a.pl.Current(); current == nil || current.ID != 8 || a.pl.Playing() || a.pl.Position() != 48*time.Second {
		t.Fatalf("restored %v at %v, playing=%t", current, a.pl.Position(), a.pl.Playing())
	}
	if _, ok := owner.LoadSession(); ok {
		t.Fatal("the startup queue was offered twice")
	}
	a.cancelPlatform()
	owner.AttachRadio("deactivated", nil)
	if a.pl.Radio() != "" {
		t.Fatal("a plugin attached an old radio feed after deactivating")
	}
}

// Preparing or rejecting a remote plugin preserves playback and revokes any queued writes from the candidate.
func TestRejectedPluginCandidateCannotReplacePlayback(t *testing.T) {
	a := newTestApp()
	t.Cleanup(func() { _ = a.pl.DLNA().Close() })
	a.platformRegistrations = []pluginapi.Registration{testRegistration("first", nil)}
	a.initPlatformState()
	a.activatePlatformView()
	defer a.deactivatePlatformView(false)
	a.pl.Cue([]pluginapi.Track{{ID: 9, Name: "Original", Duration: 30000}})
	local := (platformHost{a: a, id: a.platformID}).Player()
	scope := a.pluginActivations.Prepare("remote")
	reverse := pluginReverse{app: a, connection: "remote"}
	request := PluginHostCall{Scope: scope, Action: "resume-pause"}
	var reply PluginHostReply
	if got := bridge.Failure("", reverse.apply(request, &reply)); got.Code != "not-active" {
		t.Fatalf("candidate command = %+v", got)
	}
	a.pluginActivations.Revoke(scope)
	if a.pluginActivations.Commit(scope) {
		t.Fatal("a rejected candidate was committed")
	}
	if got := bridge.Failure("", reverse.apply(request, &reply)); got.Code != "stale-activation" {
		t.Fatalf("late command = %+v", got)
	}
	local.SetShuffle(true)
	if a.platformScope == (platformrpc.Scope{}) || a.pl.Current().ID != 9 || a.pl.Playing() || !a.pl.Shuffle() {
		t.Fatal("candidate changed the original playback or revoked its controls")
	}
}

// A delayed restore or radio request cannot change playback after the same plugin is opened again.
func TestPlaybackContinuityExpiresWhenPluginReturns(t *testing.T) {
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	a := newTestApp()
	a.settings.path = filepath.Join(dir, "settings.json")
	var continuity pluginapi.PlaybackContinuity
	a.platformRegistrations = []pluginapi.Registration{
		testRegistration("first", func(start pluginapi.Startup) pluginapi.Adapter {
			start.RegisterLifecycle(pluginapi.Lifecycle{Activate: func(_ context.Context, active pluginapi.PlaybackContinuity) {
				continuity = active
			}})
			return testStreamAdapter{id: "first"}
		}),
		testRegistration("second", nil),
	}
	a.initPlatformState()
	a.activatePlatformView()
	t.Cleanup(func() {
		a.deactivatePlatformView(false)
		a.pl.Reset()
	})
	a.pl.ResolveStream = func(ctx context.Context, _ *pluginapi.Track, _ string) (*pluginapi.Stream, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	a.pl.ResolveLyrics, a.pl.Artwork = nil, nil
	previous := continuity

	a.setPlatform("second")
	a.setPlatform("first")
	a.pl.Cue([]pluginapi.Track{{ID: 77}})
	saved := pluginapi.PlaybackSession{
		Queue: []pluginapi.Track{{ID: 39, Duration: 20000, PlatformTrack: pluginapi.PlatformTrack{Source: "first"}}},
		Order: []int{0}, Position: 2500,
	}
	previous.Restore(saved)
	previous.AttachRadio("old-radio", nil)
	if current := a.pl.Current(); current == nil || current.ID != 77 || a.pl.Radio() != "" {
		t.Fatalf("the prior activation replaced the queue or radio: track=%v radio=%q", current, a.pl.Radio())
	}
	if _, err := previous.OpenAudio(); !errors.Is(err, context.Canceled) {
		t.Fatalf("the prior activation could access playback audio: %v", err)
	}

	continuity.Restore(saved)
	continuity.AttachRadio("current-radio", nil)
	if current := a.pl.Current(); current == nil || current.ID != 39 ||
		a.pl.Playing() || a.pl.Position() != 2500*time.Millisecond || a.pl.Radio() != "current-radio" {
		t.Fatalf("the current activation could not restore playback: track=%v position=%v radio=%q",
			current, a.pl.Position(), a.pl.Radio())
	}
}

// Switching plugins clears active playback, keeps each plugin's saved queue, and revokes scheduled writes.
func TestPlatformSwitchKeepsSavedQueuesSeparate(t *testing.T) {
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	defer mygo.App.SetPath(mygo.PathUserData, "")
	var scheduled []func()
	after := sessionAfter
	defer func() { sessionAfter = after }()
	sessionAfter = func(_ time.Duration, fn func()) { scheduled = append(scheduled, fn) }
	a := newTestApp()
	a.platformRegistrations = []pluginapi.Registration{testRegistration("first", nil), testRegistration("second", nil)}
	a.initPlatformState()
	a.activatePlatformView()
	a.pl.Cue([]pluginapi.Track{{ID: 7, Name: "宝船", Duration: 200000}})
	a.keepSession()
	a.setPlatform("second")
	defer a.deactivatePlatformView(false)
	if len(a.pl.Queue()) != 0 || a.pl.Current() != nil {
		t.Fatal("the newly active plugin retained the previous playback")
	}
	firstPath, err := platformSessionPath("first")
	if err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	a.pl.Cue([]pluginapi.Track{{ID: 9, Duration: 200000}})
	a.keepSession()
	if len(scheduled) != 2 {
		t.Fatalf("expected each activation to schedule a save, got %d callbacks", len(scheduled))
	}
	path, err := platformSessionPath("second")
	if err != nil {
		t.Fatal(err)
	}
	scheduled[0]()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the prior activation wrote a session after switching: %v", err)
	}
	scheduled[1]()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var session pluginapi.PlaybackSession
	if err := json.Unmarshal(data, &session); err != nil {
		t.Fatal(err)
	}
	if len(session.Queue) != 1 || session.Queue[0].ID != 9 {
		t.Fatalf("saved queue did not match the newly active plugin: %+v", session.Queue)
	}
	if err := json.Unmarshal(first, &session); err != nil || len(session.Queue) != 1 || session.Queue[0].ID != 7 {
		t.Fatalf("previous plugin lost its saved queue: %+v, %v", session.Queue, err)
	}
}

// The session is kept a while after playback changes and every while as
// it plays, and paused nothing more is due. It was kept by a ticker that
// woke the app, and redrew its window, every half minute even at rest.
func TestSessionRestsWhilePaused(t *testing.T) {
	var due []func()
	defer func(after func(time.Duration, func())) { sessionAfter = after }(sessionAfter)
	sessionAfter = func(_ time.Duration, fn func()) { due = append(due, fn) }

	a := newTestApp()
	a.pl.Changed = a.keepSession
	a.pl.Cue([]pluginapi.Track{{ID: 7, Name: "宝船", Duration: 200000}})
	out := audio.NewPlayer(&silence{})
	out.SetVolume(0)
	out.Play()
	a.pl.Attach(out)
	defer out.Close()

	a.pl.TogglePlay()
	a.pl.TogglePlay()
	if len(due) != 1 {
		t.Fatalf("%d saves due after pausing and playing", len(due))
	}
	due[0]()
	if len(due) != 2 {
		t.Fatalf("playing, %d saves due after one, want another", len(due)-1)
	}

	a.pl.TogglePlay()
	due[1]()
	if len(due) != 2 {
		t.Errorf("paused, %d more saves due", len(due)-2)
	}
}

type legacyTrackAdapter struct{ testPluginAdapter }

func (legacyTrackAdapter) OwnsTrack(t *pluginapi.Track) bool { return t != nil && t.Source == "" }

// silence is a song of silence, to play without a stream.
type silence struct{ pos int64 }

func (s *silence) SampleRate() int           { return 48000 }
func (s *silence) Channels() int             { return 2 }
func (s *silence) Len() int64                { return 48000 * 200 }
func (s *silence) SetPosition(f int64) error { s.pos = f; return nil }
func (s *silence) Read(buf []float32) (int, error) {
	clear(buf)
	s.pos += int64(len(buf) / 2)
	return len(buf), nil
}

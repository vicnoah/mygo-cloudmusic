package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"

	"waygu-music/internal/pluginapi"
)

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
			return testPluginAdapter{}
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
		Queue: []pluginapi.Track{{ID: 39, Duration: 20000}}, Order: []int{0}, Position: 2500,
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

// Switching plugins clears old playback and only the new activation can save its queue.
func TestPlatformSwitchDiscardsSavedQueue(t *testing.T) {
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	defer mygo.App.SetPath(mygo.PathUserData, "")
	names := []string{"session.json", "session.json.part"}
	for _, name := range names {
		payload := []byte(`{"queue":[{"id":7,"name":"宝船","dt":200000}],"order":[0],"at":0,"position":48000}`)
		if err := os.WriteFile(filepath.Join(dir, name), payload, 0o600); err != nil {
			t.Fatal(err)
		}
	}
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
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s still present after switching platform: %v", name, err)
		}
	}
	a.pl.Cue([]pluginapi.Track{{ID: 9, Duration: 200000}})
	a.keepSession()
	if len(scheduled) != 2 {
		t.Fatalf("expected each activation to schedule a save, got %d callbacks", len(scheduled))
	}
	path := filepath.Join(dir, "session.json")
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

package player

import (
	"context"
	"errors"
	"testing"
	"time"

	"waygu-music/internal/pluginapi"
)

// Reset drops the current song and queue so a platform switch cannot retain playback.
func TestResetDropsPlaybackContext(t *testing.T) {
	p := &Player{at: -1, shuffle: true, repeat: RepeatOne}
	p.Cue([]pluginapi.Track{{ID: 1, Name: "一"}, {ID: 2, Name: "二"}})
	p.radio = "radio"
	p.feed = func(func([]pluginapi.Track)) {}
	p.playing = true
	p.Reset()
	if p.Current() != nil || len(p.Queue()) != 0 || p.Radio() != "" || p.Playing() {
		t.Fatalf("reset retained playback: current=%v queue=%d radio=%q playing=%v", p.Current(), len(p.Queue()), p.Radio(), p.Playing())
	}
	if p.Shuffle() || p.Repeat() != RepeatOff {
		t.Fatalf("reset retained playback modes: shuffle=%v repeat=%v", p.Shuffle(), p.Repeat())
	}
}

// Reset cancels stream, lyrics, and cover requests already started for the queue.
func TestResetCancelsMediaRequests(t *testing.T) {
	p := New(func(func()) {})
	defer p.Reset()
	stream, cover, lyrics := make(chan context.Context, 1), make(chan context.Context, 1), make(chan context.Context, 1)
	p.ResolveStream = func(ctx context.Context, _ *pluginapi.Track, _ string) (*pluginapi.Stream, error) {
		stream <- ctx
		<-ctx.Done()
		return nil, ctx.Err()
	}
	p.Artwork = func(ctx context.Context, _ *pluginapi.Track) ([]byte, error) {
		cover <- ctx
		return nil, nil
	}
	p.ResolveLyrics = func(ctx context.Context, _ *pluginapi.Track) (string, string, string, error) {
		lyrics <- ctx
		return "", "", "", nil
	}
	p.PlayQueue([]pluginapi.Track{{ID: 1}}, 0)
	requests := []context.Context{<-stream, <-cover, <-lyrics}
	p.Reset()
	for _, ctx := range requests {
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatal("a media request survived the reset")
		}
	}
}

// Appending a queue preserves the current songs and ignores duplicate IDs.
func TestAppendQueue(t *testing.T) {
	p := &Player{at: 0, queue: []pluginapi.Track{{ID: 1}}, order: []int{0}, track: &pluginapi.Track{ID: 1}}
	p.AppendQueue([]pluginapi.Track{{ID: 1}, {ID: 2}})
	if got := p.Queue(); len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Fatalf("queue = %+v", got)
	}
	if len(p.order) != 2 || p.order[1] != 1 {
		t.Fatalf("order = %+v", p.order)
	}
}

// Songs that cannot play stop the player after a few in a row, instead of
// flying through the whole queue: a NAS gone away failed every song, which
// skipped one after another.
func TestFailedLoadsStopAfterAFew(t *testing.T) {
	ch := make(chan func(), 64)
	p := New(func(fn func()) { ch <- fn })
	tried := 0
	p.ResolveStream = func(context.Context, *pluginapi.Track, string) (*pluginapi.Stream, error) {
		tried++
		return nil, errors.New("gone")
	}
	queue := make([]pluginapi.Track, 10)
	for i := range queue {
		queue[i] = pluginapi.Track{ID: int64(i + 1), Name: "song", Duration: 1000}
	}
	p.PlayQueue(queue, 0)
	for p.Loading() {
		select {
		case fn := <-ch:
			fn()
		case <-time.After(2 * time.Second):
			t.Fatal("still loading")
		}
	}
	if tried != maxSkipped || p.Playing() || p.Err() == "" {
		t.Fatalf("tried %d songs, playing=%v err=%q", tried, p.Playing(), p.Err())
	}
}

package app

import (
	"context"
	"encoding/gob"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/waygu-music-plugin/sdk/bridge"
	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/protocol"
)

// Rapid seeks and a repeat change reach the reader in order with one full queue.
func TestPluginPlaybackEventsInOrder(t *testing.T) {
	a := newTestApp()
	a.pl.Cue([]pluginapi.Track{{ID: 17}})
	scope := protocol.Scope{Connection: "test", Activation: 1}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stream := &pluginEventStream{
		ctx: ctx, cancel: cancel, done: make(chan struct{}),
		queue: make(chan queuedPlaybackEvent, playbackEventBuffer),
	}
	process := &pluginProcess{events: stream}
	a.pl.Changed = func() { process.publishPlayback(a, false) }
	process.publishPlayback(a, false)
	a.pl.Seek(2 * time.Second)
	a.pl.Seek(7 * time.Second)
	a.pl.CycleRepeat()

	host, child := net.Pipe()
	defer child.Close()
	go stream.serve(scope, func() (net.Conn, error) { return host, nil }, func(bool) {})
	want := []struct {
		position time.Duration
		repeat   pluginapi.RepeatMode
	}{
		{0, pluginapi.RepeatOff}, {2 * time.Second, pluginapi.RepeatOff},
		{7 * time.Second, pluginapi.RepeatOff}, {7 * time.Second, pluginapi.RepeatAll},
	}
	var got []PluginSnapshot
	_ = bridge.ReadPlaybackEvents(child, scope, func(snapshot PluginSnapshot) bool {
		got = append(got, snapshot)
		return len(got) < len(want)
	})
	if len(got) != len(want) {
		t.Fatalf("applied %d events, want %d", len(got), len(want))
	}
	for i, event := range got {
		if event.Position != want[i].position || event.Repeat != want[i].repeat {
			t.Fatalf("event %d reports position %v and repeat %v, want %+v", i, event.Position, event.Repeat, want[i])
		}
		if i == 0 {
			if event.UnchangedQueue || len(event.Queue) != 1 || event.Queue[0].ID != 17 {
				t.Fatalf("first event did not include the complete queue: %+v", event.Queue)
			}
		} else if !event.UnchangedQueue || len(event.Queue) != 0 {
			t.Fatalf("event %d did not reuse the established queue", i)
		}
	}
	cancel()
	select {
	case <-stream.done:
	case <-time.After(bridge.PlaybackStreamTimeout):
		t.Fatal("canceling the activation did not stop the event writer")
	}
}

// A missing event or a delta at the start of a stream cannot replace the child's queue.
func TestPluginPlaybackEventResync(t *testing.T) {
	scope := protocol.Scope{Connection: "test", Activation: 2}
	session := protocol.PluginSnapshot{Queue: []protocol.Track{{ID: 9}}, QueueRevision: 3}
	partial := protocol.PluginSnapshot{UnchangedQueue: true, QueueRevision: 3}
	consume := func(events ...protocol.PlaybackEvent) ([]PluginSnapshot, error) {
		host, child := net.Pipe()
		defer child.Close()
		go func() {
			defer host.Close()
			encoder := gob.NewEncoder(host)
			for _, event := range events {
				if encoder.Encode(event) != nil {
					return
				}
			}
		}()
		var got []PluginSnapshot
		err := bridge.ReadPlaybackEvents(child, scope, func(snapshot PluginSnapshot) bool {
			got = append(got, snapshot)
			return true
		})
		return got, err
	}
	initial := protocol.PlaybackEvent{Scope: scope, Sequence: 4, Snapshot: session}
	gap := protocol.PlaybackEvent{Scope: scope, Sequence: 6, Snapshot: partial}
	if got, err := consume(initial, gap); err == nil || len(got) != 1 {
		t.Fatalf("stream with a missing event applied %d snapshots and returned %v", len(got), err)
	}
	if got, err := consume(gap); err == nil || len(got) != 0 {
		t.Fatalf("stream starting without a complete queue applied %d snapshots and returned %v", len(got), err)
	}
	foreign := initial
	foreign.Scope.Activation++
	if got, err := consume(foreign); err == nil || len(got) != 0 {
		t.Fatalf("a stream from another activation applied %d snapshots and returned %v", len(got), err)
	}
	current := protocol.PlaybackEvent{Scope: scope, Sequence: 9, Snapshot: protocol.PluginSnapshot{
		Queue: []protocol.Track{{ID: 31}}, QueueRevision: 8,
	}}
	got, _ := consume(current)
	if len(got) != 1 || len(got[0].Queue) != 1 || got[0].Queue[0].ID != 31 {
		t.Fatalf("replacement stream did not bootstrap the current queue: %+v", got)
	}
}

// A stalled event consumer cannot block playback commands and must reopen with the current state.
func TestPluginPlaybackEventBacklog(t *testing.T) {
	a := newTestApp()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stream := &pluginEventStream{
		ctx: ctx, cancel: cancel, queue: make(chan queuedPlaybackEvent, playbackEventBuffer),
	}
	p := &pluginProcess{events: stream}
	a.pl.Changed = func() { p.publishPlayback(a, false) }
	p.publishPlayback(a, false)
	for i := 1; i <= playbackEventBuffer; i++ {
		a.pl.Seek(time.Duration(i) * time.Second)
	}
	if err := ctx.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("full event queue left the stalled subscription active: %v", err)
	}
	ctx, cancelNew := context.WithCancel(t.Context())
	defer cancelNew()
	next := &pluginEventStream{ctx: ctx, cancel: cancelNew, queue: make(chan queuedPlaybackEvent, 1)}
	p.events = next
	p.publishPlayback(a, false)
	select {
	case event := <-next.queue:
		if event.snapshot.UnchangedQueue || event.snapshot.Position != playbackEventBuffer*time.Second {
			t.Fatalf("new subscription did not start with the latest full state: %+v", event.snapshot)
		}
	default:
		t.Fatal("new subscription did not publish its starting state")
	}
}

// A jump based on the displayed order is rejected when the host has since shuffled the queue.
func TestPluginJumpRejectsOldQueueOrder(t *testing.T) {
	a := newTestApp()
	a.pl.Cue([]pluginapi.Track{{ID: 17}, {ID: 18}})
	scope := a.pluginActivations.Prepare("playback")
	if !a.pluginActivations.Commit(scope) {
		t.Fatal("could not activate the test connection")
	}
	revision, _ := a.pl.Revisions()
	reverse := pluginReverse{app: a, connection: scope.Connection}
	a.pl.ToggleShuffle()
	err := reverse.apply(PluginHostCall{
		Action: "jump", Scope: scope, HasRevision: true, QueueRevision: revision, Index: 1,
	}, &PluginHostReply{})
	var failure protocol.Failure
	if !errors.As(err, &failure) || failure.Code != "stale-playback-state" || a.pl.Current().ID != 17 {
		t.Fatalf("jump based on the previous order was not rejected: %v", err)
	}
}

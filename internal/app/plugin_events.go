package app

import (
	"context"
	"encoding/gob"
	"net"
	"reflect"
	"time"

	"github.com/egoist/mygo"
	"github.com/waygu-music-plugin/sdk/bridge"
	"github.com/waygu-music-plugin/sdk/protocol"
)

const (
	playbackHeartbeat = 2 * time.Second

	playbackEventBuffer = 16
)

type queuedPlaybackEvent struct {
	sequence uint64
	snapshot PluginSnapshot
}

type pluginEventStream struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	queue  chan queuedPlaybackEvent

	// Only the host application thread updates the published state.
	last      PluginSnapshot
	published bool
}

func (r pluginReverse) Watch(in protocol.WatchRequest, out *protocol.WatchReply) error {
	out.Failure = protocol.Failure{Code: "host-closing"}
	mygo.RunOnMain(func() {
		out.Stream, out.Failure = r.openPlaybackEvents(in)
	})
	return nil
}

func (r pluginReverse) openPlaybackEvents(in protocol.WatchRequest) (uint32, protocol.Failure) {
	a := r.app
	if in.DeadlineNano != 0 && time.Now().UnixNano() >= in.DeadlineNano {
		return 0, protocol.Failure{Code: "deadline-exceeded"}
	}
	// Opening a stream has a side effect and requires an active scope.
	if err := a.pluginActivations.Check(r.connection, in.Scope, true); err != nil {
		return 0, bridge.Failure("host", err)
	}
	active := a.remoteActive()
	if active == nil || active.scope != in.Scope {
		return 0, protocol.Failure{Code: "events-unavailable"}
	}
	if previous := active.events; previous != nil {
		select {
		case <-previous.done:
		default:
			return 0, protocol.Failure{Code: "events-busy"}
		}
	}
	ctx, cancel := context.WithCancel(a.platformContext)
	stream := &pluginEventStream{
		ctx: ctx, cancel: cancel, done: make(chan struct{}),
		queue: make(chan queuedPlaybackEvent, playbackEventBuffer),
	}
	active.events = stream
	active.publishPlayback(a, false)
	id, accept := active.transport.ReserveStream()
	go stream.serve(active.scope, accept, func(heartbeat bool) {
		mygo.RunOnMain(func() {
			if a.remoteActive() == active && active.events == stream {
				active.publishPlayback(a, heartbeat)
			}
		})
	})
	return id, protocol.Failure{}
}

func (p *pluginProcess) publishPlayback(a *app, heartbeat bool) {
	s := p.events
	if s == nil || s.ctx.Err() != nil {
		return
	}
	var known *uint64
	if s.published {
		known = &s.last.QueueRevision
	}
	snapshot := capturePluginSnapshot(a, known)
	state := snapshot
	state.Queue, state.UnchangedQueue = nil, false
	if s.published && !heartbeat && reflect.DeepEqual(s.last, state) {
		return
	}
	event := queuedPlaybackEvent{sequence: p.eventSequence + 1, snapshot: snapshot}
	select {
	case s.queue <- event:
		p.eventSequence = event.sequence
		s.last, s.published = state, true
	default:
		// A slow reader must resubscribe for a current full snapshot.
		s.cancel()
	}
}

func (s *pluginEventStream) serve(scope protocol.Scope, accept func() (net.Conn, error), sample func(bool)) {
	defer close(s.done)
	defer s.cancel()
	conn, err := accept()
	if err != nil {
		return
	}
	defer conn.Close()
	bridge.CancelOnStreamClose(s.ctx, conn, s.cancel)
	encoder := gob.NewEncoder(conn)
	ticker := time.NewTicker(bridge.PlaybackSampleInterval)
	defer ticker.Stop()
	lastSent := time.Now()
	for s.ctx.Err() == nil {
		select {
		case <-s.ctx.Done():
			return
		case now := <-ticker.C:
			sample(now.Sub(lastSent) >= playbackHeartbeat)
		case event := <-s.queue:
			if err := conn.SetWriteDeadline(time.Now().Add(bridge.PlaybackStreamTimeout)); err != nil {
				return
			}
			if err := encoder.Encode(protocol.PlaybackEvent{
				Scope: scope, Sequence: event.sequence, Snapshot: bridge.WireSnapshot(event.snapshot),
			}); err != nil {
				return
			}
			lastSent = time.Now()
		}
	}
}

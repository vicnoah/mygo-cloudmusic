package app

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"time"

	"github.com/egoist/mygo"
	"github.com/waygu-music-plugin/sdk/bridge"
	"github.com/waygu-music-plugin/sdk/platformrpc"
	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/protocol"
)

func pluginRPCTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func capturePluginSnapshot(a *app, known *uint64) PluginSnapshot {
	h := platformHost{a: a}
	pl := a.pl
	s := PluginSnapshot{
		Current: pl.Current(), Playing: pl.Playing(), Loading: pl.Loading(),
		Ready: pl.Ready(), Shuffle: pl.Shuffle(), Repeat: pl.Repeat(), Radio: pl.Radio(), Error: pl.Err(),
		Position: pl.Position(), Duration: pl.Duration(), Volume: h.Volume(), Presentation: h.Presentation(),
		Desktop: h.DesktopSettings(), Decoder: h.DecoderStatus(), Casting: h.Casting(),
		EQ: h.EqualizerSettings(), EQAvailable: h.EqualizerAvailable(), Search: h.GlobalSearchHistory(),
	}
	s.Order, s.At = pl.PlayOrder()
	s.QueueRevision, s.Attempt = pl.Revisions()
	s.Order = slices.Clone(s.Order)
	if s.Current != nil {
		current := clonePluginTrack(*s.Current)
		s.Current = &current
	}
	s.UnchangedQueue = known != nil && *known == s.QueueRevision
	if !s.UnchangedQueue {
		s.Queue = slices.Clone(pl.Queue())
		for i := range s.Queue {
			s.Queue[i] = clonePluginTrack(s.Queue[i])
		}
	}
	s.Downloaded, s.DownloadKnown = pl.Downloaded()
	s.Lyrics, s.LyricsLoaded = pl.Lyrics()
	s.Lyrics = slices.Clone(s.Lyrics)
	for i := range s.Lyrics {
		s.Lyrics[i].Words = slices.Clone(s.Lyrics[i].Words)
	}
	s.Levels, s.LevelsLive = pl.Levels()
	if st := pl.Stream(); st != nil {
		// The rendering process only needs the shape of the current audio.
		s.Stream = &PluginSource{Type: st.Type, Level: st.Level, Size: st.Size, Gain: st.Gain, Peak: st.Peak}
	}
	return s
}

func clonePluginTrack(t pluginapi.Track) pluginapi.Track {
	t.Artists = slices.Clone(t.Artists)
	t.Alias = slices.Clone(t.Alias)
	if t.Privilege != nil {
		priv := *t.Privilege
		t.Privilege = &priv
	}
	return t
}

type pluginReverse struct {
	app        *app
	connection string
}

func (r pluginReverse) Use(in protocol.PluginHostCall, reply *protocol.PluginHostReply) error {
	local := PluginHostReply{Failure: platformrpc.Failure{Code: "host-closing"}}
	mygo.RunOnMain(func() {
		local.Failure = bridge.Failure("host", r.apply(bridge.LocalHostCall(in), &local))
	})
	*reply = local.Wire()
	return nil
}

func (r pluginReverse) apply(in PluginHostCall, reply *PluginHostReply) error {
	a := r.app
	if in.DeadlineNano != 0 && time.Now().UnixNano() >= in.DeadlineNano {
		return platformrpc.Failure{Code: "deadline-exceeded"}
	}
	read := in.Action == "snapshot"
	if err := a.pluginActivations.Check(r.connection, in.Scope, !read); err != nil {
		return err
	}
	if read {
		var known *uint64
		if in.HasRevision {
			known = &in.QueueRevision
		}
		reply.Snapshot = capturePluginSnapshot(a, known)
		return nil
	}
	h := platformHost{a: a}
	p := a.pl
	queue, attempt := p.Revisions()
	if in.HasRevision && ((in.Action == "jump" && in.QueueRevision != queue) || (in.Action == "seek" && in.Attempt != attempt)) {
		return platformrpc.Failure{Code: "stale-playback-state"}
	}
	switch in.Action {
	case "play":
		if !a.processOwnsTracks(in.Tracks) || in.Index < 0 || in.Index >= len(in.Tracks) {
			return platformrpc.Failure{Code: "invalid-queue"}
		}
		if in.Feed != "" {
			p.PlayList(in.Tracks, in.Index, r.feed(in))
		} else {
			p.PlayQueue(in.Tracks, in.Index)
		}
	case "radio-play":
		if !a.processOwnsTracks(in.Tracks) || len(in.Tracks) == 0 {
			return platformrpc.Failure{Code: "invalid-queue"}
		}
		p.PlayRadio(in.Text, in.Tracks, r.feed(in))
	case "radio-from":
		if !a.processOwnsTracks(in.Tracks) {
			return platformrpc.Failure{Code: "invalid-queue"}
		}
		p.RadioFrom(in.Text, in.Tracks, r.feed(in))
	case "session-load":
		if session, ok := (playbackContinuity{a: a, ctx: a.platformContext}).LoadSession(); ok {
			reply.Session = &session
		}
	case "session-restore":
		if in.Session == nil || !a.processOwnsTracks(in.Session.Queue) {
			return platformrpc.Failure{Code: "invalid-queue"}
		}
		p.Restore(*in.Session)
	case "radio-attach":
		p.AttachRadio(in.Text, r.feed(in))
	case "audio-open":
		remote := a.remoteActive()
		if remote == nil || remote.scope != in.Scope {
			return platformrpc.Failure{Code: "stale-activation"}
		}
		select {
		case remote.pcm <- struct{}{}:
		default:
			return platformrpc.Failure{Code: "too-many-audio-readers"}
		}
		audio, err := p.OpenAudioFor(a.platformContext)
		if err != nil {
			<-remote.pcm
			return err
		}
		id, accept := remote.transport.ReserveStream()
		reply.Audio = &protocol.PluginAudioDescription{ID: id, Rate: int32(audio.SampleRate()), Channels: int32(audio.Channels())}
		go func() {
			defer func() { <-remote.pcm }()
			bridge.ServeAudio(accept, audio)
		}()
	case "append":
		if !a.processOwnsTracks(in.Tracks) {
			return platformrpc.Failure{Code: "invalid-queue"}
		}
		p.AppendQueue(in.Tracks)
	case "resume-pause":
		p.TogglePlay()
	case "previous":
		p.Prev()
	case "next":
		p.Next()
	case "seek":
		p.Seek(in.Position)
	case "jump":
		p.Jump(in.Index)
	case "shuffle":
		p.ToggleShuffle()
	case "set-shuffle":
		if p.Shuffle() != in.Flag {
			p.ToggleShuffle()
		}
	case "repeat":
		p.CycleRepeat()
	case "end-radio":
		p.EndRadio()
	case "volume":
		h.SetVolume(in.Number)
	case protocol.HostPatchDesktopPreferences:
		if !in.DesktopPatch.Valid() {
			return protocol.Failure{Code: "invalid-settings"}
		}
		if in.DesktopPatch.Fields == 0 {
			return nil
		}
		prefs := bridge.WireDesktopPreferences(h.DesktopSettings().Preferences)
		return a.applyDesktopPreferences(bridge.LocalDesktopPreferences(in.DesktopPatch.ApplyTo(prefs)))
	case "presentation":
		h.SetLyricsPresentation(in.View.LyricsStage, in.View.StageStyle, in.View.StageSparks)
	case protocol.HostSaveDesktopLyrics:
		if in.DesktopLyrics == nil {
			return protocol.Failure{Code: "invalid-settings"}
		}
		prefs := bridge.LocalDesktopLyricsPrefs(in.DesktopLyrics)
		if prefs.Color < 0 || !(prefs.FontSize > 0) || math.IsInf(float64(prefs.FontSize), 0) {
			return protocol.Failure{Code: "invalid-settings"}
		}
		return a.saveDesktopLyricsPrefs(prefs)
	case protocol.TaskCacheClear:
		return reply.AcceptTask(a.clearCache())
	case protocol.TaskDecoderDownload:
		return reply.AcceptTask(a.startMPVDownload())
	case "decoder-cancel":
		return reply.AcceptTask(a.cancelMPVDownload(in.TaskID, protocol.TaskCanceled))
	case protocol.TaskDecoderDelete:
		return reply.AcceptTask(a.deleteMPV(), nil)
	case protocol.TaskCastingScan:
		return reply.AcceptTask(a.scanDLNADevices())
	case protocol.TaskCastingOutput:
		return reply.AcceptTask(a.switchPlaybackOutput(in.Text))
	case protocol.HostDesktopTask:
		task, ok := a.desktopTasks.find(in.TaskID)
		if !ok {
			return protocol.Failure{Code: "unknown-task"}
		}
		return reply.AcceptTask(task, nil)
	case "equalizer":
		h.ApplyEqualizer(in.EQ)
	case "history-add":
		h.RememberGlobalSearch(in.Text)
	case "history-clear":
		h.ClearSearchHistory()
	case "update-check":
		h.CheckForUpdates()
	case "update-auto":
		h.SetUpdateChecks(in.Flag)
	case "update-download":
		h.SetUpdateDownloads(in.Flag)
	case "manage":
		a.openPluginManager()
	case "preferences-save":
		if !json.Valid(in.Settings) {
			return platformrpc.Failure{Code: "invalid-settings"}
		}
		return a.saveProcessSettings(in.Settings)
	default:
		return platformrpc.Failure{Code: "unknown-action"}
	}
	return nil
}

func (r pluginReverse) feed(in PluginHostCall) pluginapi.QueueFeed {
	if in.Feed == "" {
		return nil
	}
	a := r.app
	current := func() bool { return a.pluginActivations.Check(r.connection, in.Scope, true) == nil }
	return scopedQueueFeed(a.platformContext, current, a.processOwnsTracks, func(ctx context.Context, cursor pluginapi.QueueFeedCursor, got func([]pluginapi.Track, error)) {
		active := a.remoteActive()
		if active == nil || !current() {
			return
		}
		go func() {
			reply, err := active.call(ctx, PluginCall{Action: "feed", Ref: in.Feed, FeedCursor: &cursor})
			mygo.RunOnMain(func() {
				if ctx.Err() != nil || !current() {
					return
				}
				if err != nil {
					got(nil, pluginapi.ErrQueueFeedUnavailable)
					return
				}
				got(reply.Tracks, nil)
			})
		}()
	})
}

func (a *app) processOwnsTracks(tracks []pluginapi.Track) bool {
	return len(tracks) <= 8192 && a.ownsTracksFor(a.platformID, tracks)
}

func (a *app) saveProcessSettings(data json.RawMessage) error {
	for _, entry := range a.platformRegistrations {
		if entry.ID != a.platformID {
			continue
		}
		var flat map[string]json.RawMessage
		if len(entry.LegacySettingsFields) != 0 {
			if err := json.Unmarshal(data, &flat); err != nil {
				return err
			}
		}
		return (pluginSettingsStore{a: a, entry: entry}).commit(data, flat)
	}
	return platformrpc.Failure{Code: "unknown-entry"}
}

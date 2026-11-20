package player

import (
	"context"
	"errors"

	"github.com/waygu-music-plugin/sdk/pluginapi"
)

// maxSkipped is how many songs that cannot play in a row stop the player,
// with the last one's error shown.
const maxSkipped = 3

// Platform extension seam.
//
// Keep provider-neutral player additions here: embedded extension state,
// platform queue helpers, reset behavior and optional resolver hooks. The
// upstream player.go should gain only the smallest possible embedded field or
// stable hook call. Concrete provider protocol types must stay in the
// corresponding platform adapter and must not be imported here.

// PlayList plays the loaded part of a provider list and asks more for the
// remainder as playback approaches its end. It is the shared queue seam for
// platform pagers; the player never knows how a page is fetched.
func (p *Player) PlayList(tracks []pluginapi.Track, start int, more pluginapi.QueueFeed) {
	if len(tracks) == 0 {
		return
	}
	p.PlayQueue(tracks, start)
	p.feed = more
	p.refill()
}

// AppendQueue adds songs to the current queue without replacing what is
// playing. Duplicate tracks from the same instance are ignored.
func (p *Player) AppendQueue(tracks []pluginapi.Track) {
	if p == nil || len(tracks) == 0 {
		return
	}
	p.EndRadio()
	p.appendTracks(tracks)
	p.sync()
}

// Reset stops playback and drops the complete playback context. It is used
// when the application changes music platforms: a queue, radio feed, or
// in-flight load belongs to the platform that created it and must not leak
// into the next one.
func (p *Player) Reset() {
	if p == nil {
		return
	}
	p.stop(true)
	p.gen++
	p.queueRevision++
	p.endRadio()
	p.queue = nil
	p.order = nil
	p.at = -1
	p.track = nil
	p.info = nil
	p.lyrics = nil
	p.lyricsLoaded = false
	p.pending = 0
	p.seeking = false
	p.loading = false
	p.playing = false
	p.err = ""
	p.shuffle = false
	p.repeat = RepeatOff
	if p.IsCasting() {
		p.dlnaCtrl.Stop()
	}
	p.sync()
}

// PlatformPlayer holds the host callbacks used to look up media by track.
type PlatformPlayer struct {
	started       bool // the current track has been reported as started
	loadContext   context.Context
	cancelLoad    context.CancelFunc
	queueRevision uint64

	ResolveStream     func(context.Context, *pluginapi.Track, string) (*pluginapi.Stream, error)
	ArtworkURL        func(*pluginapi.Track) string
	Artwork           func(context.Context, *pluginapi.Track) ([]byte, error)
	ResolveLyrics     func(context.Context, *pluginapi.Track) (string, string, string, error)
	ResolveCastSource func(context.Context, *pluginapi.Track, string) (*pluginapi.Stream, pluginapi.SegmentedSource, error)
	ResolveSegments   func(context.Context, *pluginapi.Track, bool) (*pluginapi.Stream, pluginapi.SegmentedSource, error)
	TrackPreferences  func(*pluginapi.Track) pluginapi.AudioPreferences
	StreamError       func(*pluginapi.Track, error) string
	Started           func(*pluginapi.Track)
}

// Revisions identifies the queue and the playback attempt on the app thread.
func (p *Player) Revisions() (queue, attempt uint64) {
	return p.queueRevision, uint64(p.gen)
}

func (p *Player) trackContext() context.Context {
	if p.loadContext != nil {
		return p.loadContext
	}
	return context.Background()
}

// acceptRefill applies a provider queue refill without making player.go know
// how an exhausted feed should end. Returning false tells the caller that no
// tracks were appended and playback should stop waiting for more.
func (p *Player) acceptRefill(more []pluginapi.Track) bool {
	if len(more) == 0 && p.radio == "" {
		p.feed = nil
		if p.awaiting {
			p.awaiting = false
			p.playing = false
			p.sync()
		}
		return false
	}
	p.appendTracks(more)
	return true
}

func (p *Player) coverURL(t *pluginapi.Track) string {
	if p.ArtworkURL != nil {
		return p.ArtworkURL(t)
	}
	return t.Album.PicURL
}

func (p *Player) resolveArtwork(ctx context.Context, t *pluginapi.Track) ([]byte, error) {
	if p.Artwork != nil {
		return p.Artwork(ctx, t)
	}
	return nil, pluginapi.ErrUnavailable
}

func (p *Player) resolveLyrics(ctx context.Context, t *pluginapi.Track) (string, string, string, error) {
	if p.ResolveLyrics != nil {
		return p.ResolveLyrics(ctx, t)
	}
	return "", "", "", pluginapi.ErrUnavailable
}

// resolveAudio reports whether a source was resolved or failed to open.
func (p *Player) resolveAudio(ctx context.Context, t *pluginapi.Track, advanced bool) (*pluginapi.Stream, stream, error, bool) {
	if p.ResolveSegments == nil {
		return nil, nil, nil, false
	}
	info, source, err := p.ResolveSegments(ctx, t, advanced)
	if err != nil {
		return nil, nil, err, true
	}
	if source == nil {
		return nil, nil, nil, false
	}
	decoded, err := openSegmentedStream(ctx, source)
	if err != nil {
		source.Close()
	}
	return info, decoded, err, true
}

func (p *Player) resolveStream(ctx context.Context, t *pluginapi.Track, level string) (*pluginapi.Stream, error) {
	if p.ResolveStream != nil {
		return p.ResolveStream(ctx, t, level)
	}
	return nil, errors.New("no playback media resolver configured")
}

// markStarted reports the current track once, as it first begins to play.
func (p *Player) markStarted() {
	if p.started || p.track == nil || p.Started == nil {
		return
	}
	p.started = true
	p.Started(p.track)
}

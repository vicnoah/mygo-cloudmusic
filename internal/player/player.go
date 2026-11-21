package player

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"

	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/pluginapi/lyrics"
	"waygu-music/internal/player/dlna"
	"waygu-music/internal/player/mpv"
)

type mpvVolumeOutput struct {
	p *Player
	e *mpv.Engine
}

func (m mpvVolumeOutput) setVolume(v float64) {
	if m.e != nil {
		m.e.SetVolume(max(0, min(1, v)))
	}
}

func (m mpvVolumeOutput) setFadeLevel(level float64) {
	if m.e != nil && m.p != nil {
		m.e.SetVolume(max(0, min(1, m.p.volume*level)))
	}
}

type RepeatMode = pluginapi.RepeatMode

const (
	RepeatOff = pluginapi.RepeatOff
	RepeatAll = pluginapi.RepeatAll
	RepeatOne = pluginapi.RepeatOne
)

// Player plays a queue of tracks. Its methods run on the main thread;
// loading happens on goroutines, whose results post brings back there.
type Player struct {
	PlatformPlayer
	post func(func())
	// Quality returns the stream level to load, as the settings say.
	Quality func() string
	// Normalize reports whether to even out the loudness of tracks, as
	// the settings say.
	Normalize func() bool
	// Cache keeps the songs played, if set.
	Cache *Cache
	// TrackPlatform resolves a track to its registered instance.
	TrackPlatform func(*pluginapi.Track) pluginapi.ID
	// Changed, if set, hears that what plays, where it is or whether it
	// plays changed.
	Changed func()

	queue   []pluginapi.Track
	order   []int // the queue's indices in play order: shuffled or not
	at      int   // where in order the current track is, -1 for none
	shuffle bool
	repeat  RepeatMode
	volume  float64

	// A radio goes on with the tracks its feed gives, as the queue nears
	// its end, for as long as nothing else is played.
	radio      string
	feed       pluginapi.QueueFeed
	feedPage   uint64
	feedCancel context.CancelFunc
	feedRetry  bool // the feed failed while playback was waiting at the end
	awaiting   bool // at the queue's end, for what the feed gives

	track   *pluginapi.Track
	cover   []byte
	out     *audio.Player
	stream  stream
	info    *pluginapi.Stream // what the current track streams
	fade    fader
	gain    float64 // the current track's normalizing gain
	loading bool
	playing bool // what the listener wants: false while paused
	err     string
	gen     int // counts loads, to drop the results of stale ones

	lyrics       []lyrics.Line
	lyricsLoaded bool

	useMPV      bool
	mpvCfg      mpv.Config
	mpvOut      *mpv.Engine
	dlnaCtrl    *dlna.Controller
	cast        *castPlayback
	castWorkers sync.WaitGroup

	// rgDB is the replaygain preamp last set on mpvOut, and rgSet whether
	// one was: changing the volume need not set it again.
	rgDB  float64
	rgSet bool

	// pending holds a seek asked for while the track loads.
	pending time.Duration
	// A seek playing fades out, then lands, then fades in: meanwhile the
	// position is where it goes.
	seeking bool
	seekTo  time.Duration
	seekGen int
}

func New(post func(func())) *Player {
	p := &Player{post: post, at: -1, volume: 0.8, gain: 1}
	if ctrl, err := dlna.NewController(func() { p.post(p.castChanged) }); err == nil {
		p.dlnaCtrl = ctrl
	}
	// Music, mixed far enough ahead to play on while macOS keeps the app
	// waiting, as it does while its appearance changes.
	audio.SetLatency(audio.LatencyPlayback)
	// The media keys, and the controls of Control Center.
	mygo.NowPlaying.OnCommand(func(cmd mygo.MediaCommand, pos time.Duration) {
		switch cmd {
		case mygo.MediaPlay, mygo.MediaPause:
			if (cmd == mygo.MediaPlay) != p.castWantsPlay() {
				p.TogglePlay()
			}
		case mygo.MediaNext:
			p.Next()
		case mygo.MediaPrevious:
			p.Prev()
		case mygo.MediaSeek:
			p.Seek(pos)
		}
		p.post(func() {})
	})
	return p
}

// sync shows the system, and Changed, where playback is.
func (p *Player) sync() {
	defer p.notifyChanged()
	if p.track == nil {
		mygo.NowPlaying.Clear()
		return
	}
	state := mygo.PlaybackPaused
	if p.Playing() {
		state = mygo.PlaybackPlaying
	}
	mygo.NowPlaying.SetPlayback(state, p.Position())
}

func (p *Player) notifyChanged() {
	if p.Changed != nil {
		p.Changed()
	}
}

// announce shows the system the track, and its cover once loaded.
func (p *Player) announce(t *pluginapi.Track, artwork []byte) {
	p.cover = artwork
	state := mygo.PlaybackPaused
	if p.Playing() {
		state = mygo.PlaybackPlaying
	}
	mygo.NowPlaying.Set(mygo.NowPlayingInfo{
		Title:    t.Name,
		Artist:   t.ArtistNames(),
		Album:    t.Album.Name,
		Artwork:  artwork,
		Duration: t.Length(),
		Position: p.Position(),
		State:    state,
		Commands: mygo.MediaPlay | mygo.MediaPause | mygo.MediaNext | mygo.MediaPrevious | mygo.MediaSeek,
	})
}

func (p *Player) ended() {
	if p.repeat == RepeatOne {
		if p.IsCasting() {
			p.load(p.at)
			return
		}
		p.out.Seek(0)
		p.out.Play()
		return
	}
	if p.at+1 >= len(p.order) && p.feed != nil {
		p.awaitFeed()
		return
	}
	if p.at+1 >= len(p.order) && p.repeat == RepeatOff {
		p.playing = false
		p.sync()
		return
	}
	p.Next()
}

// PlayQueue replaces the queue with tracks and plays the one at start.
func (p *Player) PlayQueue(tracks []pluginapi.Track, start int) {
	if len(tracks) == 0 {
		return
	}
	p.endRadio()
	start = min(max(start, 0), len(tracks)-1)
	p.queue = append([]pluginapi.Track(nil), tracks...)
	p.queueRevision++
	p.order = make([]int, len(p.queue))
	for i := range p.order {
		p.order[i] = i
	}
	if p.shuffle {
		p.shuffleOrder(start)
		p.load(0)
		return
	}
	p.load(start)
}

// PlayRadio plays tracks in their order, then goes on with those feed
// gives as the queue nears its end, until something else is played. The
// radio's name says which it is. feed calls got on the main thread.
func (p *Player) PlayRadio(name string, tracks []pluginapi.Track, feed pluginapi.QueueFeed) {
	if len(tracks) == 0 {
		return
	}
	shuffle := p.shuffle
	p.shuffle = false
	p.PlayQueue(tracks, 0)
	p.shuffle = shuffle
	p.AttachRadio(name, feed)
}

// RadioFrom makes the track playing the first of a radio's queue, which
// goes on with tracks, then with those feed gives: without starting it
// over.
func (p *Player) RadioFrom(name string, tracks []pluginapi.Track, feed pluginapi.QueueFeed) {
	if p.track == nil {
		return
	}
	p.queue, p.order, p.at = []pluginapi.Track{*p.track}, []int{0}, 0
	p.queueRevision++
	p.appendTracks(tracks)
	p.AttachRadio(name, feed)
}

// AttachRadio makes the queue the radio name's, which feed goes on with,
// as after restoring a session that played it.
func (p *Player) AttachRadio(name string, feed pluginapi.QueueFeed) {
	p.endRadio()
	p.radio, p.feed = name, feed
	p.notifyChanged()
	p.refill()
}

// EndRadio stops the radio playing: the song playing plays on, but the
// songs the radio queued after it go, and it adds no more.
func (p *Player) EndRadio() {
	if p.radio == "" && p.feed == nil {
		return
	}
	awaiting := p.awaiting
	p.endRadio()
	if p.at >= 0 && p.at+1 < len(p.order) {
		kept := make([]pluginapi.Track, p.at+1)
		for i, k := range p.order[:p.at+1] {
			kept[i] = p.queue[k]
		}
		p.queue, p.order = kept, make([]int, len(kept))
		p.queueRevision++
		for i := range p.order {
			p.order[i] = i
		}
	}
	if awaiting {
		// At its end, waiting for more: nothing plays on.
		p.playing = false
		p.sync()
		return
	}
	p.notifyChanged()
}

func (p *Player) endRadio() {
	if p.feedCancel != nil {
		p.feedCancel()
		p.feedCancel = nil
	}
	p.radio, p.feed, p.feedPage, p.feedRetry, p.awaiting = "", nil, 0, false, false
}

// Radio returns the name of the radio playing, or "".
func (p *Player) Radio() string { return p.radio }

// refill asks the radio for more once the queue is near its end.
func (p *Player) refill() {
	if p.feed == nil || p.feedCancel != nil || p.at < len(p.order)-2 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.feedCancel = cancel
	cursor := pluginapi.QueueFeedCursor{Page: p.feedPage}
	if len(p.order) > 0 {
		cursor.FirstID = p.queue[p.order[0]].ID
		cursor.LastID = p.queue[p.order[len(p.order)-1]].ID
	}
	p.feed(ctx, cursor, func(more []pluginapi.Track, err error) {
		if ctx.Err() != nil {
			return
		}
		cancel()
		p.feedCancel = nil
		if err != nil {
			if p.awaiting {
				p.awaiting, p.playing, p.feedRetry = false, false, true
				p.err = "暂时无法加载更多歌曲"
				p.sync()
			}
			return
		}
		p.feedPage++
		if !p.acceptRefill(more) {
			return
		}
		switch {
		case !p.awaiting:
			p.notifyChanged()
		case p.at+1 < len(p.order):
			p.awaiting = false
			p.load(p.at + 1)
		default:
			// The radio has nothing more: it stops at its end.
			p.awaiting, p.playing = false, false
			p.err = "电台暂时没有更多歌曲"
			p.sync()
		}
	})
}

// awaitFeed waits at the end of a radio's queue for what its feed gives.
func (p *Player) awaitFeed() {
	if p.feedRetry {
		p.feedRetry = false
		p.err = ""
	}
	p.awaiting = true
	p.notifyChanged()
	p.refill()
}

// appendTracks adds tracks to the end of the queue, but those in it.
func (p *Player) appendTracks(tracks []pluginapi.Track) {
	before := len(p.queue)
	in := make(map[trackIdentity]bool, len(p.queue))
	for _, t := range p.queue {
		in[identifyTrack(p.trackOwner(&t), &t)] = true
	}
	for _, t := range tracks {
		ref := identifyTrack(p.trackOwner(&t), &t)
		if !in[ref] {
			in[ref] = true
			p.order = append(p.order, len(p.queue))
			p.queue = append(p.queue, t)
		}
	}
	if len(p.queue) != before {
		p.queueRevision++
	}
}

// shuffleOrder shuffles the order, with first first.
func (p *Player) shuffleOrder(first int) {
	rand.Shuffle(len(p.order), func(i, j int) { p.order[i], p.order[j] = p.order[j], p.order[i] })
	for i, v := range p.order {
		if v == first {
			p.order[0], p.order[i] = p.order[i], p.order[0]
		}
	}
}

func (p *Player) ToggleShuffle() {
	p.shuffle = !p.shuffle
	defer p.notifyChanged()
	if len(p.order) == 0 {
		return
	}
	p.queueRevision++
	cur := p.order[max(p.at, 0)]
	if p.shuffle {
		p.shuffleOrder(cur)
		p.at = 0
	} else {
		for i := range p.order {
			p.order[i] = i
		}
		p.at = cur
	}
}

func (p *Player) CycleRepeat() {
	p.repeat = (p.repeat + 1) % 3
	p.notifyChanged()
}

// Current returns the track playing or loading, or nil.
func (p *Player) Current() *pluginapi.Track { return p.track }

// CurrentArtwork returns the current track's cover data, which callers treat as read-only.
func (p *Player) CurrentArtwork() []byte { return p.cover }

// Queue returns the tracks queued, in the order they were given.
func (p *Player) Queue() []pluginapi.Track { return p.queue }

// Playing reports the renderer state during a cast and playback intent locally.
func (p *Player) Playing() bool {
	if p.IsCasting() {
		return p.dlnaCtrl.CastStatus().Playing
	}
	return p.playing
}

// Loading reports whether the current track is still loading.
func (p *Player) Loading() bool {
	return p.loading || p.awaiting || (p.IsCasting() && p.dlnaCtrl.CastStatus().Loading)
}

// Ready reports whether the current track has loaded, to play.
func (p *Player) Ready() bool {
	if p.IsCasting() {
		return p.cast != nil && p.dlnaCtrl.CastStatus().Ready
	}
	return p.out != nil || p.mpvOut != nil
}

// Err returns why the current track did not play, or "".
func (p *Player) Err() string {
	if p.IsCasting() && p.cast != nil {
		if status := p.dlnaCtrl.CastStatus(); status.ID == p.cast.id && status.Err != nil {
			return status.Err.Error()
		}
	}
	return p.err
}

// Stream returns what the current track streams, once loaded, or nil.
func (p *Player) Stream() *pluginapi.Stream { return p.info }

func (p *Player) Shuffle() bool      { return p.shuffle }
func (p *Player) Repeat() RepeatMode { return p.repeat }

// Lyrics returns the current track's lyrics, and whether they have loaded.
func (p *Player) Lyrics() ([]lyrics.Line, bool) { return p.lyrics, p.lyricsLoaded }

// Downloaded returns the share of the current track downloaded so far, or
// false before it has started to.
func (p *Player) Downloaded() (float64, bool) {
	if p.stream == nil {
		return 0, false
	}
	dl, _ := p.stream.source()
	return dl.Progress(), true
}

// Audio is the current track's sound decoded apart from what plays, as
// OpenAudio opens it: interleaved stereo at its rate, read as it
// downloads.
type Audio = pluginapi.Audio

// OpenAudio opens the current track's sound again, for decoding apart
// from what plays, as for transcribing an episode. It reads the same
// download, waiting for what has not come in, and fails once the track
// changes. It works on any goroutine once opened; close it when done.
func (p *Player) OpenAudio() (Audio, error) {
	return p.OpenAudioFor(context.Background())
}

// OpenAudioFor also revokes the reader when its owning plugin is deactivated.
func (p *Player) OpenAudioFor(activation context.Context) (Audio, error) {
	if err := activation.Err(); err != nil {
		return nil, err
	}
	if p.stream == nil || p.track == nil {
		return nil, errors.New("player: nothing loaded")
	}
	if _, segmented := p.stream.(*hlsStream); segmented {
		return nil, errors.New("player: independent decoding is unavailable for segmented audio")
	}
	dl, _ := p.stream.source()
	audio, err := openShared(p.trackContext(), activation, dl, p.track.Duration)
	if err != nil {
		return nil, err
	}
	return audio, nil
}

// PlayOrder returns the queue's indices in play order, and where the
// current track is among them, -1 for none.
func (p *Player) PlayOrder() ([]int, int) { return p.order, p.at }

func (p *Player) Next() {
	if len(p.order) == 0 {
		return
	}
	at := p.at + 1
	if at >= len(p.order) {
		if p.feed != nil {
			p.stop(true)
			p.loading = false
			p.awaitFeed()
			return
		}
		at = 0
	}
	p.load(at)
}

// Prev plays the track before this one, however far this one has played:
// once a few seconds in, going back to its start only kept the listener
// pressing twice.
func (p *Player) Prev() {
	if len(p.order) == 0 {
		return
	}
	at := p.at - 1
	if at < 0 {
		at = len(p.order) - 1
	}
	p.load(at)
}

// Jump plays the queue's track i.
func (p *Player) Jump(i int) {
	for k, v := range p.order {
		if v == i {
			p.load(k)
			return
		}
	}
}

// DLNA returns the DLNA casting controller, or nil if unavailable.
func (p *Player) DLNA() *dlna.Controller {
	return p.dlnaCtrl
}

// IsCasting reports whether audio is currently routed to a DLNA device.
func (p *Player) IsCasting() bool {
	return p.dlnaCtrl != nil && p.dlnaCtrl.IsCasting()
}

// SwitchOutput changes the playback output between local speaker and a DLNA device.
// Passing "" selects the local speaker; passing a non-empty device ID selects that DLNA device.
func (p *Player) SwitchOutput(devID string) error {
	if p.dlnaCtrl == nil {
		return pluginapi.ErrUnavailable
	}
	active := p.dlnaCtrl.ActiveDevice()
	if (active == nil && devID == "") || (active != nil && active.ID == devID) {
		return nil
	}
	pos := p.Position()
	playing, at := p.castWantsPlay(), p.at
	if err := p.dlnaCtrl.SelectDevice(devID); err != nil {
		p.err = err.Error()
		p.sync()
		return err
	}

	if p.track == nil || p.at < 0 {
		p.sync()
		return nil
	}

	// Reload current track on new output target starting from current position
	p.loadSkipping(at, 0, playing, pos)
	return nil
}

// SetUseMPV toggles whether to prefer the MPV high-quality audio engine.
func (p *Player) SetUseMPV(use bool) {
	if p.useMPV == use {
		return
	}
	ready := mpv.Ready()
	reload := ready && p.track != nil && p.at >= 0 && !p.IsCasting() && !p.awaiting && (p.loading || p.Ready())
	var pos time.Duration
	if reload {
		pos = p.Position()
	}
	p.useMPV = use
	if !use && p.mpvOut != nil {
		p.mpvOut.Close()
		p.mpvOut = nil
		p.rgSet = false
	}
	if reload {
		p.loadTrack(p.at, 0, p.playing, pos, p.started)
	}
}

// UsingMPV reports whether the MPV high-quality audio engine is active.
func (p *Player) UsingMPV() bool {
	return p.useMPV && mpv.Ready()
}

// SetMPVConfig updates the configuration for MPV playback.
func (p *Player) SetMPVConfig(cfg mpv.Config) {
	p.mpvCfg = cfg
	if p.mpvOut != nil {
		p.mpvOut.SetEqualizer(cfg.EqualizerPreamp, cfg.EqualizerBands, cfg.EqualizerEnabled, cfg.EqualizerType)
		p.mpvOut.SetAudioDevice(cfg.AudioDevice)
	}
}

// SetAudioDevice updates the audio output device for MPV.
func (p *Player) SetAudioDevice(dev string) {
	p.mpvCfg.AudioDevice = dev
	if p.mpvOut != nil {
		p.mpvOut.SetAudioDevice(dev)
	}
}

// AudioDevices returns the available MPV audio output devices.
func (p *Player) AudioDevices() []mpv.AudioDevice {
	if p.mpvOut != nil {
		return p.mpvOut.AudioDevices()
	}
	return mpv.AudioDevices()
}

// MPVConfig returns the current MPV configuration.
func (p *Player) MPVConfig() mpv.Config {
	return p.mpvCfg
}

// MPVReady reports whether libmpv is ready to use on this system.
func (p *Player) MPVReady() bool {
	return mpv.Ready()
}

// TogglePlay pauses, fading out, or plays, fading in.
func (p *Player) TogglePlay() {
	if p.feedRetry {
		p.awaitFeed()
		return
	}
	if p.IsCasting() {
		p.toggleCast()
		return
	}
	if p.mpvOut != nil {
		if p.playing {
			p.playing = false
			p.fade.fade(0, fadeOut, func() {
				p.post(func() {
					if p.mpvOut != nil && !p.playing {
						p.mpvOut.Pause()
						p.sync()
					}
				})
			})
			p.sync()
			return
		}
		p.playing = true
		p.markStarted()
		p.mpvOut.Resume()
		p.fade.fade(1, fadeIn, nil)
		p.sync()
		return
	}
	if p.out == nil {
		switch {
		case p.loading:
			// It plays once loaded, or not.
			p.playing = !p.playing
			p.sync()
		case len(p.order) > 0:
			p.loadSkipping(max(p.at, 0), 0, true, p.pending)
		}
		return
	}
	out := p.out
	if p.playing {
		p.playing = false
		p.fade.fade(0, fadeOut, func() {
			p.post(func() {
				if p.out == out && !p.playing {
					out.Pause()
					p.sync()
				}
			})
		})
		p.sync()
		return
	}
	p.playing = true
	p.markStarted()
	if !p.seeking && !out.Playing() {
		if out.Position() >= out.Duration()-time.Second {
			out.Seek(0)
		}
		out.Play()
	}
	p.fade.fade(1, fadeIn, nil)
	p.sync()
}

// seekGap is how long a seek waits for what plays to fade out, which the
// audio player does over 5 ms, as it pauses: within the next buffer or
// two it sends the system.
const seekGap = 30 * time.Millisecond

// Seek moves to t. Playing, it pauses and plays again around the jump,
// which the audio player fades over a few milliseconds, rather than
// cutting from one sound to the other with a click.
func (p *Player) Seek(t time.Duration) {
	if p.IsCasting() {
		if p.track == nil {
			return
		}
		if status := p.dlnaCtrl.CastStatus(); p.cast != nil && status.ID == p.cast.id {
			_ = p.dlnaCtrl.Seek(p.trackContext(), t)
		} else {
			p.pending = t
		}
		p.sync()
		return
	}
	if p.mpvOut != nil {
		p.mpvOut.Seek(t)
		p.sync()
		return
	}
	if p.out == nil {
		p.pending = t
		p.notifyChanged()
		return
	}
	out := p.out
	// Paused, it jumps at once; but a seek on its way paused it only for
	// the jump, and this one takes its place, playing on from there.
	if !out.Playing() && !p.seeking {
		out.Seek(t)
		p.sync()
		return
	}
	p.seekGen++
	gen := p.seekGen
	p.seeking, p.seekTo = true, t
	out.Pause()
	time.AfterFunc(seekGap, func() {
		p.post(func() {
			if gen != p.seekGen || p.out != out {
				return
			}
			p.seeking = false
			out.Seek(t)
			if p.playing || !p.fade.silent() {
				out.Play()
			}
			p.sync()
		})
	})
	p.sync()
}

func (p *Player) SetVolume(v float64) {
	p.volume = v
	if p.IsCasting() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.dlnaCtrl.SetVolume(ctx, v)
	}
	p.applyGain()
}

// applyGain sets the volume of what plays: the listener's, which the ear
// hears in powers, and the track's gain, if normalizing.
func (p *Player) applyGain() {
	g := p.volume * p.volume
	normalize := p.playbackPreferences(p.track).Normalize
	if normalize {
		g *= p.gain
	}
	p.fade.setGain(g)
	if p.mpvOut != nil {
		db := 0.0
		if normalize && p.info != nil && p.info.Gain != 0 {
			db = p.info.Gain + normPreamp
		}
		// Setting replaygain is not free, and a volume change keeps it:
		// set it only when the track or the normalization changed it.
		if !p.rgSet || p.rgDB != db {
			p.mpvOut.SetReplayGainPreamp(db)
			p.rgDB, p.rgSet = db, true
		}
	}
}

// normPreamp is the extra gain normalized tracks get on top of their
// measured ReplayGain. At 0 it leaves the headroom ReplayGain measured; a
// positive value lifts quiet tracks, and their noise floor with them.
const normPreamp = 0

// normGain is the gain that brings a track as loud as others, as NetEase
// measured it, but never so loud that its peaks would clip.
func normGain(st *pluginapi.Stream) float64 {
	if st == nil || st.Gain == 0 && st.Peak == 0 {
		return 1
	}
	g := math.Pow(10, (st.Gain+normPreamp)/20)
	if st.Peak > 0 {
		g = min(g, 1/st.Peak)
	}
	return g
}

func (p *Player) Position() time.Duration {
	switch {
	case p.seeking:
		return p.seekTo
	case p.IsCasting():
		if status := p.dlnaCtrl.CastStatus(); p.cast != nil && status.ID == p.cast.id {
			return status.Position
		}
		return p.pending
	case p.loading:
		return p.pending
	case p.mpvOut != nil:
		return p.mpvOut.Position()
	case p.out == nil:
		return p.pending
	}
	return p.out.Position()
}

func (p *Player) Duration() time.Duration {
	if p.IsCasting() {
		if status := p.dlnaCtrl.CastStatus(); p.cast != nil && status.ID == p.cast.id && status.Duration > 0 {
			return status.Duration
		}
	}
	if p.track != nil && p.track.Length() > 0 {
		return p.track.Length()
	}
	if p.mpvOut != nil {
		if d := p.mpvOut.Duration(); d > 0 {
			return d
		}
	}
	return 0
}

// Levels returns the spectrum of what plays now.
func (p *Player) Levels() ([Bands]float32, bool) {
	if p.stream == nil || p.out == nil || !p.playing {
		return [Bands]float32{}, false
	}
	_, spec := p.stream.source()
	frame := int64(p.Position().Seconds() * float64(p.stream.SampleRate()))
	return spec.at(frame)
}

const playbackReleaseTimeout = 2 * time.Second

// Close stops playback after the application has saved the current session.
func (p *Player) Close() {
	p.gen++
	p.endRadio()
	segmented, _ := p.stream.(*hlsStream)
	p.stop(false)
	if p.mpvOut != nil {
		p.mpvOut.Close()
		p.mpvOut = nil
	}
	if p.dlnaCtrl != nil {
		_ = p.dlnaCtrl.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), playbackReleaseTimeout)
	defer cancel()
	released := make(chan struct{})
	go func() {
		defer close(released)
		p.castWorkers.Wait()
		if segmented != nil {
			waitForSegmentRelease(ctx, segmented.h)
		}
	}()
	select {
	case <-released:
	case <-ctx.Done():
	}
}

func waitForSegmentRelease(ctx context.Context, segments pluginapi.SegmentedSource) {
	if source, ok := segments.(interface{ Released() <-chan struct{} }); ok {
		select {
		case <-source.Released():
		case <-ctx.Done():
		}
	}
}

func (p *Player) stop(fade bool) {
	if p.cast != nil {
		p.dlnaCtrl.Stop()
		if p.cast.release != nil {
			p.cast.release()
		}
		p.cast = nil
	}
	if p.cancelLoad != nil {
		p.cancelLoad()
		p.cancelLoad = nil
	}
	p.loadContext = nil
	if out := p.out; out != nil {
		if fade {
			// Let the paused output fade out to avoid a click between tracks.
			out.Pause()
			time.AfterFunc(100*time.Millisecond, out.Close)
		} else {
			out.Close()
		}
	}
	if mpvOut := p.mpvOut; mpvOut != nil {
		mpvOut.Stop()
	}
	p.out, p.stream, p.info = nil, nil, nil
	p.seeking = false
	p.fade.attach(nil, 1)
}

// load plays the track at order[at], skipping those that cannot play.
func (p *Player) load(at int) {
	p.loadSkipping(at, 0, true, 0)
}

// loadSkipping loads the track at order[at], to play or not, from from,
// skipping those that cannot play: skipped of them so far.
func (p *Player) loadSkipping(at, skipped int, play bool, from time.Duration) {
	p.loadTrack(at, skipped, play, from, false)
}

// loadTrack can keep the start report when reloading the current track.
func (p *Player) loadTrack(at, skipped int, play bool, from time.Duration, started bool) {
	p.stop(true)
	p.gen++
	gen := p.gen
	ctx, cancel := context.WithCancel(context.Background())
	p.loadContext, p.cancelLoad = ctx, cancel
	p.at = at
	t := p.queue[p.order[at]]
	p.track = &t
	p.loading, p.playing, p.started, p.err = true, play, started, ""
	p.awaiting, p.feedRetry = false, false
	p.lyrics, p.lyricsLoaded = nil, false
	p.pending = from
	p.announce(&t, nil)
	p.notifyChanged()
	p.refill()

	if p.IsCasting() {
		p.cast = &castPlayback{id: p.castTrackID(&t, gen, at)}
	}

	go func() {
		cover, err := p.resolveArtwork(ctx, &t)
		if err != nil {
			cover = nil
		}
		if err == nil {
			p.post(func() {
				if gen == p.gen && ctx.Err() == nil {
					p.announce(&t, cover)
					if p.cast != nil {
						p.cast.cover = cover
						p.dlnaCtrl.UpdateCover(p.cast.id, cover)
					}
					p.notifyChanged()
				}
			})
		}
	}()

	go func() {
		lrc, trans, yrc, err := p.resolveLyrics(ctx, &t)
		var lines []lyrics.Line
		if err == nil {
			lines = lyrics.Build(lrc, trans, yrc)
		}
		p.post(func() {
			if gen == p.gen && ctx.Err() == nil {
				p.lyrics = lines
				p.lyricsLoaded = true
				p.notifyChanged()
			}
		})
	}()
	level := p.playbackPreferences(&t).Quality
	if p.cast != nil {
		p.loading = false
		if p.playing {
			p.loadCast(ctx, t, level, false)
		} else {
			p.sync()
		}
		return
	}
	direct := p.UsingMPV()
	go func() {
		media, err := p.open(ctx, t, level, direct)
		info, s := media.info, media.decoded
		if ctx.Err() != nil {
			if s != nil {
				s.Close()
			}
			return
		}
		p.post(func() {
			if gen != p.gen || ctx.Err() != nil {
				if s != nil {
					s.Close()
				}
				return
			}
			p.loading = false
			if err != nil {
				p.err = err.Error()
				if p.StreamError != nil {
					if message := p.StreamError(&t, err); message != "" {
						p.err = message
					}
				}
				// Skip what cannot play, unless nothing in the queue can, or
				// a few in a row could not: a NAS gone away fails every
				// song, which then flew by one after another.
				if skipped+1 < min(len(p.order), maxSkipped) {
					next := at + 1
					if next >= len(p.order) {
						next = 0
					}
					p.loadSkipping(next, skipped+1, p.playing, 0)
				} else {
					p.playing = false
					p.sync()
				}
				return
			}
			p.info = info
			p.gain = normGain(info)

			fadeLevel := 1.0
			if p.pending > 0 {
				fadeLevel = 0
			}

			// 2. High-quality MPV engine active.
			var decoderErr error
			if p.UsingMPV() {
				engine := p.mpvOut
				if engine == nil || engine.Closed() {
					engine, decoderErr = mpv.NewEngine(p.mpvCfg)
					if decoderErr != nil {
						log.Printf("start MPV decoder: %v", decoderErr)
					} else {
						p.mpvOut = engine
						p.rgSet = false
					}
				}
				var playURL string
				if engine != nil {
					playURL = p.mpvURL(ctx, &t, media)
				}

				if playURL != "" {
					engine.OnPlaybackRestart(func() {
						p.post(func() {
							if gen == p.gen && ctx.Err() == nil && p.mpvOut == engine {
								p.sync()
							}
						})
					})
					decoderErr = engine.Play(playURL, nil, p.pending)
					if decoderErr == nil {
						p.fade.attach(mpvVolumeOutput{p: p, e: p.mpvOut}, fadeLevel)
						p.applyGain()
						p.mpvOut.OnEnd(func() {
							p.post(func() {
								if gen == p.gen {
									p.ended()
								}
							})
						})
						p.mpvOut.OnError(func(msg string) {
							p.post(func() {
								if gen == p.gen {
									p.err = msg
									p.playing = false
									p.sync()
								}
							})
						})
						p.pending = 0
						if p.playing {
							p.mpvOut.Resume()
							p.fade.fade(1, fadeIn, nil)
							p.markStarted()
						} else {
							p.mpvOut.Pause()
						}
						if s != nil {
							s.Close()
						}
						p.sync()
						p.prefetchNextMPV(at, gen)
						return
					}
				}
			}

			// 3. Built-in pure Go player.
			if s == nil {
				p.err = "无法解码当前音频格式"
				if decoderErr != nil {
					p.err = fmt.Sprintf("MPV 播放失败（%v）", decoderErr)
				}
				p.playing = false
				p.sync()
				return
			}
			p.stream = s
			p.out = audio.NewPlayer(s)
			p.fade.attach(audioPlayerOutput{p.out}, fadeLevel)
			p.applyGain()
			p.out.OnEnd(func() {
				p.post(func() {
					if gen == p.gen {
						p.ended()
					}
				})
			})
			if p.pending > 0 {
				p.out.Seek(p.pending)
				p.pending = 0
			}
			if p.playing {
				p.out.Play()
				p.fade.fade(1, fadeIn, nil)
				p.markStarted()
			}
			p.sync()
		})
	}()
}

// mpvURL keeps authenticated streams available for this load and its prefetch.
func (p *Player) mpvURL(ctx context.Context, t *pluginapi.Track, media playbackSource) string {
	playURL := media.location()
	if p.dlnaCtrl == nil || p.dlnaCtrl.Proxy() == nil ||
		(!strings.HasPrefix(playURL, "http://") && !strings.HasPrefix(playURL, "https://")) {
		return playURL
	}
	proxy := p.dlnaCtrl.Proxy()
	ext := media.info.Type
	if ext == "" {
		ext = t.Format
	}
	trackID := fmt.Sprintf("mpv-%d-%d", t.ID, time.Now().UnixNano())
	if ext = strings.TrimPrefix(ext, "."); ext != "" {
		trackID += "." + ext
	}
	proxy.Register(trackID, dlna.StreamSource{
		RemoteURL:   playURL,
		Header:      media.info.Header,
		Renew:       media.info.Renew,
		ContentType: dlna.FormatToMIME(media.info.Type),
	})
	context.AfterFunc(ctx, func() { proxy.Unregister(trackID) })
	return proxy.LocalURL(trackID)
}

// prefetchNextMPV resolves and preloads the next song into MPV's gapless playlist.
func (p *Player) prefetchNextMPV(at, gen int) {
	if !p.UsingMPV() || p.repeat == RepeatOne || p.mpvOut == nil {
		return
	}
	nextAt := at + 1
	if nextAt >= len(p.order) {
		if p.repeat == RepeatAll {
			nextAt = 0
		} else {
			return
		}
	}
	if nextAt >= len(p.order) {
		return
	}
	nextTrack := p.queue[p.order[nextAt]]
	ctx, level := p.loadContext, p.playbackPreferences(&nextTrack).Quality
	go func() {
		media, ok := p.openNextOutput(ctx, nextTrack, level)
		if !ok {
			return
		}
		playURL := p.mpvURL(ctx, &nextTrack, media)
		p.post(func() {
			if gen == p.gen && ctx.Err() == nil && p.mpvOut != nil && !p.mpvOut.Closed() {
				_ = p.mpvOut.Append(playURL)
			}
		})
	}()
}

func (p *Player) playbackPreferences(track *pluginapi.Track) pluginapi.AudioPreferences {
	if p.TrackPreferences != nil && track != nil {
		return p.TrackPreferences(track)
	}
	prefs := pluginapi.AudioPreferences{Quality: "exhigh"}
	if p.Quality != nil {
		prefs.Quality = p.Quality()
	}
	if p.Normalize != nil {
		prefs.Normalize = p.Normalize()
	}
	return prefs
}

func (p *Player) trackOwner(t *pluginapi.Track) pluginapi.ID {
	owner := pluginapi.ID(t.Source)
	if p.TrackPlatform != nil {
		if registered := p.TrackPlatform(t); registered != "" {
			return registered
		}
	}
	return owner
}

func (p *Player) castTrackID(t *pluginapi.Track, generation, at int) string {
	return fmt.Sprintf("%x-%d-%d", trackFingerprint(trackCacheRef(p.trackOwner(t), t)), generation, at)
}

type playbackSource struct {
	info      *pluginapi.Stream
	decoded   stream
	localFile string
}

func (s playbackSource) location() string {
	if s.localFile != "" {
		return s.localFile
	}
	if s.info == nil {
		return ""
	}
	return s.info.URL
}

// External outputs can stage a complete cached file or a network stream.
func (p *Player) openNextOutput(ctx context.Context, t pluginapi.Track, level string) (playbackSource, bool) {
	media, err := p.open(ctx, t, level, true)
	if media.decoded != nil {
		media.decoded.Close()
	}
	return media, err == nil && ctx.Err() == nil && media.location() != ""
}

// open opens track t's audio at level: kept in the cache, or else
// streamed into it, and kept once it has all come in.
func (p *Player) open(ctx context.Context, t pluginapi.Track, level string, direct bool) (playbackSource, error) {
	if err := ctx.Err(); err != nil {
		return playbackSource{}, err
	}
	if info, decoded, err, handled := p.resolveAudio(ctx, &t, direct); handled {
		return playbackSource{info: info, decoded: decoded}, err
	}
	return p.openOrdinary(ctx, t, level, direct)
}

func (p *Player) openOrdinary(ctx context.Context, t pluginapi.Track, level string, direct bool) (playbackSource, error) {
	ref := trackCacheRef(p.trackOwner(&t), &t)
	dl, info, ok := p.Cache.lookup(ref, level)
	if !ok {
		var err error
		info, err = p.resolveStream(ctx, &t, level)
		if err != nil {
			return playbackSource{}, err
		}
		if p.Cache.promoteLegacy(ctx, ref, t.ID, info) {
			var cached *pluginapi.Stream
			if dl, cached, ok = p.Cache.lookup(ref, info.Level); ok {
				info = cached
			}
		}
	}
	if err := ctx.Err(); err != nil {
		if dl != nil {
			dl.Close()
		}
		return playbackSource{}, err
	}
	if direct {
		if !ok {
			return playbackSource{info: info}, nil
		}
		path := dl.file.Name()
		dl.Close()
		return playbackSource{info: info, localFile: path}, nil
	}
	if !ok {
		dl = p.Cache.download(ctx, ref, info)
	}
	s, err := openStream(dl, t.Duration)
	if err != nil {
		dl.Close()
		return playbackSource{}, err
	}
	return playbackSource{info: info, decoded: s}, nil
}

// Session is where playback was left, to pick up again.
type Session = pluginapi.PlaybackSession

// Session returns where playback is, or false with nothing queued.
func (p *Player) Session() (Session, bool) {
	if p.track == nil || p.at < 0 {
		return Session{}, false
	}
	return Session{
		Queue:    p.queue,
		Order:    p.order,
		At:       p.at,
		Shuffle:  p.shuffle,
		Repeat:   p.repeat,
		Position: p.Position().Milliseconds(),
		Radio:    p.radio,
	}, true
}

// Restore picks up a session where it was left, paused, and loads its
// track to play at once when asked.
func (p *Player) Restore(s Session) {
	if !s.ValidQueue() {
		return
	}
	p.queue, p.order = s.Queue, s.Order
	p.queueRevision++
	p.shuffle, p.repeat = s.Shuffle, s.Repeat%3
	p.loadSkipping(s.At, 0, false, time.Duration(s.Position)*time.Millisecond)
}

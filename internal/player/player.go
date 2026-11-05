package player

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player/av3a"
	"netease-music-mygo/internal/player/dlna"
	"netease-music-mygo/internal/player/mpv"
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

type RepeatMode int

const (
	RepeatOff RepeatMode = iota
	RepeatAll
	RepeatOne
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
	// Changed, if set, hears that what plays, where it is or whether it
	// plays changed.
	Changed func()

	queue   []netease.Track
	order   []int // the queue's indices in play order: shuffled or not
	at      int   // where in order the current track is, -1 for none
	shuffle bool
	repeat  RepeatMode
	volume  float64

	// A radio goes on with the tracks its feed gives, as the queue nears
	// its end, for as long as nothing else is played.
	radio    string
	feed     func(got func([]netease.Track))
	radioGen int  // counts radios, to drop what an old one gives
	feeding  bool // the feed is asked
	awaiting bool // at the queue's end, for what the feed gives

	track   *netease.Track
	out     *audio.Player
	stream  stream
	info    *netease.Stream // what the current track streams
	fade    fader
	gain    float64 // the current track's normalizing gain
	loading bool
	playing bool // what the listener wants: false while paused
	err     string
	gen     int // counts loads, to drop the results of stale ones

	lyrics       []lyrics.Line
	lyricsLoaded bool

	useMPV   bool
	mpvCfg   mpv.Config
	mpvOut   *mpv.Engine
	dlnaCtrl *dlna.Controller
	// vividDirs are the temporary MP4/WAV directories of Audio Vivid
	// tracks that played, removed when the next track loads.
	vividMu   sync.Mutex
	vividDirs []string

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
	if ctrl, err := dlna.NewController(func() { p.post(p.sync) }); err == nil {
		p.dlnaCtrl = ctrl
		p.dlnaCtrl.OnEnd(func() {
			p.post(func() {
				if p.IsCasting() {
					p.ended()
				}
			})
		})
	}
	// Music, mixed far enough ahead to play on while macOS keeps the app
	// waiting, as it does while its appearance changes.
	audio.SetLatency(audio.LatencyPlayback)
	// The media keys, and the controls of Control Center.
	mygo.NowPlaying.OnCommand(func(cmd mygo.MediaCommand, pos time.Duration) {
		switch cmd {
		case mygo.MediaPlay, mygo.MediaPause:
			if (cmd == mygo.MediaPlay) != p.playing {
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
	if p.Changed != nil {
		defer p.Changed()
	}
	if p.track == nil {
		mygo.NowPlaying.Clear()
		return
	}
	state := mygo.PlaybackPaused
	if p.playing {
		state = mygo.PlaybackPlaying
	}
	mygo.NowPlaying.SetPlayback(state, p.Position())
}

// announce shows the system the track, and its cover once loaded.
func (p *Player) announce(t *netease.Track, artwork []byte) {
	state := mygo.PlaybackPaused
	if p.playing {
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
func (p *Player) PlayQueue(tracks []netease.Track, start int) {
	if len(tracks) == 0 {
		return
	}
	p.endRadio()
	start = min(max(start, 0), len(tracks)-1)
	p.queue = append([]netease.Track(nil), tracks...)
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
func (p *Player) PlayRadio(name string, tracks []netease.Track, feed func(got func([]netease.Track))) {
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
func (p *Player) RadioFrom(name string, tracks []netease.Track, feed func(got func([]netease.Track))) {
	if p.track == nil {
		return
	}
	p.queue, p.order, p.at = []netease.Track{*p.track}, []int{0}, 0
	p.appendTracks(tracks)
	p.AttachRadio(name, feed)
}

// AttachRadio makes the queue the radio name's, which feed goes on with,
// as after restoring a session that played it.
func (p *Player) AttachRadio(name string, feed func(got func([]netease.Track))) {
	p.endRadio()
	p.radio, p.feed = name, feed
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
		kept := make([]netease.Track, p.at+1)
		for i, k := range p.order[:p.at+1] {
			kept[i] = p.queue[k]
		}
		p.queue, p.order = kept, make([]int, len(kept))
		for i := range p.order {
			p.order[i] = i
		}
	}
	if awaiting {
		// At its end, waiting for more: nothing plays on.
		p.playing = false
		p.sync()
	}
}

func (p *Player) endRadio() {
	p.radioGen++
	p.radio, p.feed, p.feeding, p.awaiting = "", nil, false, false
}

// Radio returns the name of the radio playing, or "".
func (p *Player) Radio() string { return p.radio }

// refill asks the radio for more once the queue is near its end.
func (p *Player) refill() {
	if p.feed == nil || p.feeding || p.at < len(p.order)-2 {
		return
	}
	p.feeding = true
	gen := p.radioGen
	p.feed(func(more []netease.Track) {
		if gen != p.radioGen {
			return
		}
		p.feeding = false
		if !p.acceptRefill(more) {
			return
		}
		switch {
		case !p.awaiting:
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
	p.awaiting = true
	p.refill()
}

// appendTracks adds tracks to the end of the queue, but those in it.
func (p *Player) appendTracks(tracks []netease.Track) {
	in := make(map[int64]bool, len(p.queue))
	for _, t := range p.queue {
		in[t.ID] = true
	}
	for _, t := range tracks {
		if !in[t.ID] {
			in[t.ID] = true
			p.order = append(p.order, len(p.queue))
			p.queue = append(p.queue, t)
		}
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
	if len(p.order) == 0 {
		return
	}
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

func (p *Player) CycleRepeat() { p.repeat = (p.repeat + 1) % 3 }

// Current returns the track playing or loading, or nil.
func (p *Player) Current() *netease.Track { return p.track }

// Queue returns the tracks queued, in the order they were given.
func (p *Player) Queue() []netease.Track { return p.queue }

// Playing reports what the listener wants: false while paused.
func (p *Player) Playing() bool { return p.playing }

// Loading reports whether the current track is still loading.
func (p *Player) Loading() bool { return p.loading || p.awaiting }

// Ready reports whether the current track has loaded, to play.
func (p *Player) Ready() bool {
	return p.out != nil || p.mpvOut != nil || p.IsCasting()
}

// Err returns why the current track did not play, or "".
func (p *Player) Err() string { return p.err }

// Stream returns what the current track streams, once loaded, or nil.
func (p *Player) Stream() *netease.Stream { return p.info }

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
type Audio interface {
	SampleRate() int
	Channels() int
	Read(dst []float32) (int, error)
	// SetPosition goes to a frame, of SampleRate a second.
	SetPosition(frame int64) error
	Close() error
}

// OpenAudio opens the current track's sound again, for decoding apart
// from what plays, as for transcribing an episode. It reads the same
// download, waiting for what has not come in, and fails once the track
// changes. It works on any goroutine once opened; close it when done.
func (p *Player) OpenAudio() (Audio, error) {
	if p.stream == nil || p.track == nil {
		return nil, errors.New("player: nothing loaded")
	}
	dl, _ := p.stream.source()
	return openShared(dl, p.track.Duration)
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
			p.stop()
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
func (p *Player) SwitchOutput(devID string) {
	if p.dlnaCtrl == nil {
		return
	}
	wasCasting := p.IsCasting()
	_ = p.dlnaCtrl.SelectDevice(devID)

	if p.track == nil || p.at < 0 {
		p.sync()
		return
	}

	pos := p.Position()
	playing := p.playing
	at := p.at

	// Stop previous output
	if wasCasting {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = p.dlnaCtrl.Stop(ctx)
		cancel()
	} else {
		p.stop()
	}

	// Reload current track on new output target starting from current position
	p.loadSkipping(at, 0, playing, pos)
}

// SetUseMPV toggles whether to prefer the MPV high-quality audio engine.
func (p *Player) SetUseMPV(use bool) {
	if p.useMPV != use {
		p.useMPV = use
		if !use && p.mpvOut != nil {
			p.mpvOut.Close()
			p.mpvOut = nil
		}
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
		p.mpvOut.SetEqualizer(cfg.EqualizerPreamp, cfg.EqualizerGains, cfg.EqualizerEnabled)
		p.mpvOut.SetAudioDevice(cfg.AudioDevice)
		p.mpvOut.SetAudioOptions(cfg.AudioChannels, cfg.AudioSpdif)
		enabled := true
		if p.info != nil {
			enabled = spatialProcessingFor(p.info.Level)
		}
		p.mpvOut.SetSpatialOptions(cfg.SpatialMode, cfg.HrtfProfile, enabled)
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

// SetEqualizer updates equalizer configuration and pushes to active MPV engine.
func (p *Player) SetEqualizer(preamp float64, gains [10]float64, enabled bool) {
	p.mpvCfg.EqualizerEnabled = enabled
	p.mpvCfg.EqualizerPreamp = preamp
	p.mpvCfg.EqualizerGains = gains
	if p.mpvOut != nil {
		p.mpvOut.SetEqualizer(preamp, gains, enabled)
	}
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
	if p.IsCasting() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if p.playing {
			p.playing = false
			_ = p.dlnaCtrl.Pause(ctx)
		} else {
			p.playing = true
			_ = p.dlnaCtrl.Resume(ctx)
		}
		p.sync()
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
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.dlnaCtrl.Seek(ctx, t)
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
	if p.Normalize != nil && p.Normalize() {
		g *= p.gain
	}
	p.fade.setGain(g)
	if p.mpvOut != nil {
		db := 0.0
		if p.Normalize != nil && p.Normalize() && p.info != nil && p.info.Gain != 0 {
			db = p.info.Gain + normPreamp
		}
		p.mpvOut.SetReplayGainPreamp(db)
	}
}

// normPreamp lifts normalized tracks, which NetEase brings to the quiet
// loudness of ReplayGain, nearer to how loud music plays elsewhere.
const normPreamp = 4

// normGain is the gain that brings a track as loud as others, as NetEase
// measured it, but never so loud that its peaks would clip.
func normGain(st *netease.Stream) float64 {
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
		_, pos, _ := p.dlnaCtrl.Status()
		return pos
	case p.mpvOut != nil:
		return p.mpvOut.Position()
	case p.out == nil:
		return p.pending
	}
	return p.out.Position()
}

func (p *Player) Duration() time.Duration {
	if p.track != nil && p.track.Length() > 0 {
		return p.track.Length()
	}
	if p.IsCasting() {
		_, _, dur := p.dlnaCtrl.Status()
		if dur > 0 {
			return dur
		}
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

func (p *Player) stop() {
	if out := p.out; out != nil {
		// Paused first, it fades out over a few milliseconds, rather than
		// stopping with a click.
		out.Pause()
		time.AfterFunc(100*time.Millisecond, out.Close)
	}
	if mpvOut := p.mpvOut; mpvOut != nil {
		mpvOut.Stop()
	}
	p.clearVividDirs()
	p.out, p.stream, p.info = nil, nil, nil
	p.seeking = false
	p.fade.attach(nil, 1)
}

// clearVividDirs drops the temporary files made to decode Audio Vivid.
func (p *Player) clearVividDirs() {
	p.vividMu.Lock()
	dirs := p.vividDirs
	p.vividDirs = nil
	p.vividMu.Unlock()
	for _, dir := range dirs {
		_ = os.RemoveAll(dir)
	}
}

// load plays the track at order[at], skipping those that cannot play.
func (p *Player) load(at int) {
	p.loadSkipping(at, 0, true, 0)
}

// loadSkipping loads the track at order[at], to play or not, from from,
// skipping those that cannot play: skipped of them so far.
func (p *Player) loadSkipping(at, skipped int, play bool, from time.Duration) {
	p.stop()
	p.gen++
	gen := p.gen
	p.at = at
	t := p.queue[p.order[at]]
	p.track = &t
	p.loading, p.playing, p.started, p.err = true, play, false, ""
	p.awaiting = false
	p.lyrics, p.lyricsLoaded = nil, false
	p.pending = from
	p.announce(&t, nil)
	p.refill()

	go func() {
		cover, err := p.resolveArtwork(&t)
		if err == nil {
			p.post(func() {
				if gen == p.gen {
					p.announce(&t, cover)
				}
			})
		}
	}()

	go func() {
		lrc, trans, yrc, err := p.resolveLyrics(&t)
		var lines []lyrics.Line
		if err == nil {
			lines = lyrics.Build(lrc, trans, yrc)
		}
		p.post(func() {
			if gen == p.gen {
				p.lyrics = lines
				p.lyricsLoaded = true
			}
		})
	}()
	go func() {
		level := "exhigh"
		if p.Quality != nil {
			level = p.Quality()
		}
		info, s, err := p.open(t, level)
		p.post(func() {
			if gen != p.gen {
				if s != nil {
					s.Close()
				}
				return
			}
			p.loading = false
			if err != nil {
				p.err = err.Error()
				switch {
				case err != netease.ErrUnavailable:
				case t.ProgramID != 0:
					p.err = "这期节目需要购买才能收听"
				default:
					p.err = "这首歌需要 VIP 才能播放"
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

			// 1. DLNA target active: stream via local proxy to remote device.
			if p.IsCasting() {
				if s != nil {
					s.Close()
				}
				headers := http.Header{}
				if info.Header != nil {
					headers = info.Header()
				}
				localFile := ""
				if p.Cache != nil {
					localFile = p.Cache.FilePath(t.ID, level)
				}
				trackID := fmt.Sprintf("%d", t.ID)
				meta := dlna.TrackMeta{
					ID:        trackID,
					Title:     t.Name,
					Artist:    t.ArtistNames(),
					Album:     t.Album.Name,
					CoverURL:  netease.Sized(t.Album.PicURL, 600),
					StreamURL: info.URL,
					LocalFile: localFile,
					Format:    info.Type,
					Duration:  t.Length(),
					Size:      info.Size,
					Headers:   headers,
				}
				if err := p.dlnaCtrl.PlayTrack(context.Background(), meta, p.pending); err != nil {
					p.err = fmt.Sprintf("无线投送失败: %v", err)
					p.playing = false
					p.sync()
					return
				}
				p.pending = 0
				p.markStarted()
				p.sync()
				return
			}

			// 2. High-quality MPV engine active.
			if p.UsingMPV() {
				engine := p.mpvOut
				if engine == nil || engine.Closed() {
					var err error
					engine, err = mpv.NewEngine(p.mpvCfg)
					if err != nil {
						p.err = err.Error()
						p.loading = false
						p.playing = false
						p.sync()
						return
					}
					p.mpvOut = engine
				}
				playURL := info.PlayPath
				if playURL == "" {
					playURL = info.URL
					cacheLevel := level
					if info.Level != "" {
						cacheLevel = info.Level
					}
					if p.Cache != nil {
						if f := p.Cache.FilePath(t.ID, cacheLevel); f != "" {
							playURL = f
						}
					}
				}
				// If streaming over network and we have a proxy, route via local proxy
				// to solve auth headers, cookies, range requests and MIME types cleanly.
				if (strings.HasPrefix(playURL, "http://") || strings.HasPrefix(playURL, "https://")) && p.dlnaCtrl != nil && p.dlnaCtrl.Proxy() != nil {
					headers := http.Header{}
					if info.Header != nil {
						headers = info.Header()
					}
					ext := info.Type
					if ext == "" {
						ext = t.Format
					}
					ext = strings.TrimPrefix(ext, ".")
					var trackID string
					if ext != "" {
						trackID = fmt.Sprintf("mpv-%d-%d.%s", t.ID, time.Now().UnixNano(), ext)
					} else {
						trackID = fmt.Sprintf("mpv-%d-%d", t.ID, time.Now().UnixNano())
					}
					p.dlnaCtrl.Proxy().Register(trackID, dlna.StreamSource{
						RemoteURL:   playURL,
						Headers:     headers,
						ContentType: dlna.FormatToMIME(info.Type),
					})
					playURL = p.dlnaCtrl.Proxy().LocalURL(trackID)
				}

				if playURL != "" {
					// jyeffect is already a spatialised stereo master;
					// applying crossfeed/HRTF again would colour it twice.
					engine.SetSpatialOptions(p.mpvCfg.SpatialMode, p.mpvCfg.HrtfProfile, spatialProcessingFor(info.Level))
					// Spatial streams (Vivid 7.1.4, sky 5.1, Dolby/E-AC-3)
					// must go through the normal CoreAudio mixer: hog mode
					// bypasses the system volume and can make macOS volume
					// keys / the menu slider appear dead. It can also fail
					// before E-AC-3 is decoded. Keep exclusive mode for
					// ordinary stereo PCM only.
					engine.SetExclusive(exclusiveFor(p.mpvCfg.Exclusive, info.Level))
					if err := engine.Play(playURL, nil, p.pending); err == nil {
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

// prefetchNextMPV resolves and preloads the next song into MPV's gapless playlist.
func (p *Player) prefetchNextMPV(at, gen int) {
	if !p.UsingMPV() || p.repeat == RepeatOne || p.mpvOut == nil {
		return
	}
	// MPV's af graph is global, so an appended track would inherit the
	// current track's HRTF/crossfeed filter. Headphone mode starts each
	// track normally and chooses its filter from that track's level.
	if p.mpvCfg.SpatialMode == "headphones" {
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
	go func() {
		level := "exhigh"
		if p.Quality != nil {
			level = p.Quality()
		}
		info, _, err := p.open(nextTrack, level)
		if err != nil || info == nil || (info.URL == "" && info.PlayPath == "") {
			return
		}
		playURL := info.PlayPath
		if playURL == "" {
			playURL = info.URL
			cacheLevel := level
			if info.Level != "" {
				cacheLevel = info.Level
			}
			if p.Cache != nil {
				if f := p.Cache.FilePath(nextTrack.ID, cacheLevel); f != "" {
					playURL = f
				}
			}
		}
		if (strings.HasPrefix(playURL, "http://") || strings.HasPrefix(playURL, "https://")) && p.dlnaCtrl != nil && p.dlnaCtrl.Proxy() != nil {
			headers := http.Header{}
			if info.Header != nil {
				headers = info.Header()
			}
			ext := info.Type
			if ext == "" {
				ext = nextTrack.Format
			}
			ext = strings.TrimPrefix(ext, ".")
			trackID := fmt.Sprintf("mpv-%d-%d", nextTrack.ID, time.Now().UnixNano())
			if ext != "" {
				trackID += "." + ext
			}
			p.dlnaCtrl.Proxy().Register(trackID, dlna.StreamSource{
				RemoteURL:   playURL,
				Headers:     headers,
				ContentType: dlna.FormatToMIME(info.Type),
			})
			playURL = p.dlnaCtrl.Proxy().LocalURL(trackID)
		}
		p.post(func() {
			if gen == p.gen && p.mpvOut != nil && !p.mpvOut.Closed() {
				_ = p.mpvOut.Append(playURL)
			}
		})
	}()
}

// spatialProcessingFor reports whether a level should go through the
// headphone spatial filter. jyeffect is already spatialised by NetEase and
// must not be processed twice; ordinary stereo has nothing to render.
func spatialProcessingFor(level string) bool {
	return netease.Spatial(level) && level != "jyeffect"
}

// exclusiveFor keeps hog mode for ordinary stereo PCM. Spatial streams
// need the CoreAudio mixer so macOS system volume keys and the menu slider
// keep working; hog mode also makes E-AC-3 initialization less reliable.
func exclusiveFor(pref bool, level string) bool {
	return pref && !netease.Spatial(level)
}

// effectiveLevel lowers a level the current output cannot play. Audio Vivid
// needs its helper and MPV; surround FLAC needs MPV; casting gets the
// stereo spatial level.
func (p *Player) effectiveLevel(level string) string {
	if level == "vivid" && !av3a.Ready() {
		level = "sky"
	}
	if p.IsCasting() && netease.Spatial(level) {
		return "jyeffect"
	}
	if !p.UsingMPV() && netease.Spatial(level) {
		switch level {
		case "vivid", "dolby", "sky":
			return "jyeffect"
		}
	}
	return level
}

// open opens track t's audio at level: kept in the cache, or else
// streamed into it, and kept once it has all come in.
func (p *Player) open(t netease.Track, level string) (*netease.Stream, stream, error) {
	level = p.effectiveLevel(level)
	if info, decoded, err, handled := p.resolveAudio(&t, level); handled {
		return info, decoded, err
	}
	if level == "vivid" && p.UsingMPV() {
		return p.openVivid(t)
	}
	dl, info, ok := p.Cache.lookup(t.ID, level)
	if !ok {
		var err error
		info, err = p.resolveStream(&t, level)
		if err != nil {
			return nil, nil, err
		}
		if p.UsingMPV() || p.IsCasting() {
			return info, nil, nil
		}
		dl = p.Cache.download(t.ID, info)
	}
	if p.UsingMPV() || p.IsCasting() {
		return info, nil, nil
	}
	s, err := openStream(dl, t.Duration)
	if err != nil {
		dl.Close()
		return nil, nil, err
	}
	return info, s, nil
}

// openVivid fetches the AV3A MP4, decodes it to a WAV with the helper, and
// hands that WAV to MPV. If the server served a lower level, or the helper
// cannot decode, it falls back to the surround FLAC stream instead.
func (p *Player) openVivid(t netease.Track) (*netease.Stream, stream, error) {
	info, err := p.resolveStream(&t, "vivid")
	if err != nil {
		return nil, nil, err
	}
	if info.Level != "vivid" {
		return info, nil, nil
	}
	if err := p.prepareVivid(t, info); err != nil {
		fallback, ferr := p.resolveStream(&t, "sky")
		if ferr == nil {
			return fallback, nil, nil
		}
		return nil, nil, err
	}
	return info, nil, nil
}

// prepareVivid downloads the MP4 to a temporary directory and decodes it
// into a WAV MPV can play. The directory is removed when the next track
// loads.
func (p *Player) prepareVivid(t netease.Track, info *netease.Stream) error {
	if p.Cache != nil {
		if cached := p.Cache.VividPath(t.ID); cached != "" {
			info.PlayPath = cached
			return nil
		}
	}
	dir, err := os.MkdirTemp("", fmt.Sprintf("mygo-vivid-%d-", t.ID))
	if err != nil {
		return err
	}
	p.vividMu.Lock()
	p.vividDirs = append(p.vividDirs, dir)
	p.vividMu.Unlock()
	ext := info.Type
	if ext != "mp4" && ext != "m4a" {
		ext = "mp4"
	}
	src := filepath.Join(dir, "source."+ext)
	if err := downloadStream(info, src); err != nil {
		return err
	}
	out := filepath.Join(dir, "decoded.wav")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := av3a.Decode(ctx, src, out); err != nil {
		return err
	}
	if p.Cache != nil {
		if cached, ok := p.Cache.keepVivid(t.ID, out); ok {
			info.PlayPath = cached
			return nil
		}
	}
	info.PlayPath = out
	return nil
}

// downloadStream fetches an authenticated stream URL into path.
func downloadStream(info *netease.Stream, path string) error {
	req, err := http.NewRequest(http.MethodGet, info.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", netease.UserAgent)
	if info.Header != nil {
		for k, v := range info.Header() {
			req.Header[k] = v
		}
	}
	res, err := (&http.Client{Timeout: 10 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("vivid: download: %s", res.Status)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, res.Body)
	return err
}

// Session is where playback was left, to pick up again.
type Session struct {
	Queue    []netease.Track `json:"queue"`
	Order    []int           `json:"order"`
	At       int             `json:"at"`
	Shuffle  bool            `json:"shuffle"`
	Repeat   RepeatMode      `json:"repeat"`
	Position int64           `json:"position"` // milliseconds
	Radio    string          `json:"radio,omitempty"`
}

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
	n := len(s.Queue)
	if n == 0 || len(s.Order) != n || s.At < 0 || s.At >= n {
		return
	}
	seen := make([]bool, n)
	for _, i := range s.Order {
		if i < 0 || i >= n || seen[i] {
			return
		}
		seen[i] = true
	}
	p.queue, p.order = s.Queue, s.Order
	p.shuffle, p.repeat = s.Shuffle, s.Repeat%3
	p.loadSkipping(s.At, 0, false, time.Duration(s.Position)*time.Millisecond)
}

package mpv

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	eqpkg "netease-music-mygo/internal/eq"
)

// AudioDevice describes an audio output device enumerated by MPV.
type AudioDevice struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

var (
	devsMu       sync.Mutex
	cachedDevs   []AudioDevice
	devsCachedAt time.Time
)

// AudioDevices returns the cached or probed list of audio devices.
func AudioDevices() []AudioDevice {
	devsMu.Lock()
	defer devsMu.Unlock()
	if time.Since(devsCachedAt) < 5*time.Second && cachedDevs != nil {
		return cachedDevs
	}
	if !Ready() {
		return cachedDevs
	}
	inst, err := NewInstance(map[string]string{"ao": "null"})
	if err != nil {
		return cachedDevs
	}
	defer inst.Close()
	raw := inst.GetProperty("audio-device-list")
	if raw != "" {
		var list []AudioDevice
		if err := json.Unmarshal([]byte(raw), &list); err == nil {
			cachedDevs = list
			devsCachedAt = time.Now()
		}
	}
	return cachedDevs
}

// Config holds user choices for MPV audio playback.
type Config struct {
	// Exclusive enables bit-perfect exclusive mode (e.g. WASAPI exclusive on Windows).
	Exclusive bool
	// ReplayGain sets audio volume normalization ("no", "album", "track").
	ReplayGain string
	// AudioDevice specifies output audio device, or "" for default.
	AudioDevice string
	// Gapless enables gapless playback for consecutive tracks.
	Gapless bool
	// Equalizer settings
	EqualizerEnabled bool
	EqualizerType    string
	EqualizerPreamp  float64
	EqualizerBands   []float64
	// NullAO uses a dummy null audio output (for testing/headless environments).
	NullAO bool
}

// Engine wraps an active MPV instance to drive audio playback.
type Engine struct {
	mu     sync.RWMutex
	inst   *Instance
	cfg    Config
	closed bool

	onEnd   func()
	onError func(string)

	playing   bool
	position  time.Duration
	duration  time.Duration
	volume    float64
	eqEnabled bool
	eqType    string
	eqPreamp  float64
	eqBands   []float64

	eventDone chan struct{}
}

// NewEngine creates and configures a high-quality audio playback engine.
func NewEngine(cfg Config) (*Engine, error) {
	// weak keeps files of the same format gapless, and reopens the device
	// when the format changes, rather than resampling them to the first
	// file's rate as yes does. Off has to be said: MPV defaults to weak.
	gapless := "no"
	if cfg.Gapless {
		gapless = "weak"
	}
	opts := map[string]string{
		"stream-lavf-o":  "seekable=1,reconnect=1,reconnect_streamed=1,reconnect_delay_max=5",
		"force-seekable": "yes",
		"gapless-audio":  gapless,
	}
	if cfg.NullAO {
		opts["ao"] = "null"
	}
	if cfg.Exclusive {
		opts["audio-exclusive"] = "yes"
	}
	if cfg.AudioDevice != "" {
		opts["audio-device"] = cfg.AudioDevice
	}
	inst, err := NewInstance(opts)
	if err != nil {
		return nil, err
	}

	// Apply audiophile options
	if cfg.ReplayGain != "" && cfg.ReplayGain != "no" {
		inst.SetProperty("replaygain", cfg.ReplayGain)
		inst.SetProperty("replaygain-clip", "yes")
	}
	if cfg.AudioDevice != "" {
		inst.SetProperty("audio-device", cfg.AudioDevice)
	}

	e := &Engine{
		inst:      inst,
		cfg:       cfg,
		volume:    0.8,
		eqEnabled: cfg.EqualizerEnabled,
		eqType:    cfg.EqualizerType,
		eqPreamp:  cfg.EqualizerPreamp,
		eqBands:   cfg.EqualizerBands,
		eventDone: make(chan struct{}),
	}

	// Apply initial equalizer filters if enabled
	e.applyAudioFiltersLocked()

	// Set initial volume
	inst.SetProperty("volume", fmt.Sprintf("%.1f", e.volume*100))

	go e.runEvents()

	return e, nil
}

// Play loads and starts playing a stream or local file.
func (e *Engine) Play(sourceURL string, headers map[string]string, startPos time.Duration) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed || e.inst == nil {
		return fmt.Errorf("mpv: engine closed")
	}

	// Set HTTP request headers if any
	if len(headers) > 0 {
		var hList []string
		for k, v := range headers {
			hList = append(hList, fmt.Sprintf("%s: %s", k, v))
		}
		e.inst.SetProperty("http-header-fields", strings.Join(hList, ", "))
	}

	// Load file
	code := e.inst.Command([]string{"loadfile", sourceURL, "replace"})
	if code < 0 {
		return fmt.Errorf("mpv: loadfile failed with error %d", code)
	}

	e.position = startPos
	e.duration = 0

	// If start position is specified, seek right away
	if startPos > 0 {
		e.inst.Command([]string{"seek", fmt.Sprintf("%.3f", startPos.Seconds()), "absolute"})
	}

	e.inst.SetProperty("pause", "no")
	e.playing = true
	return nil
}

// Pause pauses playback.
func (e *Engine) Pause() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.inst != nil {
		e.inst.SetProperty("pause", "yes")
		// Exclusive holds the audio device. Let another app have it while
		// paused; Resume takes it back.
		if e.cfg.Exclusive {
			e.inst.SetProperty("audio-exclusive", "no")
		}
		e.playing = false
	}
}

// Resume unpauses playback.
func (e *Engine) Resume() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.inst != nil {
		if e.cfg.Exclusive {
			e.inst.SetProperty("audio-exclusive", "yes")
		}
		e.inst.SetProperty("pause", "no")
		e.playing = true
	}
}

// Playing reports whether the engine is currently playing.
func (e *Engine) Playing() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.playing
}

// Seek seeks to a target position.
func (e *Engine) Seek(pos time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.inst != nil {
		e.inst.Command([]string{"seek", fmt.Sprintf("%.3f", pos.Seconds()), "absolute+exact"})
		e.position = pos
	}
}

// Position returns current playback position.
func (e *Engine) Position() time.Duration {
	e.mu.RLock()
	inst := e.inst
	cached := e.position
	e.mu.RUnlock()

	if inst == nil {
		return cached
	}
	s := inst.GetProperty("time-pos")
	if s == "" {
		return cached
	}
	pos := parseSecs(s)
	if pos > 0 {
		e.mu.Lock()
		e.position = pos
		e.mu.Unlock()
	}
	return pos
}

// Duration returns total length of current track.
func (e *Engine) Duration() time.Duration {
	e.mu.RLock()
	inst := e.inst
	cached := e.duration
	e.mu.RUnlock()

	if inst == nil {
		return cached
	}
	s := inst.GetProperty("duration")
	if s == "" {
		return cached
	}
	d := parseSecs(s)
	if d > 0 {
		e.mu.Lock()
		e.duration = d
		e.mu.Unlock()
	}
	return d
}

// Volume returns the current volume between 0 and 1.
func (e *Engine) Volume() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.volume
}

// SetVolume sets the volume between 0 and 1.
func (e *Engine) SetVolume(vol float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.volume = vol
	if e.inst != nil {
		e.inst.SetProperty("volume", fmt.Sprintf("%.1f", vol*100))
	}
}

// SetReplayGainPreamp sets replaygain pre-amplification in dB.
// If db != 0, enables replaygain normalization with the given gain in dB.
// If db == 0, falls back to neutral or cfg.ReplayGain.
func (e *Engine) SetReplayGainPreamp(db float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.inst != nil {
		if db != 0 {
			rgMode := e.cfg.ReplayGain
			if rgMode == "" || rgMode == "no" {
				rgMode = "track"
			}
			e.inst.SetProperty("replaygain", rgMode)
			e.inst.SetProperty("replaygain-preamp", fmt.Sprintf("%.2f", db))
			e.inst.SetProperty("replaygain-fallback", fmt.Sprintf("%.2f", db))
			e.inst.SetProperty("replaygain-clip", "yes")
		} else {
			if e.cfg.ReplayGain != "" && e.cfg.ReplayGain != "no" {
				e.inst.SetProperty("replaygain", e.cfg.ReplayGain)
				e.inst.SetProperty("replaygain-preamp", "0")
				e.inst.SetProperty("replaygain-fallback", "0")
			} else {
				e.inst.SetProperty("replaygain", "no")
				e.inst.SetProperty("replaygain-preamp", "0")
				e.inst.SetProperty("replaygain-fallback", "0")
			}
		}
	}
}

// SetEqualizer configures and applies the graphic equalizer filter.
func (e *Engine) SetEqualizer(preamp float64, bands []float64, enabled bool, typ string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.eqPreamp = preamp
	e.eqBands = bands
	e.eqEnabled = enabled
	e.eqType = typ
	e.applyAudioFiltersLocked()
}

func (e *Engine) applyAudioFiltersLocked() {
	if e.inst == nil {
		return
	}
	var filters []string
	if e.eqEnabled {
		if f := eqpkg.BuildEqualizerFilter(e.eqPreamp, e.eqBands, e.eqType); f != "" {
			filters = append(filters, f)
		}
	}
	e.inst.SetProperty("af", strings.Join(filters, ","))
}

// OnEnd sets the callback to invoke when a track reaches its natural end.
func (e *Engine) OnEnd(fn func()) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onEnd = fn
}

// OnError sets the callback to invoke when an audio loading or decoding failure occurs.
func (e *Engine) OnError(fn func(string)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onError = fn
}

func (e *Engine) runEvents() {
	for {
		e.mu.RLock()
		inst := e.inst
		closed := e.closed
		e.mu.RUnlock()

		if closed || inst == nil {
			return
		}

		evID, _, endReason := inst.WaitEvent(0.2)
		if evID != EventNone {
			fmt.Printf("[mpv-event] id=%d reason=%d\n", evID, endReason)
		}
		switch evID {
		case EventShutdown:
			return
		case EventEndFile:
			if endReason == EndFileReasonError {
				e.mu.Lock()
				e.playing = false
				onError := e.onError
				e.mu.Unlock()
				if onError != nil {
					onError("音频加载或解码失败")
				}
				continue
			}
			if endReason == EndFileReasonEOF {
				e.mu.RLock()
				onEnd := e.onEnd
				e.mu.RUnlock()
				if onEnd != nil {
					onEnd()
				}
			}
		case EventFileLoaded, EventPlaybackRestart:
			e.mu.Lock()
			e.playing = true
			e.mu.Unlock()
		}
	}
}

// Append adds a track to the end of MPV's internal playlist for gapless prefetching.
func (e *Engine) Append(sourceURL string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.inst == nil {
		return fmt.Errorf("mpv: engine closed")
	}
	code := e.inst.Command([]string{"loadfile", sourceURL, "append"})
	if code < 0 {
		return fmt.Errorf("mpv: loadfile append failed with code %d", code)
	}
	return nil
}

// Stop stops playback and clears playlist without destroying the engine.
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.inst != nil && !e.closed {
		e.playing = false
		e.position = 0
		e.duration = 0
		e.inst.Command([]string{"stop"})
	}
}

// Closed reports whether the engine has been shut down.
func (e *Engine) Closed() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.closed || e.inst == nil
}

// SetOption sets an MPV option.
func (e *Engine) SetOption(name, value string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.inst != nil {
		return e.inst.SetOption(name, value)
	}
	return -1
}

// RequestLogMessages requests MPV log messages as events.
func (e *Engine) RequestLogMessages(minLevel string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.inst != nil {
		e.inst.RequestLogMessages(minLevel)
	}
}

// AudioDevices returns the list of audio output devices reported by MPV.
func (e *Engine) AudioDevices() []AudioDevice {
	e.mu.RLock()
	inst := e.inst
	e.mu.RUnlock()
	if inst == nil {
		return AudioDevices()
	}
	devsMu.Lock()
	defer devsMu.Unlock()
	if time.Since(devsCachedAt) < 3*time.Second && cachedDevs != nil {
		return cachedDevs
	}
	raw := inst.GetProperty("audio-device-list")
	if raw != "" {
		var list []AudioDevice
		if err := json.Unmarshal([]byte(raw), &list); err == nil {
			cachedDevs = list
			devsCachedAt = time.Now()
			return list
		}
	}
	return cachedDevs
}

// SetAudioDevice dynamically sets the audio output device (e.g. "auto", or device identifier).
func (e *Engine) SetAudioDevice(dev string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cfg.AudioDevice = dev
	if e.inst != nil {
		if dev == "" {
			dev = "auto"
		}
		e.inst.SetProperty("audio-device", dev)
	}
}

// Close terminates the engine.
// GetProperty returns the string value of an MPV property.
func (e *Engine) GetProperty(prop string) string {
	e.mu.RLock()
	inst := e.inst
	e.mu.RUnlock()
	if inst == nil {
		return ""
	}
	return inst.GetProperty(prop)
}

func (e *Engine) Close() {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.closed = true
	inst := e.inst
	e.inst = nil
	e.mu.Unlock()

	if inst != nil {
		inst.Close()
	}
}

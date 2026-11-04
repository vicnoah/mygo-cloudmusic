package mpv

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

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
	// NullAO uses a dummy null audio output (for testing/headless environments).
	NullAO bool
}

// Engine wraps an active MPV instance to drive audio playback.
type Engine struct {
	mu     sync.RWMutex
	inst   *Instance
	cfg    Config
	closed bool

	onEnd func()

	playing  bool
	position time.Duration
	duration time.Duration
	volume   float64

	eventDone chan struct{}
}

// NewEngine creates and configures a high-quality audio playback engine.
func NewEngine(cfg Config) (*Engine, error) {
	opts := map[string]string{
		"stream-lavf-o":  "seekable=1",
		"force-seekable": "yes",
	}
	if cfg.NullAO {
		opts["ao"] = "null"
	}
	inst, err := NewInstance(opts)
	if err != nil {
		return nil, err
	}

	// Apply audiophile options
	if cfg.Exclusive {
		inst.SetProperty("audio-exclusive", "yes")
	}
	if cfg.ReplayGain != "" && cfg.ReplayGain != "no" {
		inst.SetProperty("replaygain", cfg.ReplayGain)
		inst.SetProperty("replaygain-clip", "yes")
	}
	if cfg.Gapless {
		inst.SetProperty("gapless-audio", "yes")
	}
	if cfg.AudioDevice != "" {
		inst.SetProperty("audio-device", cfg.AudioDevice)
	}

	e := &Engine{
		inst:      inst,
		cfg:       cfg,
		volume:    0.8,
		eventDone: make(chan struct{}),
	}

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
		e.playing = false
	}
}

// Resume unpauses playback.
func (e *Engine) Resume() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.inst != nil {
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
		e.inst.Command([]string{"seek", fmt.Sprintf("%.3f", pos.Seconds()), "absolute"})
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

// SetVolume sets the volume between 0 and 1.
func (e *Engine) SetVolume(vol float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.volume = vol
	if e.inst != nil {
		e.inst.SetProperty("volume", fmt.Sprintf("%.1f", vol*100))
	}
}

// OnEnd sets the callback to invoke when a track reaches its end.
func (e *Engine) OnEnd(fn func()) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onEnd = fn
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

		evID, _ := inst.WaitEvent(0.2)
		if evID != EventNone {
			fmt.Printf("[mpv-event] id=%d\n", evID)
		}
		switch evID {
		case EventShutdown:
			return
		case EventEndFile:
			e.mu.RLock()
			onEnd := e.onEnd
			e.mu.RUnlock()
			if onEnd != nil {
				onEnd()
			}
		case EventFileLoaded, EventPlaybackRestart:
			e.mu.Lock()
			e.playing = true
			e.mu.Unlock()
		}
	}
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

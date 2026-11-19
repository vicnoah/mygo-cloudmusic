package pluginapi

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi/eq"
	"waygu-music/internal/pluginapi/lyrics"
)

type RepeatMode int

const (
	RepeatOff RepeatMode = iota
	RepeatAll
	RepeatOne
)

// PlaybackControls is the audio engine's state and commands.
type PlaybackControls interface {
	Current() *Track
	Playing() bool
	Loading() bool
	Ready() bool
	Err() string
	Position() time.Duration
	Duration() time.Duration
	Downloaded() (float64, bool)
	TogglePlay()
	Prev()
	Next()
	Seek(time.Duration)
	Shuffle() bool
	ToggleShuffle()
	Repeat() RepeatMode
	CycleRepeat()
	Queue() []Track
	PlayOrder() ([]int, int)
	Jump(int)
	Lyrics() ([]lyrics.Line, bool)
	Stream() *Stream
	Levels() ([SpectrumBands]float32, bool)
	Radio() string
	EndRadio()
}

// PlaybackLoadingText describes preparation if the host provides more detail.
func PlaybackLoadingText(p PlaybackControls) string {
	if detail, ok := p.(interface{ LoadingText() string }); ok {
		return detail.LoadingText()
	}
	return "正在缓冲…"
}

// DecodedArtwork keeps the cover and its blurred bitmap alive while displayed.
type DecodedArtwork interface {
	Bitmap() *ui.Bitmap
	Blurred() *ui.Bitmap
	Tint() ui.Color
	Failed() bool
}

type Presentation struct {
	Smoke, Fog, LyricsStage, StageSparks bool
	StageStyle                           string
}

type DesktopLyricsState struct {
	Locked, PointerInside, ShowTranslation bool
	FontSize                               float32
	Color                                  int
}

type CastingDevice struct {
	ID, Name string
}

type CastingState struct {
	Available, Sending, Scanning bool
	Devices                      []CastingDevice
	Active                       *CastingDevice
}

// DesktopPlaybackServices connects playback to a plugin's window.
type DesktopPlaybackServices interface {
	Playback() PlaybackControls
	Volume() float64
	SetVolume(float64)
	Presentation() Presentation
	SetLyricsPresentation(stage bool, style string, sparks bool)
	DecodedArtwork(url string, size int, blurred bool) DecodedArtwork
	SetMainWindowButtonsTheme(mygo.ThemeSource)
	DesktopLyricsVisible() bool
	ToggleDesktopLyrics()
	DesktopLyricsLocked() bool
	ToggleDesktopLyricsLock()
	DesktopLyricsState() DesktopLyricsState
	SetDesktopLyricsStyle(translation bool, size float32, color int)
	HideDesktopLyrics()
	Casting() CastingState
	ScanCastingDevices()
	SwitchPlaybackOutput(string)
	EqualizerAvailable() bool
	EqualizerSettings() eq.Settings
	ApplyEqualizer(eq.Settings)
}

// Audio decodes a second copy of the current track. Read returns
// interleaved PCM at SampleRate frames per second.
type Audio interface {
	SampleRate() int
	Channels() int
	Read(dst []float32) (int, error)
	// SetPosition goes to a frame, of SampleRate a second.
	SetPosition(frame int64) error
	Close() error
}

// PlaybackSession persists the queue and its paused playback position.
type PlaybackSession struct {
	Queue    []Track    `json:"queue"`
	Order    []int      `json:"order"`
	At       int        `json:"at"`
	Shuffle  bool       `json:"shuffle"`
	Repeat   RepeatMode `json:"repeat"`
	Position int64      `json:"position"` // milliseconds
	Radio    string     `json:"radio,omitempty"`
}

// PlaybackContinuity provides paused queue restoration, resumable radio
// queues, and independent decoding for features such as transcription.
// Use it on the application thread during the activation that supplied it.
type PlaybackContinuity interface {
	LoadSession() (PlaybackSession, bool)
	Restore(PlaybackSession)
	RadioFrom(string, []Track, func(func([]Track)))
	AttachRadio(string, func(func([]Track)))
	OpenAudio() (Audio, error)
}

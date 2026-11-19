package pluginapi

import (
	"context"
	"time"
)

// SpectrumBands is the number of frequency bands sampled during playback.
const SpectrumBands = 24

// SegmentedSource supplies encoded audio while the engine handles decoding and seeking.
type SegmentedSource interface {
	SegmentCount() int
	SegmentDuration(int) time.Duration
	TotalDuration() time.Duration
	FetchSegment(context.Context, int) ([]byte, error)
	Close()
}

// PlaybackMedia describes how a platform serves its own tracks.
type PlaybackMedia struct {
	Preferences func() AudioPreferences
	Stream      func(context.Context, *Track, string) (*Stream, error)
	Cover       func(context.Context, *Track) ([]byte, error)
	Lyrics      func(context.Context, *Track) (string, string, string, error)
	Started     func(*Track)

	// CastSource optionally requests an output-specific source. An empty format
	// asks for any custom source selected by the platform preferences. Returning
	// no source lets the host use the normal URL, or report that a requested
	// compatible format is unavailable. The host does not encode audio.
	CastSource func(context.Context, *Track, string) (*Stream, SegmentedSource, error)

	// ErrorMessage optionally describes a failed stream in the platform's terms.
	ErrorMessage func(*Track, error) string

	// Segments is optional. A nil source and nil error use the ordinary stream path.
	// Advanced reports whether the current output accepts formats beyond the built-in decoder.
	Segments func(ctx context.Context, track *Track, advanced bool) (*Stream, SegmentedSource, error)
}

// AudioPreferences describes the stream quality and normalization selected
// by the plugin that owns the active playback controls.
type AudioPreferences struct {
	Quality   string
	Normalize bool
}

package fnmusic

import (
	"context"
	"time"

	"waygu-music/internal/pluginapi"
	backend "waygu-music/internal/plugins/fnmusic/internal/client"
)

const ID = backend.PlatformID

func Registration() pluginapi.Registration {
	return pluginapi.Registration{
		Info:  pluginapi.Info{ID: ID, Name: "飞牛音乐", Icon: iconRadio},
		Start: Start, LegacySettingsKey: string(ID),
	}
}

// Adapter owns one complete Feiniu context. The root app stores only this
// interface, so provider state and UI do not leak into shared app files.
type Adapter struct {
	state   *State
	runtime *RuntimeAdapter
	home    *Home
}

var _ pluginapi.Adapter = (*Adapter)(nil)

func NewAdapter(host Host, settings *platformPreferences, save ...func()) *Adapter {
	state := NewState()
	if settings != nil {
		cfg := fromConnectionConfig(settings.Config)
		LoadCredentials(&cfg)
		state.SetConfig(cfg)
		if cfg.Valid() {
			// Mark the restored session as loading before the first window
			// frame. The plugin connects when the window is ready, so this
			// prevents a configured account from
			// flashing the connection form once at startup.
			state.setConnecting(state.Generation(), true)
		}
	}
	if host != nil {
		state.SetPost(host.Post)
	}
	var persist func()
	if len(save) > 0 {
		persist = save[0]
	}
	runtime := NewRuntime(state, settings, persist)
	return &Adapter{state: state, runtime: runtime, home: NewHome(state, host, runtime)}
}

func (a *Adapter) Home() *Home {
	if a == nil {
		return nil
	}
	return a.home
}

func (a *Adapter) Configured() bool { return a.runtime.Configured() }

func (a *Adapter) FetchArtwork(ctx context.Context, ref string, size int) ([]byte, bool, error) {
	return a.runtime.FetchArtwork(ctx, ref, size)
}

func (a *Adapter) OwnsTrack(tr *pluginapi.Track) bool {
	return tr != nil && tr.Source == backend.Source
}

func (a *Adapter) PlaybackMedia() pluginapi.PlaybackMedia {
	s := a.state
	return pluginapi.PlaybackMedia{
		Segments: func(ctx context.Context, tr *pluginapi.Track, advanced bool) (*pluginapi.Stream, pluginapi.SegmentedSource, error) {
			client := s.Client()
			if client == nil {
				return nil, nil, nil
			}
			settings := a.runtime.Transcode()
			mode := backend.TranscodeAuto
			switch settings.Mode {
			case transcodeNative:
				mode = backend.TranscodeNative
			case transcodeAlways:
				mode = backend.TranscodeAlways
			}
			if mode == backend.TranscodeAlways || (!advanced && backend.ShouldTranscode(tr, mode)) {
				h, err := client.TranscodeHLSWithBitrate(ctx, tr, settings.Bitrate)
				if err != nil {
					return nil, nil, err
				}
				return &pluginapi.Stream{Type: "mp3", Level: "exhigh"}, hlsSource{h}, nil
			}
			if !advanced && (!backend.Native(tr.Format) || tr.Cue) {
				return nil, nil, backend.NativePlaybackError(tr)
			}
			return nil, nil, nil
		},
		Stream: func(ctx context.Context, tr *pluginapi.Track, _ string) (*pluginapi.Stream, error) {
			client := s.Client()
			if client == nil {
				return nil, backend.ErrSignedOut
			}
			return client.WithContext(ctx).Stream(tr)
		},
		Cover: func(ctx context.Context, tr *pluginapi.Track) ([]byte, error) {
			provider := s.Provider()
			if provider == nil {
				return nil, backend.ErrSignedOut
			}
			return provider.Cover(ctx, tr.Album.PicURL, 600)
		},
		Lyrics: func(ctx context.Context, tr *pluginapi.Track) (string, string, string, error) {
			provider := s.Provider()
			if provider == nil {
				return "", "", "", backend.ErrSignedOut
			}
			l, err := provider.Lyrics(ctx, tr)
			return l.LRC, l.Translation, l.YRC, err
		},
		Started: func(tr *pluginapi.Track) {
			if a.home != nil {
				a.home.ReportStarted(tr)
			}
		},
	}
}

// hlsSource is the small provider-to-player translation for Feiniu HLS.
type hlsSource struct{ h *backend.HLS }

func (s hlsSource) SegmentCount() int                   { return len(s.h.Segments) }
func (s hlsSource) SegmentDuration(i int) time.Duration { return s.h.Segments[i].Duration }
func (s hlsSource) TotalDuration() time.Duration        { return s.h.Total }
func (s hlsSource) FetchSegment(ctx context.Context, i int) ([]byte, error) {
	return s.h.Fetch(ctx, i)
}
func (s hlsSource) Close() { s.h.Release() }

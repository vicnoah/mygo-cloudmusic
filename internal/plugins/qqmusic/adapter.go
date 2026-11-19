package qqmusic

import (
	"context"
	"strings"

	"waygu-music/internal/pluginapi"
	backend "waygu-music/internal/plugins/qqmusic/internal/client"
)

const (
	ID     = backend.PlatformID
	Source = string(ID)
)

func Registration() pluginapi.Registration {
	return pluginapi.Registration{
		Info:  pluginapi.Info{ID: ID, Name: "QQ 音乐", Icon: iconNote},
		Start: Start, DeveloperOnly: true, LegacySettingsKey: string(ID),
	}
}

type Adapter struct {
	home *Home
}

var _ pluginapi.Adapter = (*Adapter)(nil)

func NewAdapter(host Host, settings *platformPreferences, save ...func()) *Adapter {
	home := NewHome(NewState(), host)
	home.preferences = settings
	if len(save) > 0 {
		home.savePreferences = save[0]
	}
	return &Adapter{home: home}
}

func (a *Adapter) Home() *Home {
	if a == nil {
		return nil
	}
	return a.home
}

// QQ Music can browse and play before a listener signs in.
func (*Adapter) Configured() bool { return true }

func (a *Adapter) OwnsTrack(tr *pluginapi.Track) bool {
	return tr != nil && tr.Source == Source
}

func (*Adapter) FetchArtwork(ctx context.Context, raw string, size int) ([]byte, bool, error) {
	if raw == "" {
		return nil, true, nil
	}
	if !strings.HasPrefix(raw, "http") {
		return nil, false, nil
	}
	b, err := backend.Fetch(ctx, backend.Sized(raw, size))
	return b, true, err
}

func (a *Adapter) PlaybackMedia() pluginapi.PlaybackMedia {
	return pluginapi.PlaybackMedia{
		Preferences: func() pluginapi.AudioPreferences {
			out := pluginapi.AudioPreferences{Quality: "exhigh", Normalize: true}
			if s := a.home.preferences; s != nil {
				if s.Quality != "" {
					out.Quality = s.Quality
				}
				out.Normalize = s.Normalize == nil || *s.Normalize
			}
			return out
		},
		Stream: func(ctx context.Context, tr *pluginapi.Track, level string) (*pluginapi.Stream, error) {
			s, err := backend.SongURL(ctx, tr.ID, level)
			if err != nil {
				return nil, err
			}
			return &pluginapi.Stream{URL: s.URL, Size: s.Size, Type: s.Type, Level: s.Level, Gain: s.Gain, Peak: s.Peak}, nil
		},
		CastSource: func(ctx context.Context, tr *pluginapi.Track, format string) (*pluginapi.Stream, pluginapi.SegmentedSource, error) {
			if format != "mp3" {
				return nil, nil, nil
			}
			st, err := backend.SongURL(ctx, tr.ID, "exhigh")
			if err != nil {
				return nil, nil, err
			}
			return &pluginapi.Stream{URL: st.URL, Type: st.Type, Level: st.Level, Size: st.Size}, nil, nil
		},
		Cover: func(ctx context.Context, tr *pluginapi.Track) ([]byte, error) {
			return backend.Fetch(ctx, backend.Sized(tr.Album.PicURL, 600))
		},
		Lyrics: func(ctx context.Context, tr *pluginapi.Track) (string, string, string, error) {
			return backend.Lyrics(ctx, tr.ID)
		},
		Started: func(tr *pluginapi.Track) {
			if a.home != nil {
				a.home.Started(tr)
			}
		},
	}
}

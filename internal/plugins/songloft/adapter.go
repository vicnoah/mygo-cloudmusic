package songloft

import (
	"context"

	"waygu-music/internal/pluginapi"
	backend "waygu-music/internal/plugins/songloft/internal/client"
)

const ID = backend.PlatformID

func Registration() pluginapi.Registration {
	return pluginapi.Registration{
		Info:  pluginapi.Info{ID: ID, Name: backend.DefaultProfile.Name, Icon: iconNote},
		Start: Start, LegacySettingsKey: string(ID),
	}
}

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
		if cfg.Address != "" && cfg.Username != "" && cfg.Password != "" {
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
	return &Adapter{
		state:   state,
		runtime: runtime,
		home:    NewHome(state, host, runtime),
	}
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

func (a *Adapter) ownsSource(src string) bool {
	return src == backend.Source || src == string(ID)
}

func (a *Adapter) OwnsTrack(tr *pluginapi.Track) bool {
	return tr != nil && a.ownsSource(tr.Source)
}

func (a *Adapter) PlaybackMedia() pluginapi.PlaybackMedia {
	s := a.state

	return pluginapi.PlaybackMedia{
		Stream: func(ctx context.Context, tr *pluginapi.Track, _ string) (*pluginapi.Stream, error) {
			provider := s.Provider()
			if provider == nil {
				return nil, backend.ErrNotConnected
			}
			return provider.Stream(ctx, tr)
		},
		Cover: func(ctx context.Context, tr *pluginapi.Track) ([]byte, error) {
			provider := s.Provider()
			if provider == nil {
				return nil, backend.ErrNotConnected
			}
			return provider.Cover(ctx, tr.Album.PicURL, 600)
		},
		Lyrics: func(ctx context.Context, tr *pluginapi.Track) (string, string, string, error) {
			provider := s.Provider()
			if provider == nil {
				return "", "", "", backend.ErrNotConnected
			}
			l, err := provider.Lyrics(ctx, tr)
			return l.LRC, l.Translation, l.YRC, err
		},
		Started: func(tr *pluginapi.Track) {
			if a.home != nil {
				a.home.RecordStarted(*tr)
			}
			provider := s.Provider()
			if provider == nil {
				return
			}
			cli := provider.Client()
			if cli != nil && tr.ID != 0 {
				go func() {
					_ = cli.ReportPlay(context.Background(), tr.ID)
				}()
			}
		},
	}
}

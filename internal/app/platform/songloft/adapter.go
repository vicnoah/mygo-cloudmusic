package songloft

import (
	"context"

	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform"
	backend "netease-music-mygo/internal/platform/songloft"
	"netease-music-mygo/internal/player"
)

type Adapter struct {
	state   *State
	runtime appplatform.Runtime
	home    *Home
}

var _ appplatform.Adapter = (*Adapter)(nil)

func NewAdapter(host Host, settings *appplatform.PlatformSettings, save ...func()) *Adapter {
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

func (a *Adapter) Home() appplatform.Home {
	if a == nil {
		return nil
	}
	return a.home
}

func (a *Adapter) Info() appplatform.Info {
	return appplatform.Info{
		ID:        platform.SongloftID,
		Name:      backend.DefaultProfile.Name,
		Comments:  false,
		Podcasts:  false,
		Quality:   false,
		Normalize: false,
	}
}

func (a *Adapter) Runtime() appplatform.Runtime {
	if a == nil {
		return nil
	}
	return a.runtime
}

func (a *Adapter) ownsSource(src string) bool {
	return src == backend.Source || src == string(platform.SongloftID)
}

func (a *Adapter) OwnsTrack(tr *netease.Track) bool {
	return tr != nil && a.ownsSource(tr.Source)
}

func (a *Adapter) Liked(tr *netease.Track) bool {
	return a != nil && a.state != nil && a.state.LikedTrack(tr)
}

func (a *Adapter) Favorite(ctx context.Context, tr *netease.Track, liked bool) error {
	if a == nil || a.state == nil {
		return backend.ErrNotConnected
	}
	return a.state.SetFavorite(ctx, tr, liked)
}

func (a *Adapter) ConfigurePlayer(p *player.Player) {
	if a == nil || a.state == nil || p == nil {
		return
	}
	s := a.state

	p.ResolveAudio = func(tr *netease.Track, level string) (*netease.Stream, player.AudioStream, error) {
		return nil, nil, nil
	}

	p.ResolveStream = func(tr *netease.Track, level string) (*netease.Stream, error) {
		if tr == nil {
			return nil, netease.ErrUnavailable
		}
		if !a.ownsSource(tr.Source) || s == nil {
			return netease.SongURL(tr.ID, level)
		}
		provider := s.Provider()
		if provider == nil {
			return nil, backend.ErrNotConnected
		}
		return provider.Stream(context.Background(), tr)
	}

	p.Artwork = func(tr *netease.Track) ([]byte, error) {
		if tr == nil {
			return nil, netease.ErrUnavailable
		}
		if !a.ownsSource(tr.Source) || s == nil {
			return netease.Fetch(netease.Sized(tr.Album.PicURL, 600))
		}
		provider := s.Provider()
		if provider == nil {
			return nil, backend.ErrNotConnected
		}
		return provider.Cover(context.Background(), tr.Album.PicURL, 600)
	}

	p.ResolveLyrics = func(tr *netease.Track) (string, string, string, error) {
		if tr == nil {
			return "", "", "", netease.ErrUnavailable
		}
		if !a.ownsSource(tr.Source) || s == nil {
			return netease.Lyrics(tr.ID)
		}
		provider := s.Provider()
		if provider == nil {
			return "", "", "", backend.ErrNotConnected
		}
		l, err := provider.Lyrics(context.Background(), tr)
		return l.LRC, l.Translation, l.YRC, err
	}

	p.Started = func(tr *netease.Track) {
		if tr == nil || !a.ownsSource(tr.Source) || s == nil {
			return
		}
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
	}
}

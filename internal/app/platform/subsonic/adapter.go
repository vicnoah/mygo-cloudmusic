package subsonic

import (
	"context"

	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform"
	backend "netease-music-mygo/internal/platform/subsonic"
	"netease-music-mygo/internal/player"
)

type Adapter struct {
	profile backend.Profile
	state   *State
	runtime appplatform.Runtime
	home    *Home
}

var _ appplatform.Adapter = (*Adapter)(nil)

func NewAdapter(host Host, profile backend.Profile, settings *appplatform.PlatformSettings, save ...func()) *Adapter {
	if profile.ID == "" {
		profile = backend.ProfileSubsonic
	}
	state := NewState(profile)
	if settings != nil {
		cfg := fromConnectionConfig(settings.Config)
		LoadCredentials(profile.ID, &cfg)
		state.SetConfig(cfg)
		if cfg.Valid() {
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
		profile: profile,
		state:   state,
		runtime: runtime,
		home:    NewHome(state, host, runtime),
	}
}

func (a *Adapter) Profile() backend.Profile {
	if a == nil {
		return backend.ProfileSubsonic
	}
	return a.profile
}

func (a *Adapter) Home() appplatform.Home {
	if a == nil {
		return nil
	}
	return a.home
}

func (a *Adapter) Info() appplatform.Info {
	id := platform.SubsonicID
	name := "Subsonic"
	if a != nil && a.profile.ID != "" {
		id = a.profile.ID
		name = a.profile.Name
	}
	return appplatform.Info{
		ID:        id,
		Name:      name,
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
	if src == backend.Source {
		return true
	}
	if a != nil && a.profile.ID != "" && src == string(a.profile.ID) {
		return true
	}
	return false
}

func (a *Adapter) OwnsTrack(tr *netease.Track) bool {
	return tr != nil && a.ownsSource(tr.Source)
}

func (a *Adapter) Liked(tr *netease.Track) bool {
	return a != nil && a.state != nil && a.state.LikedTrack(tr)
}

func (a *Adapter) Favorite(ctx context.Context, tr *netease.Track, liked bool) error {
	if a == nil || a.state == nil {
		return backend.ErrSignedOut
	}
	return a.state.SetFavorite(ctx, tr, liked)
}

func (a *Adapter) ConfigurePlayer(p *player.Player) {
	if a == nil || a.state == nil || p == nil {
		return
	}
	s := a.state

	p.ResolveAudio = func(tr *netease.Track, level string) (*netease.Stream, player.AudioStream, error) {
		// Nil decoded stream tells player to use standard HTTP streaming/caching decoder
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
			return nil, backend.ErrSignedOut
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
			return nil, backend.ErrSignedOut
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
			return "", "", "", backend.ErrSignedOut
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
		sc := provider.Client().Subsonic()
		if sc != nil && tr.Ref != "" {
			_ = sc.Scrobble(tr.Ref, map[string]string{"submission": "false"})
		}
	}
}

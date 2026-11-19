package subsonic

import (
	"context"

	"waygu-music/internal/pluginapi"
	backend "waygu-music/internal/plugins/subsonic/internal/client"
)

const (
	ID          = backend.PlatformID
	NavidromeID = backend.NavidromeID
	JellyfinID  = backend.JellyfinID
	GonicID     = backend.GonicID
)

type Adapter struct {
	profile backend.Profile
	state   *State
	runtime *RuntimeAdapter
	home    *Home
}

var _ pluginapi.Adapter = (*Adapter)(nil)

// Platforms registers the endpoints that share the Subsonic implementation.
func Platforms() []pluginapi.Registration {
	profiles := []backend.Profile{
		backend.ProfileNavidrome, backend.ProfileJellyfin, backend.ProfileGonic, backend.ProfileSubsonic,
	}
	entries := make([]pluginapi.Registration, 0, len(profiles))
	for _, profile := range profiles {
		entries = append(entries, pluginapi.Registration{
			Info:              pluginapi.Info{ID: profile.ID, Name: profile.Name, Icon: iconHeadset},
			LegacySettingsKey: string(profile.ID),
			Start: func(start pluginapi.Startup) pluginapi.Adapter {
				return startProfile(start, profile)
			},
		})
	}
	return entries
}

func NewAdapter(host Host, profile backend.Profile, settings *platformPreferences, save ...func()) *Adapter {
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
	state.SetTranscode(runtime.Transcode())
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
	if src == backend.Source {
		return true
	}
	if a != nil && a.profile.ID != "" && src == string(a.profile.ID) {
		return true
	}
	return false
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
				return nil, backend.ErrSignedOut
			}
			return provider.Stream(ctx, tr)
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
		},
	}
}

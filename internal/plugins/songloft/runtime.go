package songloft

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	appplatform "waygu-music/internal/pluginapi"
	backend "waygu-music/internal/plugins/songloft/internal/client"
)

type RuntimeAdapter struct {
	state      *State
	settingsMu sync.Mutex
	settings   *platformPreferences
	persist    func()
}

// forgetCredentials is replaced in tests so a test sign-out stays local.
var forgetCredentials = ClearCredentials

func NewRuntime(state *State, settings *platformPreferences, save ...func()) *RuntimeAdapter {
	var persist func()
	if len(save) > 0 {
		persist = save[0]
	}
	return &RuntimeAdapter{
		state:    state,
		settings: settings,
		persist:  persist,
	}
}

func (r *RuntimeAdapter) Config() connectionConfig {
	if r.state == nil {
		return connectionConfig{}
	}
	return toConnectionConfig(r.state.Config())
}

func (r *RuntimeAdapter) SetConfig(cfg connectionConfig) {
	if r.state == nil {
		return
	}
	bcfg := fromConnectionConfig(cfg)
	r.state.SetConfig(bcfg)
	r.settingsMu.Lock()
	defer r.settingsMu.Unlock()
	if r.settings != nil {
		r.settings.Config = cfg
		if r.persist != nil {
			r.persist()
		}
	}
}

func (r *RuntimeAdapter) Configured() bool {
	if r.state == nil {
		return false
	}
	cfg := r.state.Config()
	return cfg.Address != "" && cfg.Username != "" && cfg.Password != ""
}

func (r *RuntimeAdapter) Connected() bool {
	return r.state != nil && r.state.Connected()
}

func (r *RuntimeAdapter) Connecting() bool {
	return r.state != nil && r.state.Connecting()
}

func (r *RuntimeAdapter) Error() error {
	if r.state == nil {
		return nil
	}
	return r.state.Error()
}

func (r *RuntimeAdapter) Connect(ctx context.Context, cfg connectionConfig) error {
	if r.state == nil {
		return backend.ErrNotConnected
	}
	bcfg := fromConnectionConfig(cfg)
	if bcfg.Password == "" {
		LoadCredentials(&bcfg)
	}
	err := r.state.Connect(ctx, bcfg)
	if err == nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		_ = SaveCredentials(bcfg)
		r.SetConfig(toConnectionConfig(bcfg))
	}
	return err
}

func (r *RuntimeAdapter) Disconnect(ctx context.Context) error {
	if r.state == nil {
		return nil
	}
	err := r.state.Disconnect()
	_ = forgetCredentials()
	r.SetConfig(connectionConfig{})
	return err
}

func (r *RuntimeAdapter) Cancel(context.Context) error {
	return r.state.Disconnect()
}

func (r *RuntimeAdapter) FetchArtwork(ctx context.Context, raw string, size int) ([]byte, bool, error) {
	if r.state == nil {
		return nil, false, backend.ErrNotConnected
	}
	provider := r.state.Provider()
	if provider == nil {
		return nil, false, backend.ErrNotConnected
	}
	if !strings.HasPrefix(raw, backend.CoverScheme) &&
		!strings.HasPrefix(raw, backend.PlaylistCoverScheme) &&
		!strings.HasPrefix(raw, backend.FacetCoverScheme) {
		return nil, false, nil
	}
	data, err := provider.Cover(ctx, raw, size)
	return data, len(data) > 0, err
}

func (r *RuntimeAdapter) SetSort(key, value string) {
	if r.settings == nil {
		return
	}
	r.settingsMu.Lock()
	defer r.settingsMu.Unlock()
	if r.settings.Sorts == nil {
		r.settings.Sorts = make(map[string]string)
	}
	r.settings.Sorts[key] = value
	if r.persist != nil {
		r.persist()
	}
}

func toConnectionConfig(cfg backend.Config) connectionConfig {
	return connectionConfig{
		Address:  cfg.Address,
		Username: cfg.Username,
		PassHash: cfg.Password,
		DeviceID: cfg.DeviceID,
	}
}

func fromConnectionConfig(cfg connectionConfig) backend.Config {
	return backend.Config{
		Address:  cfg.Address,
		Username: cfg.Username,
		Password: cfg.PassHash,
		DeviceID: cfg.DeviceID,
	}
}

func (a *Adapter) connectOnActivation(event appplatform.Activation) {
	rt := a.runtime
	if !rt.Configured() || (!event.ColdStart && rt.Connected()) {
		return
	}
	cfg := rt.Config()
	go func() {
		if !event.ColdStart {
			_ = rt.Connect(event.Context, cfg)
			return
		}
		err := appplatform.Retry(event.Context, []time.Duration{0, 1500 * time.Millisecond, 3 * time.Second}, func(ctx context.Context) error {
			return rt.Connect(ctx, cfg)
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			a.home.host.Post(func() {
				if event.Context.Err() == nil {
					a.home.host.Toast(err.Error())
				}
			})
		}
	}()
}

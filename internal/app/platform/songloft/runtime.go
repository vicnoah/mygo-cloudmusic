package songloft

import (
	"context"
	"strings"

	appplatform "netease-music-mygo/internal/app/platform"
	backend "netease-music-mygo/internal/platform/songloft"
)

type RuntimeAdapter struct {
	state    *State
	settings *appplatform.PlatformSettings
	persist  func()
}

var _ appplatform.Runtime = (*RuntimeAdapter)(nil)

func NewRuntime(state *State, settings *appplatform.PlatformSettings, save ...func()) *RuntimeAdapter {
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

func (r *RuntimeAdapter) Config() appplatform.ConnectionConfig {
	if r.state == nil {
		return appplatform.ConnectionConfig{}
	}
	return toConnectionConfig(r.state.Config())
}

func (r *RuntimeAdapter) SetConfig(cfg appplatform.ConnectionConfig) {
	if r.state == nil {
		return
	}
	bcfg := fromConnectionConfig(cfg)
	r.state.SetConfig(bcfg)
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

func (r *RuntimeAdapter) Connect(ctx context.Context, cfg appplatform.ConnectionConfig) error {
	if r.state == nil {
		return backend.ErrNotConnected
	}
	bcfg := fromConnectionConfig(cfg)
	if bcfg.Password == "" {
		LoadCredentials(&bcfg)
	}
	err := r.state.Connect(ctx, bcfg)
	if err == nil {
		_ = SaveCredentials(bcfg)
		r.SetConfig(toConnectionConfig(bcfg))
	}
	return err
}

func (r *RuntimeAdapter) Disconnect(ctx context.Context) error {
	if r.state == nil {
		return nil
	}
	_ = ClearCredentials()
	err := r.state.Disconnect()
	if r.settings != nil {
		r.settings.Config = appplatform.ConnectionConfig{}
		if r.persist != nil {
			r.persist()
		}
	}
	return err
}

func (r *RuntimeAdapter) Cancel(ctx context.Context) error {
	return r.Disconnect(ctx)
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

func (r *RuntimeAdapter) Sorts() map[string]string {
	if r.settings == nil {
		return map[string]string{}
	}
	if r.settings.Sorts == nil {
		r.settings.Sorts = map[string]string{}
	}
	return r.settings.Sorts
}

func (r *RuntimeAdapter) SetSort(key, value string) {
	if r.settings == nil {
		return
	}
	if r.settings.Sorts == nil {
		r.settings.Sorts = make(map[string]string)
	}
	r.settings.Sorts[key] = value
	if r.persist != nil {
		r.persist()
	}
}

func (r *RuntimeAdapter) Transcode() appplatform.TranscodeSettings {
	return appplatform.TranscodeSettings{Mode: appplatform.TranscodeNative}
}

func (r *RuntimeAdapter) SetTranscode(ts appplatform.TranscodeSettings) {}

func (r *RuntimeAdapter) TranscodeBitrates() []int {
	return []int{0, 320, 256, 192, 128}
}

func (r *RuntimeAdapter) AccountActions() []appplatform.AccountAction {
	return nil
}

func (r *RuntimeAdapter) OpenAccount() {}

func toConnectionConfig(cfg backend.Config) appplatform.ConnectionConfig {
	return appplatform.ConnectionConfig{
		Address:  cfg.Address,
		Username: cfg.Username,
		PassHash: cfg.Password,
		DeviceID: cfg.DeviceID,
	}
}

func fromConnectionConfig(cfg appplatform.ConnectionConfig) backend.Config {
	return backend.Config{
		Address:  cfg.Address,
		Username: cfg.Username,
		Password: cfg.PassHash,
		DeviceID: cfg.DeviceID,
	}
}

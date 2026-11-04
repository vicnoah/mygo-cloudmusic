package subsonic

import (
	"context"

	appplatform "netease-music-mygo/internal/app/platform"
	backend "netease-music-mygo/internal/platform/subsonic"
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
	cfg := r.state.Config()
	return toConnectionConfig(cfg)
}

func (r *RuntimeAdapter) SetConfig(cc appplatform.ConnectionConfig) {
	if r.state == nil {
		return
	}
	cfg := fromConnectionConfig(cc)
	r.state.SetConfig(cfg)
	if r.settings != nil {
		r.settings.Config = cc
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
	return cfg.Valid()
}

func (r *RuntimeAdapter) Connected() bool {
	return r.state != nil && r.state.Connected()
}

func (r *RuntimeAdapter) Error() error {
	if r.state == nil {
		return nil
	}
	return r.state.Error()
}

func (r *RuntimeAdapter) Connect(ctx context.Context, cc appplatform.ConnectionConfig) error {
	if r.state == nil {
		return backend.ErrSignedOut
	}
	pid := r.state.Profile().ID
	cfg := fromConnectionConfig(cc)
	LoadCredentials(pid, &cfg)
	if cc.PassHash != "" {
		cfg.Password = cc.PassHash
	}
	err := r.state.Connect(ctx, cfg)
	if err == nil {
		_ = SaveCredentials(pid, cfg)
		if r.settings != nil {
			r.settings.Config = toConnectionConfig(cfg)
			if r.persist != nil {
				r.persist()
			}
		}
	}
	return err
}

func (r *RuntimeAdapter) Cancel(ctx context.Context) error {
	if r.state == nil {
		return nil
	}
	return r.state.Cancel(ctx)
}

func (r *RuntimeAdapter) Disconnect(ctx context.Context) error {
	if r.state == nil {
		return nil
	}
	ClearCredentials(r.state.Profile().ID)
	err := r.state.Disconnect(ctx)
	if r.settings != nil {
		r.settings.Config.PassHash = ""
		if r.persist != nil {
			r.persist()
		}
	}
	return err
}

func (r *RuntimeAdapter) FetchArtwork(ctx context.Context, raw string, size int) ([]byte, bool, error) {
	if r.state == nil {
		return nil, false, backend.ErrSignedOut
	}
	provider := r.state.Provider()
	if provider == nil {
		return nil, false, backend.ErrSignedOut
	}
	data, err := provider.Cover(ctx, raw, size)
	return data, len(data) > 0, err
}

func (r *RuntimeAdapter) Transcode() appplatform.TranscodeSettings {
	if r.settings == nil {
		return appplatform.TranscodeSettings{Mode: appplatform.TranscodeAuto}
	}
	mode := r.settings.TranscodeMode
	if mode != appplatform.TranscodeAuto && mode != appplatform.TranscodeNative && mode != appplatform.TranscodeAlways {
		mode = appplatform.TranscodeAuto
	}
	return appplatform.TranscodeSettings{Mode: mode, Bitrate: r.settings.TranscodeBitrate}
}

func (r *RuntimeAdapter) SetTranscode(ts appplatform.TranscodeSettings) {
	if r.settings == nil {
		return
	}
	mode := ts.Mode
	if mode != appplatform.TranscodeAuto && mode != appplatform.TranscodeNative && mode != appplatform.TranscodeAlways {
		mode = appplatform.TranscodeAuto
	}
	r.settings.TranscodeMode = mode
	r.settings.TranscodeBitrate = ts.Bitrate
	if r.state != nil {
		r.state.SetTranscode(ts)
	}
}

func (r *RuntimeAdapter) TranscodeBitrates() []int {
	return []int{0, 320, 256, 192, 128}
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
		r.settings.Sorts = map[string]string{}
	}
	r.settings.Sorts[key] = value
	if r.persist != nil {
		r.persist()
	}
}

func toConnectionConfig(cfg backend.Config) appplatform.ConnectionConfig {
	return appplatform.ConnectionConfig{
		Address:  cfg.Address,
		Username: cfg.Username,
		PassHash: cfg.Password,
		HTTPS:    cfg.Insecure,
	}
}

func fromConnectionConfig(cc appplatform.ConnectionConfig) backend.Config {
	return backend.Config{
		Address:  cc.Address,
		Username: cc.Username,
		Password: cc.PassHash,
		Insecure: cc.HTTPS,
	}
}

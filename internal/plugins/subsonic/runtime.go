package subsonic

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	appplatform "waygu-music/internal/pluginapi"
	backend "waygu-music/internal/plugins/subsonic/internal/client"
)

type RuntimeAdapter struct {
	state      *State
	settingsMu sync.Mutex
	settings   *platformPreferences
	persist    func()
}

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
	cfg := r.state.Config()
	return toConnectionConfig(cfg)
}

func (r *RuntimeAdapter) SetConfig(cc connectionConfig) {
	if r.state == nil {
		return
	}
	cfg := fromConnectionConfig(cc)
	r.state.SetConfig(cfg)
	r.saveConfig(cc)
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

func (r *RuntimeAdapter) Connect(ctx context.Context, cc connectionConfig) error {
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
		if err := ctx.Err(); err != nil {
			return err
		}
		_ = SaveCredentials(pid, cfg)
		r.saveConfig(toConnectionConfig(cfg))
	}
	return err
}

func (r *RuntimeAdapter) saveConfig(cfg connectionConfig) {
	r.settingsMu.Lock()
	defer r.settingsMu.Unlock()
	if r.settings != nil {
		r.settings.Config = cfg
		if r.persist != nil {
			r.persist()
		}
	}
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
	r.settingsMu.Lock()
	defer r.settingsMu.Unlock()
	if r.settings != nil {
		r.settings.Config.PassHash = ""
		if r.persist != nil {
			r.persist()
		}
	}
	return err
}

func (r *RuntimeAdapter) FetchArtwork(ctx context.Context, raw string, size int) ([]byte, bool, error) {
	if !strings.HasPrefix(raw, backend.CoverScheme) {
		return nil, false, nil
	}
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

func (r *RuntimeAdapter) Transcode() transcodeSettings {
	if r.settings == nil {
		return transcodeSettings{Mode: transcodeAuto}
	}
	r.settingsMu.Lock()
	defer r.settingsMu.Unlock()
	mode := r.settings.TranscodeMode
	if mode != transcodeAuto && mode != transcodeNative && mode != transcodeAlways {
		mode = transcodeAuto
	}
	return transcodeSettings{Mode: mode, Bitrate: r.settings.TranscodeBitrate}
}

func (r *RuntimeAdapter) SetTranscode(ts transcodeSettings) {
	if r.settings == nil {
		return
	}
	mode := ts.Mode
	if mode != transcodeAuto && mode != transcodeNative && mode != transcodeAlways {
		mode = transcodeAuto
	}
	r.settingsMu.Lock()
	defer r.settingsMu.Unlock()
	r.settings.TranscodeMode = mode
	r.settings.TranscodeBitrate = ts.Bitrate
	if r.state != nil {
		r.state.SetTranscode(transcodeSettings{Mode: mode, Bitrate: ts.Bitrate})
	}
	if r.persist != nil {
		r.persist()
	}
}

func (r *RuntimeAdapter) TranscodeBitrates() []int {
	return []int{0, 320, 256, 192, 128}
}

func (r *RuntimeAdapter) SetSort(key, value string) {
	if r.settings == nil {
		return
	}
	r.settingsMu.Lock()
	defer r.settingsMu.Unlock()
	if r.settings.Sorts == nil {
		r.settings.Sorts = map[string]string{}
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
		HTTPS:    cfg.Insecure,
	}
}

func fromConnectionConfig(cc connectionConfig) backend.Config {
	return backend.Config{
		Address:  cc.Address,
		Username: cc.Username,
		Password: cc.PassHash,
		Insecure: cc.HTTPS,
	}
}

func (a *Adapter) connectOnActivation(event appplatform.Activation) {
	rt := a.runtime
	if !rt.Configured() {
		return
	}
	if !event.ColdStart && rt.Connected() {
		go a.state.loadInitialLibrary(event.Context)
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

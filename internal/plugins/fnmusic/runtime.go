package fnmusic

import (
	"context"
	"errors"
	"maps"
	"sync"
	"time"

	appplatform "waygu-music/internal/pluginapi"
	backend "waygu-music/internal/plugins/fnmusic/internal/client"
)

// RuntimeAdapter translates the generic application seam into Feiniu's
// protocol configuration and lifecycle. No Feiniu client type escapes this
// package.
type RuntimeAdapter struct {
	state      *State
	settingsMu sync.Mutex
	settings   *platformPreferences
	save       func()
}

func NewRuntime(state *State, settings *platformPreferences, save ...func()) *RuntimeAdapter {
	var persist func()
	if len(save) > 0 {
		persist = save[0]
	}
	return &RuntimeAdapter{state: state, settings: settings, save: persist}
}

func (r *RuntimeAdapter) Config() connectionConfig {
	if r == nil || r.state == nil {
		return connectionConfig{}
	}
	return toConnectionConfig(r.state.Config())
}

func (r *RuntimeAdapter) SetConfig(cfg connectionConfig) {
	if r == nil || r.state == nil {
		return
	}
	r.state.SetConfig(fromConnectionConfig(cfg))
}

func (r *RuntimeAdapter) Configured() bool {
	if r == nil {
		return false
	}
	return fromConnectionConfig(r.Config()).Valid()
}

func (r *RuntimeAdapter) Connected() bool { return r != nil && r.state != nil && r.state.Connected() }

func (r *RuntimeAdapter) Error() error {
	if r == nil || r.state == nil {
		return nil
	}
	return r.state.Error()
}

func (r *RuntimeAdapter) Connect(ctx context.Context, cfg connectionConfig) error {
	if r == nil || r.state == nil {
		return backend.ErrSignedOut
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	gen := r.state.beginConnect()
	defer r.state.setConnecting(gen, false)
	if err := r.state.connect(ctx, fromConnectionConfig(cfg), gen); err != nil {
		return err
	}
	if err := r.rememberConnection(ctx, gen); err != nil {
		return err
	}
	_, _ = r.state.LoadPlaylists(ctx)
	return nil
}

func (r *RuntimeAdapter) rememberConnection(ctx context.Context, gen uint64) error {
	r.settingsMu.Lock()
	defer r.settingsMu.Unlock()
	client, current, err := r.state.activeClient(ctx)
	if err != nil {
		return err
	}
	if current != gen {
		return context.Canceled
	}
	cfg := client.Config()
	_ = SaveCredentials(cfg)
	r.saveConfigLocked(toConnectionConfig(cfg))
	return nil
}

func (r *RuntimeAdapter) Cancel(context.Context) error {
	if r != nil && r.state != nil {
		r.state.Cancel()
	}
	return nil
}

func (r *RuntimeAdapter) Disconnect(ctx context.Context) error {
	if r == nil || r.state == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Clear the local account before a logout that can outlive this connection.
	r.settingsMu.Lock()
	client := r.state.Detach()
	DeleteCredentials()
	r.saveConfigLocked(connectionConfig{})
	r.settingsMu.Unlock()
	if client != nil {
		client.WithContext(ctx).Logout()
	}
	return nil
}

func (r *RuntimeAdapter) saveConfigLocked(cfg connectionConfig) {
	if r.settings != nil {
		r.settings.Config = cfg
	}
	if r.save != nil {
		r.save()
	}
}

func (r *RuntimeAdapter) FetchArtwork(ctx context.Context, raw string, size int) ([]byte, bool, error) {
	if r == nil || r.state == nil {
		return nil, false, nil
	}
	return r.state.FetchArtwork(ctx, raw, size)
}

func (r *RuntimeAdapter) Transcode() transcodeSettings {
	if r == nil || r.settings == nil {
		return transcodeSettings{Mode: transcodeAuto}
	}
	r.settingsMu.Lock()
	defer r.settingsMu.Unlock()
	return transcodeSettings{Mode: r.settings.TranscodeMode, Bitrate: r.settings.TranscodeBitrate}
}

func (r *RuntimeAdapter) SetTranscode(v transcodeSettings) {
	if r == nil || r.settings == nil {
		return
	}
	mode := v.Mode
	if mode != transcodeAuto && mode != transcodeNative && mode != transcodeAlways {
		mode = transcodeAuto
	}
	b := v.Bitrate
	if !backend.ValidTranscodeBitrate(b) {
		b = 0
	}
	r.settingsMu.Lock()
	defer r.settingsMu.Unlock()
	r.settings.TranscodeMode, r.settings.TranscodeBitrate = mode, b
	if r.save != nil {
		r.save()
	}
}

func (r *RuntimeAdapter) TranscodeBitrates() []int {
	return append([]int(nil), backend.TranscodeBitrates...)
}

func (r *RuntimeAdapter) Sorts() map[string]string {
	if r == nil || r.settings == nil {
		return nil
	}
	r.settingsMu.Lock()
	defer r.settingsMu.Unlock()
	return maps.Clone(r.settings.Sorts)
}

func (r *RuntimeAdapter) SetSort(key, value string) {
	if r == nil || r.settings == nil || key == "" {
		return
	}
	r.settingsMu.Lock()
	defer r.settingsMu.Unlock()
	if r.settings.Sorts == nil {
		r.settings.Sorts = map[string]string{}
	}
	if r.settings.Sorts[key] == value {
		return
	}
	r.settings.Sorts[key] = value
	if r.save != nil {
		r.save()
	}
}

func toConnectionConfig(c backend.Config) connectionConfig {
	return connectionConfig{Address: c.Address, Username: c.Username, PassHash: c.PassHash, AccessCode: c.AccessCode, DeviceID: c.DeviceID, Mode: c.Mode, HTTPS: c.HTTPS}
}

func fromConnectionConfig(c connectionConfig) backend.Config {
	return backend.Config{Address: c.Address, Username: c.Username, PassHash: c.PassHash, AccessCode: c.AccessCode, DeviceID: c.DeviceID, Mode: c.Mode, HTTPS: c.HTTPS}
}

func (a *Adapter) connectOnActivation(event appplatform.Activation) {
	rt := a.runtime
	if !rt.Configured() {
		return
	}
	if !event.ColdStart && rt.Connected() {
		if !a.state.playlistsLoaded() {
			go func() {
				_, _ = a.state.LoadPlaylists(event.Context)
				a.home.host.Post(func() {
					if event.Context.Err() == nil {
						a.home.host.Invalidate()
					}
				})
			}()
		}
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

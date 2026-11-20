package app

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/waygu-music-plugin/sdk/bridge"
	"github.com/waygu-music-plugin/sdk/platformrpc"
	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/pluginapi/network"
	"github.com/waygu-music-plugin/sdk/protocol"
)

var hostOffer = platformrpc.Offer{
	Version: protocol.Version,
	Provides: []string{
		platformrpc.CapabilityPlayback, platformrpc.CapabilitySettings, platformrpc.CapabilityMediaStreams,
		protocol.CapabilityPlaybackEvents, protocol.CapabilityDesktopLyrics, protocol.CapabilityDesktopTasks,
		protocol.CapabilityDesktopPatches,
		protocol.CapabilityMediaRenewal, protocol.CapabilityPluginEntries,
	},
	Needs: []string{platformrpc.CapabilityWindow, platformrpc.CapabilityMedia, platformrpc.CapabilityMediaStreams},
}

type localPlugin struct {
	entry      pluginapi.Registration
	program    platformrpc.Program
	pending    context.CancelFunc
	request    uint64
	adapter    *processAdapter
	startError string
}

type processAdapter struct {
	id     pluginapi.ID
	active atomic.Pointer[pluginProcess]
}

type pluginProcess struct {
	transport *platformrpc.Process
	owner     pluginapi.ID
	adapter   *processAdapter
	scope     platformrpc.Scope
	info      PluginPrepared
	pcm       chan struct{}
	media     bridge.MediaSlots
	visible   bool

	// The event subscription and sequence belong to the host UI thread.
	events        *pluginEventStream
	eventSequence uint64
}

// RunManaged uses locally installed releases with Feiniu built in.
func RunManaged(registrations []pluginapi.Registration) error {
	return run(registrations, true)
}

// pluginCredentialService keeps the installed host's historical mygo.json
// identifier when the plugin binary has its own name or package metadata.
func pluginCredentialService() *string {
	service := ""
	if !mygo.IsDev() {
		service = "dev.egoist.cloudmusic"
	}
	return &service
}

func (a *app) startLocalPlugin(id pluginapi.ID) {
	local := a.localPlugins[id]
	if local == nil || a.platformID == id || local.pending != nil {
		return
	}
	if local.program.Path == "" {
		a.catalogPackage = local.program.Identity.Package
		a.openPluginManager()
		return
	}
	if pkg := a.managedPlugins[local.program.Identity.Package]; pkg != nil && pkg.busy {
		return
	}
	a.cancelLocalRequest()
	starting := a.platformID == ""
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		a.showPluginError(local, err)
		return
	}
	program := local.program
	program.StateDir = filepath.Join(dir, "plugin-ui", url.PathEscape(string(local.entry.ID)))
	program.LegacyCacheDir, _ = mygo.App.Path(mygo.PathCache)
	program.CredentialService = pluginCredentialService()
	program.ApplicationID = desktopApplicationID()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	local.pending = cancel
	local.request++
	request := local.request
	store := pluginSettingsStore{a: a, entry: local.entry}
	preferences, err := store.raw()
	if err != nil {
		local.pending = nil
		cancel()
		a.showPluginError(local, err)
		return
	}
	local.startError = ""
	if starting {
		a.pluginStartup.failure = ""
		a.scheduleStartupNotice(local, request)
	}
	a.redraw()
	input := PluginPrepare{Settings: preferences, Tray: a.tray != nil || runtime.GOOS == "darwin",
		Appearance: a.settings.Appearance, WindowIcon: desktopApplicationIcon()}
	a.pluginWorkers.Go(func() {
		defer cancel()
		defer mygo.RunOnMain(func() {
			if local.request == request {
				local.pending = nil
				if !a.quitting {
					a.redraw()
				}
			}
		})
		process, err := platformrpc.Start(ctx, program, hostOffer, func(connection string) any {
			return pluginReverse{app: a, connection: connection}
		})
		if err != nil {
			mygo.RunOnMain(func() {
				if local.request == request && !a.quitting {
					a.showPluginError(local, err)
				}
			})
			return
		}
		candidate := &pluginProcess{transport: process, owner: id, adapter: local.adapter,
			pcm: make(chan struct{}, 4), media: bridge.NewMediaSlots()}
		committed := false
		defer func() {
			if !committed {
				process.Close()
			}
		}()
		mygo.RunOnMain(func() {
			if local.request != request || a.quitting {
				return
			}
			candidate.scope = a.pluginActivations.Prepare(process.Connection())
			input.Scope = candidate.scope
			snapshot := capturePluginSnapshot(a, nil)
			input.Facade = PluginSnapshot{
				At: -1, Volume: snapshot.Volume, Presentation: snapshot.Presentation,
				Desktop: snapshot.Desktop, Decoder: snapshot.Decoder, Casting: snapshot.Casting,
				EQ: snapshot.EQ, EQAvailable: snapshot.EQAvailable, Search: snapshot.Search,
			}
		})
		if candidate.scope.Activation == 0 {
			return
		}
		err = process.Call(ctx, "Prepare", input.Wire(), &candidate.info)
		if err == nil {
			err = candidate.info.Failure.Err()
		}
		if err == nil {
			err = bridge.ValidateWindowActions(candidate.info)
		}
		activate := PluginAction{Action: "activate"}
		mygo.RunOnMain(func() {
			if err == nil {
				err = ctx.Err()
			}
			if err != nil || local.request != request || a.quitting {
				a.pluginActivations.Revoke(candidate.scope)
				if err != nil && local.request == request && !a.quitting {
					a.showPluginError(local, err)
				}
				return
			}
			if !a.pluginActivations.Commit(candidate.scope) {
				return
			}
			candidate.visible = true
			local.adapter.active.Store(candidate)
			if pkg := a.managedPlugins[local.program.Identity.Package]; pkg != nil {
				if a.settings.LastPluginEntries == nil {
					a.settings.LastPluginEntries = make(map[string]string)
				}
				a.settings.LastPluginEntries[pkg.id] = local.program.Identity.Entry
			}
			a.commitPlatform(local.entry.ID, true)
			if candidate.supportsDesktopLyrics() {
				activate.DesktopLyrics = bridge.WireDesktopLyricsPrefs(a.desktopLyricsPrefs())
			}
			a.win.Hide()
			a.syncDock()
			local.pending = nil
			committed = true
		})
		if committed {
			go a.watchLocalPlugin(candidate, activate, starting)
		}
	})
}

func (a *app) watchLocalPlugin(candidate *pluginProcess, activate PluginAction, starting bool) {
	if _, err := candidate.windowAction(activate); err != nil {
		mygo.RunOnMain(func() {
			if starting && !a.quitting && candidate == a.remoteActive() {
				a.commitPlatform("", false)
				a.failPluginStartup(candidate.owner, err)
				return
			}
			a.processLost(candidate, err)
		})
		return
	}
	<-candidate.transport.Done()
	mygo.RunOnMain(func() { a.processLost(candidate, nil) })
}

func (a *app) cancelLocalRequest() {
	a.dismissStartupNotice()
	for _, local := range a.localPlugins {
		if local.pending != nil {
			local.request++
			local.pending()
			local.pending = nil
		}
	}
}

func (a *app) remoteActive() *pluginProcess {
	return a.activeRemote.Load()
}

func (a *app) revokeLocalPlugin() {
	if active := a.activeRemote.Swap(nil); active != nil {
		a.pluginActivations.Revoke(active.scope)
		active.adapter.active.Store(nil)
		a.pluginWorkers.Go(active.transport.Close)
	}
}

func (a *app) processLost(process *pluginProcess, err error) {
	if a.quitting || process != a.remoteActive() {
		return
	}
	local := a.localPlugins[a.platformID]
	a.cancelLocalRequest()
	a.commitPlatform(a.defaultPlatform(), false)
	if err != nil {
		a.showPluginError(local, err)
	}
}

func (a *app) showPluginError(local *localPlugin, err error) {
	local.startError = "插件启动失败：" + err.Error()
	a.catalogPackage = local.program.Identity.Package
	if a.platformID == "" {
		a.failPluginStartup(local.entry.ID, err)
	} else if a.pluginsWin != nil {
		a.pluginsWin.Invalidate()
	} else if a.remoteActive() != nil {
		a.openPluginManager()
	} else {
		(platformHost{a: a}).Toast(local.startError)
	}
}

func (p *pluginProcess) action(action string) (PluginActionReply, error) {
	return p.windowAction(PluginAction{Action: action})
}

func (p *pluginProcess) supportsDesktopLyrics() bool {
	return p.info.SupportsWindowAction(protocol.WindowToggleDesktopLyrics) &&
		p.info.SupportsWindowAction(protocol.WindowToggleDesktopLyricsLock)
}

func (p *pluginProcess) windowAction(in PluginAction) (PluginActionReply, error) {
	if !p.info.SupportsWindowAction(in.Action) {
		return PluginActionReply{}, platformrpc.Failure{Code: "unsupported-window-action"}
	}
	ctx, cancel := pluginRPCTimeout()
	defer cancel()
	var reply PluginActionReply
	in.Scope = p.scope
	err := p.transport.Call(ctx, "Window", in, &reply)
	if err == nil {
		err = reply.Failure.Err()
	}
	return reply, err
}

func (a *processAdapter) Configured() bool {
	p := a.active.Load()
	return p != nil && p.info.Configured
}

func (a *processAdapter) OwnsTrack(track *pluginapi.Track) bool {
	if track == nil {
		return false
	}
	switch a.id {
	case "netease":
		return track.Source == ""
	case "fnmusic":
		return track.Source == "fn" || track.Source == string(a.id)
	default:
		return track.Source == string(a.id)
	}
}

func (a *processAdapter) call(ctx context.Context, in PluginCall) (PluginMedia, error) {
	p := a.active.Load()
	if p == nil {
		return PluginMedia{}, pluginapi.ErrUnavailable
	}
	return p.call(ctx, in)
}

func (p *pluginProcess) call(ctx context.Context, in PluginCall) (PluginMedia, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	in.Scope = p.scope
	deadline, _ := ctx.Deadline()
	in.DeadlineNano = deadline.UnixNano()
	return p.callMediaStream(ctx, in)
}

func (a *processAdapter) FetchArtwork(ctx context.Context, ref string, size int) ([]byte, bool, error) {
	reply, err := a.call(ctx, PluginCall{Action: "artwork", Ref: ref, Size: int32(size)})
	return reply.Cover, reply.Handled, err
}

func (a *processAdapter) PlaybackMedia() pluginapi.PlaybackMedia {
	return pluginapi.PlaybackMedia{
		Preferences: func() pluginapi.AudioPreferences {
			if p := a.active.Load(); p != nil {
				return pluginapi.AudioPreferences(p.info.Preferences)
			}
			return pluginapi.AudioPreferences{}
		},
		Stream: func(ctx context.Context, track *pluginapi.Track, level string) (*pluginapi.Stream, error) {
			return a.resolveMediaSource(ctx, PluginCall{Action: "stream", Track: clonePluginTrack(*track), Level: level})
		},
		CastSource: func(ctx context.Context, track *pluginapi.Track, format string) (*pluginapi.Stream, pluginapi.SegmentedSource, error) {
			stream, err := a.resolveMediaSource(ctx, PluginCall{Action: "cast", Track: clonePluginTrack(*track), Format: format})
			return stream, nil, err
		},
		Cover: func(ctx context.Context, track *pluginapi.Track) ([]byte, error) {
			r, err := a.call(ctx, PluginCall{Action: "cover", Track: *track})
			return r.Cover, err
		},
		Lyrics: func(ctx context.Context, track *pluginapi.Track) (string, string, string, error) {
			r, err := a.call(ctx, PluginCall{Action: "lyrics", Track: *track})
			return r.Original, r.Trans, r.Word, err
		},
		ErrorMessage: func(_ *pluginapi.Track, err error) string {
			var failure platformrpc.Failure
			if errors.As(err, &failure) && failure.Code == protocol.FailureMediaMessage {
				return failure.Message
			}
			return ""
		},
		Started: func(track *pluginapi.Track) {
			if p := a.active.Load(); p != nil {
				in := PluginCall{Action: "started", Track: clonePluginTrack(*track)}
				go func() { _, _ = p.call(context.Background(), in) }()
			}
		},
	}
}

func (a *processAdapter) resolveMediaSource(ctx context.Context, request PluginCall) (*pluginapi.Stream, error) {
	process := a.active.Load()
	if process == nil {
		return nil, pluginapi.ErrUnavailable
	}
	reply, err := process.call(ctx, request)
	if err != nil || reply.Stream == nil {
		return nil, err
	}
	source := reply.Stream
	stream := source.Stream()
	if source.Renewable && process.transport.Provides(protocol.CapabilityMediaRenewal) {
		stream.Renew = func(ctx context.Context, rejected network.MediaEndpoint) (network.MediaEndpoint, error) {
			if a.active.Load() != process {
				return network.MediaEndpoint{}, pluginapi.ErrUnavailable
			}
			renewal := request
			renewal.PreviousURL = rejected.URL
			next, err := process.call(ctx, renewal)
			if err != nil {
				return network.MediaEndpoint{}, err
			}
			if a.active.Load() != process {
				return network.MediaEndpoint{}, pluginapi.ErrUnavailable
			}
			if !source.SameMedia(next.Stream) {
				return network.MediaEndpoint{}, platformrpc.Failure{Code: "media-source-changed"}
			}
			return network.MediaEndpoint{URL: next.Stream.URL, Header: http.Header(next.Stream.Headers)}, nil
		}
	}
	return stream, nil
}

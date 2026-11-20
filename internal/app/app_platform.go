package app

import (
	"context"

	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/pluginapi/network"
)

// Platform extension seam.
//
// Keep provider-neutral adapter lookup, capability checks and dispatch here.
// Concrete provider state, protocol types and provider UI belong in the
// provider adapter directory. Changes to the upstream app should use this
// seam instead of adding provider-specific fields or branches.

func (a *app) activePlatform() pluginapi.Adapter {
	if a == nil || a.platforms == nil {
		return nil
	}
	if remote := a.remoteActive(); remote != nil {
		return remote.adapter
	}
	return a.platforms[a.platformID]
}

func (a *app) initPlatformState() bool {
	newPlatformAdapters(a)
	id := pluginapi.ID(a.settings.Platform)
	if !a.hasPlatform(id) || a.localPlugins[id] != nil {
		id = a.defaultPlatform()
	}
	a.platformID = id
	if id == "" {
		return false
	}
	if a.settings.Platform == "" {
		a.settings.Platform = string(id)
	}
	return true
}

func (a *app) defaultPlatform() pluginapi.ID {
	for _, entry := range a.platformRegistrations {
		if a.localPlugins[entry.ID] == nil && a.hasPlatform(entry.ID) {
			return entry.ID
		}
	}
	return ""
}

func (a *app) configurePlatformPlayer() {
	if a.art != nil {
		a.art.SetFetcher(func() func(string) ([]byte, error) {
			owner, ctx := a.platformID, a.platformContext
			if ctx == nil {
				ctx = context.Background()
			}
			return func(url string) ([]byte, error) {
				return (platformHost{a: a}).fetchArtwork(ctx, owner, url)
			}
		}, func() uint64 { return a.platformGeneration })
	}
	if a.pl == nil {
		return
	}
	a.pl.Quality = func() string { return a.playbackPreferences(nil).Quality }
	a.pl.Normalize = func() bool { return a.playbackPreferences(nil).Normalize }
	a.pl.TrackPreferences = a.playbackPreferences
	a.pl.TrackPlatform = func(track *pluginapi.Track) pluginapi.ID {
		id, _ := a.trackPlatform(track)
		return id
	}
	a.pl.ArtworkURL = func(tr *pluginapi.Track) string {
		return network.SizedArtworkURL(tr.Album.PicURL, 600)
	}
	a.pl.ResolveStream = func(ctx context.Context, tr *pluginapi.Track, level string) (*pluginapi.Stream, error) {
		if load := a.playbackMedia(tr).Stream; load != nil {
			return load(ctx, tr, level)
		}
		return nil, pluginapi.ErrUnavailable
	}
	a.pl.Artwork = func(ctx context.Context, tr *pluginapi.Track) ([]byte, error) {
		if load := a.playbackMedia(tr).Cover; load != nil {
			return load(ctx, tr)
		}
		return nil, pluginapi.ErrUnavailable
	}
	a.pl.ResolveLyrics = func(ctx context.Context, tr *pluginapi.Track) (string, string, string, error) {
		if load := a.playbackMedia(tr).Lyrics; load != nil {
			return load(ctx, tr)
		}
		return "", "", "", pluginapi.ErrUnavailable
	}
	a.pl.ResolveSegments = func(ctx context.Context, tr *pluginapi.Track, advanced bool) (*pluginapi.Stream, pluginapi.SegmentedSource, error) {
		if load := a.playbackMedia(tr).Segments; load != nil {
			return load(ctx, tr, advanced)
		}
		return nil, nil, nil
	}
	a.pl.ResolveCastSource = func(ctx context.Context, tr *pluginapi.Track, format string) (*pluginapi.Stream, pluginapi.SegmentedSource, error) {
		if load := a.playbackMedia(tr).CastSource; load != nil {
			return load(ctx, tr, format)
		}
		return nil, nil, nil
	}
	a.pl.Started = func(tr *pluginapi.Track) {
		if report := a.playbackMedia(tr).Started; report != nil {
			report(tr)
		}
	}
	a.pl.StreamError = func(tr *pluginapi.Track, err error) string {
		if describe := a.playbackMedia(tr).ErrorMessage; describe != nil {
			return describe(tr, err)
		}
		return ""
	}
}

func (a *app) playbackPreferences(track *pluginapi.Track) pluginapi.AudioPreferences {
	adapter := a.activePlatform()
	if track != nil {
		adapter = a.trackAdapter(track)
	}
	if adapter != nil {
		if prefs := adapter.PlaybackMedia().Preferences; prefs != nil {
			return prefs()
		}
	}
	return pluginapi.AudioPreferences{Quality: a.settings.Quality, Normalize: a.settings.Normalize}
}

func (a *app) playbackMedia(tr *pluginapi.Track) pluginapi.PlaybackMedia {
	if tr == nil {
		return pluginapi.PlaybackMedia{}
	}
	if adapter := a.trackAdapter(tr); adapter != nil {
		return adapter.PlaybackMedia()
	}
	return pluginapi.PlaybackMedia{}
}

func (a *app) platformReady(coldStart bool) {
	if ready := a.platformLifecycles[a.platformID].Ready; ready != nil {
		ready(pluginapi.Activation{Context: a.platformContext, Main: a.win, ColdStart: coldStart})
	}
}

func (a *app) activatePlatformView() {
	if local := a.localPlugins[a.platformID]; local != nil {
		a.activeRemote.Store(local.adapter.active.Load())
	}
	if remote := a.remoteActive(); remote != nil {
		a.platformScope = remote.scope
	} else {
		a.platformScope = a.pluginActivations.Prepare("builtin:" + string(a.platformID))
		a.pluginActivations.Commit(a.platformScope)
	}
	a.platformContext, a.cancelPlatform = context.WithCancel(context.Background())
	a.platformAuthority.Store(&platformAuthority{id: a.platformID, ctx: a.platformContext})
	if activate := a.platformLifecycles[a.platformID].Activate; activate != nil {
		activate(a.platformContext, playbackContinuity{a: a, ctx: a.platformContext})
	}
}

func (a *app) deactivatePlatformView(switching bool) {
	a.platformAuthority.Store(nil)
	a.pluginActivations.Revoke(a.platformScope)
	if a.cancelPlatform != nil {
		a.cancelPlatform()
	}
	if deactivate := a.platformLifecycles[a.platformID].Deactivate; deactivate != nil {
		deactivate(switching)
	}
}

func (a *app) hasPlatform(id pluginapi.ID) bool {
	return id != "" && a.platforms[id] != nil && a.platformWindows[id][pluginapi.MainWindow].Root != nil
}

func (a *app) trackPlatform(tr *pluginapi.Track) (pluginapi.ID, bool) {
	if tr == nil {
		return "", false
	}
	if remote := a.remoteActive(); remote != nil && remote.adapter.OwnsTrack(tr) {
		return remote.owner, true
	}
	// Sources that are also instance IDs must win over legacy source aliases.
	id := pluginapi.ID(tr.Source)
	if adapter := a.platforms[id]; adapter != nil && adapter.OwnsTrack(tr) {
		return id, true
	}
	for id, adapter := range a.platforms {
		if adapter != nil && adapter.OwnsTrack(tr) {
			return id, true
		}
	}
	return "", false
}

func (a *app) trackAdapter(tr *pluginapi.Track) pluginapi.Adapter {
	if id, ok := a.trackPlatform(tr); ok {
		if remote := a.remoteActive(); remote != nil && remote.owner == id {
			return remote.adapter
		}
		return a.platforms[id]
	}
	return nil
}

func (a *app) ownsTracksFor(id pluginapi.ID, tracks []pluginapi.Track) bool {
	for i := range tracks {
		if owner, ok := a.trackPlatform(&tracks[i]); !ok || owner != id {
			return false
		}
	}
	return true
}

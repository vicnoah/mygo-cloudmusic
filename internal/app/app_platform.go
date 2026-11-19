package app

import (
	"context"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/network"
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
	return a.platforms[a.platformID]
}

func (a *app) initPlatformState() bool {
	newPlatformAdapters(a)
	id := pluginapi.ID(a.settings.Platform)
	if !a.hasPlatform(id) {
		id = ""
		for _, entry := range a.platformRegistrations {
			if !a.hasPlatform(entry.ID) {
				continue
			}
			if a.settings.DeveloperMode || !entry.DeveloperOnly {
				id = entry.ID
				break
			}
			if id == "" {
				id = entry.ID
			}
		}
	}
	a.platformID = id
	if id == "" {
		return false
	}
	a.settings.Platform = string(id)
	return true
}

func (a *app) configurePlatformPlayer() {
	if a.art != nil {
		a.art.fetch = func() func(string) ([]byte, error) {
			owner, ctx := a.platformID, a.platformContext
			if ctx == nil {
				ctx = context.Background()
			}
			return func(url string) ([]byte, error) {
				return (platformHost{a: a}).fetchArtwork(ctx, owner, url)
			}
		}
		a.art.generation = func() uint64 { return a.platformGeneration }
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
	a.platformContext, a.cancelPlatform = context.WithCancel(context.Background())
	if activate := a.platformLifecycles[a.platformID].Activate; activate != nil {
		activate(a.platformContext, playbackContinuity{a: a, ctx: a.platformContext})
	}
}

func (a *app) deactivatePlatformView(switching bool) {
	if a.cancelPlatform != nil {
		a.cancelPlatform()
	}
	if deactivate := a.platformLifecycles[a.platformID].Deactivate; deactivate != nil {
		deactivate(switching)
	}
}

// platformInfos shows the plugins in registration order.
func (a *app) platformInfos() []pluginapi.Info {
	var infos []pluginapi.Info
	for _, entry := range a.platformRegistrations {
		if a.hasPlatform(entry.ID) && (!entry.DeveloperOnly || a.settings.DeveloperMode) {
			infos = append(infos, entry.Info)
		}
	}
	return infos
}

func (a *app) hasPlatform(id pluginapi.ID) bool {
	return id != "" && a.platforms[id] != nil && a.platformWindows[id][pluginapi.MainWindow].Root != nil
}

func (a *app) trackPlatform(tr *pluginapi.Track) (pluginapi.ID, bool) {
	if tr == nil {
		return "", false
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
		return a.platforms[id]
	}
	return nil
}

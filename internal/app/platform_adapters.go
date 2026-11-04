package app

import (
	appplatform "netease-music-mygo/internal/app/platform"
	appfnmusic "netease-music-mygo/internal/app/platform/fnmusic"
	appqqmusic "netease-music-mygo/internal/app/platform/qqmusic"
	appsubsonic "netease-music-mygo/internal/app/platform/subsonic"
	appsongloft "netease-music-mygo/internal/app/platform/songloft"
	"netease-music-mygo/internal/platform"
	backendsubsonic "netease-music-mygo/internal/platform/subsonic"
)

// Concrete adapter registration.
//
// This is the root application's only bootstrap point that names a concrete
// provider package. Keep provider construction here; the rest of the app
// should communicate through internal/app/platform interfaces.

// newPlatformAdapters is the only root bootstrap point that names a concrete
// adapter. All later app code uses the platform seam and can remain unchanged
// when another provider is added.
func newPlatformAdapters(a *app) {
	if a == nil {
		return
	}
	if a.platforms == nil {
		a.platforms = map[platform.ID]appplatform.Adapter{}
	}
	a.platforms[platform.FnmusicID] = appfnmusic.NewAdapter(platformHost{a: a}, &a.settings.FnMusic, func() { _ = a.settings.save() })
	a.platforms[platform.QQMusicID] = appqqmusic.NewAdapter(platformHost{a: a}, &a.settings.QQMusic, func() { _ = a.settings.save() })
	a.platforms[platform.NavidromeID] = appsubsonic.NewAdapter(platformHost{a: a}, backendsubsonic.ProfileNavidrome, &a.settings.Navidrome, func() { _ = a.settings.save() })
	a.platforms[platform.JellyfinID] = appsubsonic.NewAdapter(platformHost{a: a}, backendsubsonic.ProfileJellyfin, &a.settings.Jellyfin, func() { _ = a.settings.save() })
	a.platforms[platform.GonicID] = appsubsonic.NewAdapter(platformHost{a: a}, backendsubsonic.ProfileGonic, &a.settings.Gonic, func() { _ = a.settings.save() })
	a.platforms[platform.SubsonicID] = appsubsonic.NewAdapter(platformHost{a: a}, backendsubsonic.ProfileSubsonic, &a.settings.Subsonic, func() { _ = a.settings.save() })
	a.platforms[platform.SongloftID] = appsongloft.NewAdapter(platformHost{a: a}, &a.settings.Songloft, func() { _ = a.settings.save() })
}

// Package builtin assembles the music platforms included in the application.
package builtin

import (
	appplatform "waygu-music/internal/pluginapi"
	"waygu-music/internal/plugins/fnmusic"
	"waygu-music/internal/plugins/netease"
	"waygu-music/internal/plugins/qqmusic"
	"waygu-music/internal/plugins/songloft"
	"waygu-music/internal/plugins/subsonic"
)

// Platforms returns the platform constructors included in the desktop app.
func Platforms() []appplatform.Registration {
	// Feiniu is the bundled default on a fresh install.
	entries := []appplatform.Registration{fnmusic.Registration()}
	entries = append(entries, subsonic.Platforms()...)
	return append(entries, songloft.Registration(), netease.Registration(), qqmusic.Registration())
}

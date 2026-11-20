// Package builtin assembles the music platforms included in the application.
package builtin

import (
	"github.com/waygu-music-plugin/fnmusic"
	"github.com/waygu-music-plugin/sdk/pluginapi"
)

// Platforms returns the platform constructors included in the desktop app.
func Platforms() []pluginapi.Registration {
	return []pluginapi.Registration{fnmusic.Registration()}
}

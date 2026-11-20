// Package platformcatalog lists external platform identities and legacy settings keys.
package platformcatalog

import "github.com/waygu-music-plugin/sdk/pluginapi"

// OfficialCatalog is the public directory bundled with Wagyu Music.
const OfficialCatalog = "https://raw.githubusercontent.com/waygu-music-plugin/registry/main/catalog.json"
const OfficialPublisherKey = "dHGddBwaCGi+SH1mBves8SC5IJ4Xe+jFpxEEZSMGwy0="

// ExternalPlatforms returns metadata for entries supplied by installed packages.
func ExternalPlatforms() []pluginapi.Registration {
	return []pluginapi.Registration{
		{Info: pluginapi.Info{ID: "navidrome", Name: "Navidrome"}, LegacySettingsKey: "navidrome"},
		{Info: pluginapi.Info{ID: "jellyfin", Name: "Jellyfin"}, LegacySettingsKey: "jellyfin"},
		{Info: pluginapi.Info{ID: "gonic", Name: "Gonic"}, LegacySettingsKey: "gonic"},
		{Info: pluginapi.Info{ID: "subsonic", Name: "Subsonic 兼容"}, LegacySettingsKey: "subsonic"},
		{Info: pluginapi.Info{ID: "songloft", Name: "Songloft"}, LegacySettingsKey: "songloft"},
		{
			Info:                 pluginapi.Info{ID: "netease", Name: "网易云音乐"},
			LegacySettingsFields: []string{"quality", "normalize", "captions", "hotComments"},
		},
		{Info: pluginapi.Info{ID: "qqmusic", Name: "QQ 音乐"}, LegacySettingsKey: "qqmusic"},
	}
}

package songloft

import (
	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
)

// Page identifies a route owned by the Songloft platform UI.
type Page = string

const (
	PageLibrary       Page = "library"
	PageSearch        Page = "search"
	PageSearchAlbums  Page = "search-albums"
	PageSearchArtists Page = "search-artists"
	PageAlbums        Page = "albums"
	PageArtists       Page = "artists"
	PagePlaylist      Page = "playlist"
	PageFolders       Page = "folders"
)

// Route identifies a page in the Songloft UI.
type Route struct {
	Page  Page
	ID    int64
	Query string
	Kind  string
}

// PlayerPort is the playback interface for controlling queue and tracks.
type PlayerPort = pluginapi.PlayerPort

// Host provides desktop, playback, and persistence operations to Songloft.
type Host interface {
	pluginapi.DesktopServices
	pluginapi.DesktopPlaybackServices
	Post(func())
	Invalidate()
	ShowMain()
	OpenSettings()
	CloseSettings()
	Toast(string)
	TakeToast() string
	Player() PlayerPort
	PlaybackLevels() ([pluginapi.SpectrumBands]float32, bool)
	Artwork(string, int) (*ui.Bitmap, bool)
	CopyText(string)
	GlobalSearchHistory() []string
	RememberGlobalSearch(string)
	ClearSearchHistory()
	CurrentPlatform() pluginapi.ID
	PlatformOptions() []pluginapi.Info
	PlatformConfigured(pluginapi.ID) bool
	SwitchPlatform(pluginapi.ID)
}

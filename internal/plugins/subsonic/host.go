package subsonic

import appplatform "waygu-music/internal/pluginapi"

// Page identifies a route owned by the Subsonic platform UI.
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

// Route is the navigation value exchanged between views and the host.
type Route struct {
	Page  Page
	ID    int64
	Query string
	Kind  string
}

// PlayerPort is the playback interface for controlling queue and tracks.
type PlayerPort = appplatform.PlayerPort

// Host provides desktop and playback services to the plugin.
type Host = appplatform.Services

package fnmusic

import appplatform "waygu-music/internal/pluginapi"

// Page identifies a route owned by a platform UI. The root app translates it
// to its shared route type; the platform package never imports that type.
type Page = string

const (
	PageLibrary       Page = "library"
	PageSearch        Page = "search"
	PageSearchAlbums  Page = "search-albums"
	PageSearchArtists Page = "search-artists"
	PageAlbums        Page = "albums"
	PageArtists       Page = "artists"
	PageArtistAlbums  Page = "artist-albums"
	PagePlaylist      Page = "playlist"
	PageRoam          Page = "roam"
	PageFolders       Page = "folders"
	PageGenres        Page = "genres"
)

// Route is the small navigation value exchanged between a platform view and
// the root app. It deliberately contains no UI elements or app internals.
type Route struct {
	Page  Page
	ID    int64
	Query string
	Kind  string
}

// PlayerPort is the shared playback seam used by platform queue actions.
// The concrete player remains in the root app.
type PlayerPort = appplatform.PlayerPort

// Host provides the desktop and playback services used by the plugin.
type Host = appplatform.Services

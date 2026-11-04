package subsonic

import appplatform "netease-music-mygo/internal/app/platform"

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
type Route = appplatform.Route

// PlayerPort is the playback interface for controlling queue and tracks.
type PlayerPort = appplatform.PlayerPort

// Host is the seam giving access to rendering, notifications, and navigation.
type Host interface {
	appplatform.Host
}

type AccountAction = appplatform.AccountAction
type TrackView = appplatform.TrackView
type SearchView = appplatform.SearchView
type CardView = appplatform.CardView
type SortChoice = appplatform.SortChoice
type Runtime = appplatform.Runtime

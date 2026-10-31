package fnmusic

import "netease-music-mygo/internal/netease"

// Page identifies a route owned by a platform UI. The root app translates it
// to its shared route type; the platform package never imports that type.
type Page string

const (
	PageLibrary  Page = "library"
	PageSearch   Page = "search"
	PageAlbums   Page = "albums"
	PageArtists  Page = "artists"
	PagePlaylist Page = "playlist"
	PageRoam     Page = "roam"
)

// Route is the small navigation value exchanged between a platform view and
// the root app. It deliberately contains no UI elements or app internals.
type Route struct {
	Page  Page
	ID    int64
	Query string
	Kind  netease.SearchType
}

// PlayerPort is the shared playback seam used by platform queue actions.
// The concrete player remains in the root app.
type PlayerPort interface {
	Current() *netease.Track
	PlayQueue([]netease.Track, int)
	SetShuffle(bool)
}

// Host is the lifecycle seam for platform state and views. Rendering helpers
// will be added as grouped ports when the existing Feiniu home is moved; the
// platform package never receives *app or a collection of root callbacks.
type Host interface {
	Post(func())
	Invalidate()
	Navigate(Route)
	ShowMain()
	Toast(string)
	Player() PlayerPort
}

package fnmusic

import (
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform/paging"
)

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

	// The platform owns which navigation item and page is shown. The host
	// supplies only shared drawing and interaction primitives.
	SearchField(*ui.Context)
	NavItem(*ui.Context, string, string, Route)
	AccountRow(*ui.Context)
	NowPlaying(*ui.Context, *netease.Track)
	RenderConnection(*ui.Context, *State)
	RenderTracks(*ui.Context, *List, TrackView)
	RenderCards(*ui.Context, *paging.Pager[netease.Playlist], CardView)
	RenderSearch(*ui.Context, *State, Route)
}

// TrackView describes a platform list without exposing root app routes or UI
// list state to the platform package.
type TrackView struct {
	Title string
	Sub   string
}

// CardView describes an album, artist or playlist card page.
type CardView struct {
	Title string
	Sub   string
}

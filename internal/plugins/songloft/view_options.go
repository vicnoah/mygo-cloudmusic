package songloft

import (
	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/paging"
)

type AccountAction struct {
	Icon      *ui.SVG
	Label     string
	Shortcut  string
	Separator bool
	Danger    bool
	Action    func()
}

type TrackView struct {
	Title          string
	Sub            string
	Cover          string
	OnPlay         func(int)
	Header         func(*ui.Context)
	HeaderSkeleton func(*ui.Context)
	Menu           func(*ui.Menu, *pluginapi.Track, int64)
	Liked          func(int64) bool
	Favorite       func(pluginapi.Track, bool)
	ShowAlbum      bool
}

type CardView struct {
	Title          string
	Sub            string
	Count          int
	Header         func(*ui.Context)
	HeaderSkeleton func(*ui.Context)
	Sort           []SortChoice
	Current        string
	OnSort         func(string)
	OnOpen         func(int64)
	OnPlay         func(int64)
	// Meta reserves the card's secondary line in its loading skeleton.
	Meta bool
	// Round draws an artist's round photo over the name, with no play.
	Round bool
}

type SearchView struct {
	Play      func(int)
	PlayCard  func(int64)
	TrackMenu func(*ui.Menu, *pluginapi.Track, int64)
	Liked     func(int64) bool
	Favorite  func(pluginapi.Track, bool)
	OpenList  func(int64)
	OpenCards func(string)
}

type SearchResults struct {
	Query   string
	Songs   *paging.Pager[pluginapi.Track]
	Albums  *paging.Pager[pluginapi.Playlist]
	Artists *paging.Pager[pluginapi.Playlist]
}

type SortChoice struct{ Value, Label string }

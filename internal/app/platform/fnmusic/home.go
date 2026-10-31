package fnmusic

import (
	"github.com/egoist/mygo/ui"

	backend "netease-music-mygo/internal/platform/fnmusic"
)

// Home owns the Feiniu navigation and page selection. It deliberately does
// not know the root app's route type or render implementation.
type Home struct {
	state *State
	host  Host
	route Route
}

// NewHome creates the platform home shell. The initial route is the library,
// matching the existing Feiniu home without changing its layout.
func NewHome(state *State, host Host) *Home {
	return &Home{state: state, host: host, route: Route{Page: PageLibrary}}
}

// State returns the session rendered by this home.
func (h *Home) State() *State { return h.state }

// Route returns the platform-owned route.
func (h *Home) Route() Route { return h.route }

// Open changes the platform route without touching the root app's history.
func (h *Home) Open(route Route) {
	if route.Page == "" {
		route.Page = PageLibrary
	}
	h.route = route
}

// Sidebar renders Feiniu navigation through the host's shared controls.
func (h *Home) Sidebar(c *ui.Context) {
	if h == nil || h.host == nil {
		return
	}
	h.host.SearchField(c)
	if !h.state.Connected() {
		h.host.NavItem(c, "server", "连接飞牛音乐", Route{Page: PageLibrary})
	} else {
		h.host.NavItem(c, "library", "音乐库", Route{Page: PageLibrary})
		h.host.NavItem(c, "album", "专辑", Route{Page: PageAlbums})
		h.host.NavItem(c, "artist", "歌手", Route{Page: PageArtists})
		h.host.NavItem(c, "heart", "我喜欢", Route{Page: PagePlaylist, ID: backend.FavoritesID})
		h.host.NavItem(c, "clock", "最近播放", Route{Page: PagePlaylist, ID: backend.RecentID})
		h.host.NavItem(c, "radio", "漫游", Route{Page: PageRoam})
	}
	if h.route.Page == PageSearch {
		h.host.NavItem(c, "search", "搜索结果", h.route)
	}
	if player := h.host.Player(); player != nil {
		if tr := player.Current(); tr != nil {
			h.host.NowPlaying(c, tr)
		}
	}
	h.host.AccountRow(c)
}

// Content renders the selected Feiniu page. The host supplies the shared
// list/card painter, while the platform owns which data source is selected.
func (h *Home) Content(c *ui.Context) {
	if h == nil || h.host == nil || h.state == nil {
		return
	}
	if !h.state.Connected() {
		h.host.RenderConnection(c, h.state)
		return
	}
	switch h.route.Page {
	case PageSearch:
		h.host.RenderSearch(c, h.state, h.route)
	case PageAlbums:
		h.host.RenderCards(c, h.state.Catalog().AlbumsList(), CardView{Title: "专辑"})
	case PageArtists:
		h.host.RenderCards(c, h.state.Catalog().ArtistsList(), CardView{Title: "歌手"})
	case PageRoam:
		// Roaming has platform-specific controls and is rendered by the host
		// until its actions move into State.
		h.host.RenderConnection(c, h.state)
	default:
		l := h.state.Catalog().ListFor(h.route.ID)
		if h.route.Page == PageLibrary {
			l = h.state.Catalog().ListFor(backend.LibraryID)
		}
		if l != nil {
			h.host.RenderTracks(c, l, TrackView{Title: l.Name, Sub: l.Kind})
		} else {
			h.host.RenderConnection(c, h.state)
		}
	}
}

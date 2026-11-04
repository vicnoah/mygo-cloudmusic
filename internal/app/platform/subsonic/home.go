package subsonic

import (
	"context"

	"github.com/egoist/mygo/ui"

	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/subsonic"
)

type Home struct {
	state   *State
	host    Host
	runtime appplatform.Runtime
	route   Route
	history []Route
	forward []Route

	formAddress     string
	formUser        string
	formPassword    string
	formInsecure    bool
	connectionOpen  bool
	connectionBusy  bool
	connectionError string
	connectionGen   uint64
	accountOpen     bool
	search          string
	navScroll       ui.ScrollState

	nameOpen   bool
	deleteOpen bool
	name       string
	rename     *netease.Playlist
	delete     *netease.Playlist
	addTracks  []netease.Track
	addList    *List

	folderID        string
	folderPageData  backend.FolderPage
	folderLoading   bool
	folderError     string
	folderHistory   []string
	folderScroll    ui.ScrollState
	folderListState ui.ListState
	folderCrumbs    []folderCrumb
}

type folderCrumb struct {
	ID   string
	Name string
}

func NewHome(state *State, host Host, runtimes ...appplatform.Runtime) *Home {
	var runtime appplatform.Runtime
	if len(runtimes) > 0 {
		runtime = runtimes[0]
	}
	if runtime == nil {
		runtime = NewRuntime(state, nil)
	}
	h := &Home{
		state:   state,
		host:    host,
		runtime: runtime,
		route:   Route{Page: PageLibrary},
	}
	if state != nil {
		cfg := toConnectionConfig(state.Config())
		h.formAddress = cfg.Address
		h.formUser = cfg.Username
		h.formInsecure = cfg.HTTPS
	}
	return h
}

func (h *Home) State() *State { return h.state }

func (h *Home) Route() Route { return h.route }

func (h *Home) CanBack() bool    { return h != nil && len(h.history) > 0 }
func (h *Home) CanForward() bool { return h != nil && len(h.forward) > 0 }

func (h *Home) ResetNavigation(route Route) {
	if h == nil {
		return
	}
	if route.Page == "" {
		route.Page = PageLibrary
	}
	h.history, h.forward = nil, nil
	h.openRoute(route)
}

func (h *Home) SearchText() *string {
	if h == nil {
		return nil
	}
	return &h.search
}

func (h *Home) Open(route Route) {
	if h == nil {
		return
	}
	if route.Page == "" {
		route.Page = PageLibrary
	}
	if route == h.route {
		return
	}
	h.history = append(h.history, h.route)
	h.forward = nil
	h.openRoute(route)
}

func (h *Home) openRoute(route Route) {
	h.route = route
	if (route.Page == PageSearch || route.Page == PageSearchAlbums || route.Page == PageSearchArtists) && h.state != nil {
		if err := h.state.Catalog().Search(route.Query); err != nil {
			h.state.SetError(err)
		}
	}
	if route.Page == PageFolders && h.state != nil && h.state.Connected() {
		h.loadFolder(route.Query)
	}
}

func (h *Home) Back() {
	if h == nil || len(h.history) == 0 {
		return
	}
	n := len(h.history) - 1
	route := h.history[n]
	h.history = h.history[:n]
	h.forward = append(h.forward, h.route)
	h.openRoute(route)
}

func (h *Home) Forward() {
	if h == nil || len(h.forward) == 0 {
		return
	}
	n := len(h.forward) - 1
	route := h.forward[n]
	h.forward = h.forward[:n]
	h.history = append(h.history, h.route)
	h.openRoute(route)
}

func (h *Home) CloseTransient() {
	if h == nil {
		return
	}
	h.accountOpen = false
	h.connectionGen++
	h.connectionOpen, h.connectionBusy, h.connectionError = false, false, ""
	h.nameOpen, h.deleteOpen = false, false
}

func (h *Home) Content(c *ui.Context) {
	if h == nil || h.host == nil || h.state == nil {
		return
	}
	if h.state.Connecting() {
		h.connectionLoading(c)
		h.dialogs(c)
		return
	}
	if !h.state.Connected() {
		h.connectionPage(c)
		h.dialogs(c)
		return
	}
	switch h.route.Page {
	case PageSearch, PageSearchAlbums, PageSearchArtists, PageAlbums, PageArtists:
		h.cardsPage(c)
	case PageFolders:
		h.folderPage(c)
	default:
		h.listPage(c)
	}
	h.dialogs(c)
}

func (h *Home) connectionLoading(c *ui.Context) {
	switch h.route.Page {
	case PageSearch:
		h.host.SearchSkeleton(c)
	case PageSearchAlbums, PageSearchArtists, PageAlbums, PageArtists:
		h.host.CardsSkeleton(c, h.cardSkeletonView())
	default:
		h.host.TrackSkeleton(c, TrackView{})
	}
}

func (h *Home) RequestFavorite(tr netease.Track, liked bool) {
	if h.state == nil || h.state.LikeBusy(tr.ID) {
		return
	}
	// Optimistic UI update immediately
	h.state.SetLikedMemory(tr.ID, liked)
	if h.host != nil {
		h.host.Invalidate()
	}
	go func() {
		err := h.state.SetFavorite(context.Background(), &tr, liked)
		if h.host != nil {
			h.host.Post(func() {
				if err != nil {
					h.host.Toast("操作失败，请稍后再试")
				}
				// Refresh favorites list so "我喜欢" updates in real time
				if cat := h.state.Catalog(); cat != nil {
					if favList := cat.Lists[backend.FavoritesID]; favList != nil {
						favList.Refresh()
					}
				}
				h.host.Invalidate()
			})
		}
	}()
}

func (h *Home) cardSkeletonView() CardView {
	return CardView{
		HeaderSkeleton: h.cardsHeaderSkeleton,
	}
}

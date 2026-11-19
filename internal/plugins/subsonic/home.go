package subsonic

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	backend "waygu-music/internal/plugins/subsonic/internal/client"
)

type Home struct {
	state   *State
	scope   context.Context
	host    Host
	runtime *RuntimeAdapter
	route   Route
	history []Route
	forward []Route

	laidOutRoute                Route
	trackList, platformCardList ui.ListState
	platformHeroSince           time.Time
	underTop, underBottom       bool
	rowBars                     barState
	playbackView

	searchInput      ui.Handle
	platformMenuOpen bool

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
	rename     *pluginapi.Playlist
	delete     *pluginapi.Playlist
	addTracks  []pluginapi.Track
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

func NewHome(state *State, host Host, runtimes ...*RuntimeAdapter) *Home {
	var runtime *RuntimeAdapter
	if len(runtimes) > 0 {
		runtime = runtimes[0]
	}
	if runtime == nil {
		runtime = NewRuntime(state, nil)
	}
	h := &Home{
		state:   state,
		scope:   context.Background(),
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
	h.resetPageLayout()
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
	h.accountOpen, h.platformMenuOpen = false, false
	h.closePlaybackView()
	h.connectionGen++
	h.connectionOpen, h.connectionBusy, h.connectionError = false, false, ""
	h.nameOpen, h.deleteOpen = false, false
}

func (h *Home) navigate(route Route) {
	h.Open(route)
	h.nowPlaying = false
	h.host.Invalidate()
}

func (h *Home) content(c *ui.Context) {
	if h == nil || h.host == nil || h.state == nil {
		return
	}
	if h.laidOutRoute != h.route {
		h.resetPageLayout()
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
		h.platformSearchSkeleton(c)
	case PageSearchAlbums, PageSearchArtists, PageAlbums, PageArtists:
		h.platformCardsSkeleton(c, h.cardSkeletonView())
	default:
		h.platformTrackSkeleton(c, TrackView{})
	}
}

func (h *Home) RequestFavorite(tr pluginapi.Track, liked bool) {
	if h.state == nil || h.state.LikeBusy(tr.ID) {
		return
	}
	gen, ctx := h.state.Generation(), h.scope
	go func() {
		err := h.state.SetFavorite(ctx, &tr, liked)
		if h.host != nil {
			h.postResult(ctx, func() {
				if h.state.Generation() != gen {
					return
				}
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

func (h *Home) RecordStarted(tr pluginapi.Track) {
	if h == nil || h.state == nil {
		return
	}
	if cat := h.state.Catalog(); cat != nil && cat.Recent != nil {
		cat.Recent.Add(tr)
		if l := cat.Lists[backend.RecentID]; l != nil {
			l.Refresh()
		}
	}
}

func (h *Home) RemoveRecent(id int64) {
	if h == nil || h.state == nil {
		return
	}
	if cat := h.state.Catalog(); cat != nil {
		cat.RemoveRecent(id)
	}
	if h.host != nil {
		h.host.Invalidate()
	}
}

func (h *Home) ClearRecent() {
	if h == nil || h.state == nil {
		return
	}
	if cat := h.state.Catalog(); cat != nil {
		cat.ClearRecent()
	}
	if h.host != nil {
		h.host.Invalidate()
	}
}

// postResult applies a background result only to the view that started it.
func (h *Home) postResult(ctx context.Context, fn func()) {
	if h.host == nil {
		return
	}
	h.host.Post(func() {
		if ctx.Err() == nil {
			fn()
		}
	})
}

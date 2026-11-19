package qqmusic

import (
	"context"
	"math/rand/v2"
	"strconv"
	"sync"
	"time"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/paging"
	backend "waygu-music/internal/plugins/qqmusic/internal/client"

	"github.com/egoist/mygo/ui"
)

type playlistPage struct {
	pl      pluginapi.Playlist
	loading bool
	err     error
}

type albumPage struct {
	al        backend.AlbumProfile
	tracks    []pluginapi.Track
	liked     bool
	likeKnown bool
	likeBusy  bool
	loading   bool
	err       error
}

type artistPage struct {
	ar      backend.ArtistProfile
	tracks  []pluginapi.Track
	latest  []pluginapi.Track
	albums  []pluginapi.Playlist
	loading bool
	err     error
}

type userPage struct {
	user       *backend.User
	playlists  []pluginapi.Playlist
	loading    int
	err        error
	membership backend.Membership
	memberBusy bool
	memberErr  error
}

type searchPage struct {
	query   string
	songs   *paging.Pager[pluginapi.Track]
	albums  *paging.Pager[pluginapi.Playlist]
	artists *paging.Pager[pluginapi.Playlist]
}

type Home struct {
	state *State
	scope context.Context
	host  Host

	route   Route
	history []Route
	forward []Route

	laidOutRoute                Route
	trackList, platformCardList ui.ListState
	platformHeroSince           time.Time
	underTop, underBottom       bool
	rowBars                     barState
	playbackView

	preferences     *platformPreferences
	savePreferences func()

	booted                    bool
	bootPending               bool
	reloadHome, reloadLibrary bool
	libraryLoading            bool
	homeLoading               bool
	homeErr                   error
	daily                     []pluginapi.Track
	featured                  []pluginapi.Playlist
	charts                    []pluginapi.Playlist
	rec                       []pluginapi.Playlist
	shelves                   []backend.SongShelf
	shelfLoading              bool
	shelfPagers               map[string]*songShelfPager

	mu                  sync.Mutex
	playlists           map[int64]*playlistPage
	albums              map[int64]*albumPage
	artists             map[int64]*artistPage
	users               map[int64]*userPage
	liked               []pluginapi.Track
	likedLoaded         bool
	likedLoading        bool
	likedErr            string
	likedAlbums         []pluginapi.Playlist
	likedAlbumsLoaded   bool
	likedAlbumsMore     bool
	likedAlbumsBusy     bool
	fmStarting          bool
	artistTab           int
	purchasedSongs      []pluginapi.Track
	purchasedAlbums     []pluginapi.Playlist
	purchasedSongsMore  bool
	purchasedAlbumsMore bool
	purchasedMoreBusy   bool
	search              searchPage
	searchKind          int
	login               loginState
	accountOpen         bool
	nameOpen            bool
	name                string
	editing             *pluginapi.Playlist
	pending             []pluginapi.Track
	nameErr             string
	removeOpen          bool
	removeBusy          bool
	removeSaved         bool
	removeErr           string
	removing            *pluginapi.Playlist
	query               string
	searchBox           ui.Handle
	suggest             suggestState
	navScroll           ui.ScrollState
	savedIDs            map[int64]bool
	mineOpen            bool
	savedOpen           bool
	lists               map[Route]*ui.ListState
	purchasedTab        int
	purchasedLoaded     bool
	purchasedLoading    bool
	purchasedErr        error
}

func NewHome(state *State, host Host) *Home {
	h := &Home{
		state: state, scope: context.Background(), host: host, route: Route{Page: PageHome},
		homeLoading: true,
		playlists:   map[int64]*playlistPage{}, albums: map[int64]*albumPage{},
		artists: map[int64]*artistPage{}, users: map[int64]*userPage{}, lists: map[Route]*ui.ListState{},
		savedIDs: map[int64]bool{}, mineOpen: true, savedOpen: true,
	}
	return h
}

func (h *Home) ensureBoot() {
	if h.booted {
		if h.reloadHome {
			h.reloadHome = false
			h.loadHome()
		}
		if h.reloadLibrary {
			h.reloadLibrary = false
			if prof := h.state.Profile(); prof != nil {
				h.loadLibrary(prof.UserID)
			}
		}
		return
	}
	h.booted, h.bootPending = true, true
	go h.boot(h.scope)
}

func (h *Home) listState() *ui.ListState {
	st := h.lists[h.route]
	if st == nil {
		st = &ui.ListState{}
		h.lists[h.route] = st
	}
	return st
}

func (h *Home) postResult(ctx context.Context, fn func()) {
	if h == nil || h.host == nil {
		return
	}
	h.host.Post(func() {
		if ctx.Err() == nil && h.host.CurrentPlatform() == ID {
			fn()
		}
	})
}

func (h *Home) boot(ctx context.Context) {
	var prof *backend.Profile
	if backend.RestoreSession() {
		p, err := backend.AccountContext(ctx)
		if err == nil {
			prof = p
		}
	}
	h.postResult(ctx, func() {
		h.bootPending = false
		h.signedIn(prof)
	})
}

func (h *Home) signedIn(prof *backend.Profile) {
	h.state.SetProfile(prof)
	h.state.SetPlaylists(nil)
	h.state.SetLikedIDs(nil)
	h.mu.Lock()
	h.playlists = map[int64]*playlistPage{}
	h.savedIDs = map[int64]bool{}
	h.albums = map[int64]*albumPage{}
	h.artists = map[int64]*artistPage{}
	h.users = map[int64]*userPage{}
	h.liked, h.likedLoaded, h.likedLoading, h.likedErr = nil, false, false, ""
	h.likedAlbums, h.likedAlbumsLoaded = nil, false
	h.likedAlbumsMore, h.likedAlbumsBusy = false, false
	h.fmStarting = false
	h.shelves = nil
	h.shelfPagers = nil
	h.shelfLoading = false
	h.purchasedSongs, h.purchasedAlbums = nil, nil
	h.purchasedSongsMore, h.purchasedAlbumsMore, h.purchasedMoreBusy = false, false, false
	h.purchasedLoaded, h.purchasedLoading, h.purchasedErr = false, false, nil
	h.search = searchPage{}
	h.mu.Unlock()
	h.loadHome()
	if prof != nil {
		h.loadLibrary(prof.UserID)
	}
	h.host.Invalidate()
}

func (h *Home) loadHome() {
	ctx := h.scope
	h.homeLoading = true
	h.homeErr = nil
	go func() {
		featured, err1 := backend.FeaturedPlaylists(ctx, 30)
		charts, err2 := backend.Charts(ctx)
		daily, _ := backend.DailySongs(ctx)
		var rec []backend.Playlist
		var shelves []backend.SongShelf
		if h.state.Profile() != nil {
			rec, _ = backend.RecommendedPlaylists(ctx)
			shelves, _ = backend.RecommendedSongShelves(ctx)
		}
		h.postResult(ctx, func() {
			h.homeLoading = false
			if err1 != nil && err2 != nil {
				h.homeErr = err1
				h.host.Invalidate()
				return
			}
			h.featured = playlists(featured)
			h.charts = playlists(charts)
			h.daily = tracks(daily)
			h.rec = playlists(rec)
			h.shelves = shelves
			h.shelfPagers = nil
			h.host.Invalidate()
		})
	}()
}

func (h *Home) loadLibrary(uid int64) {
	ctx := h.scope
	h.libraryLoading = true
	go func() {
		made, saved, err := backend.UserPlaylists(ctx, uid)
		albums, _, _ := backend.LikedAlbums(ctx, 100, 0)
		savedIDs := make(map[int64]bool, len(saved))
		for _, pl := range saved {
			savedIDs[pl.ID] = true
		}
		h.postResult(ctx, func() {
			h.libraryLoading = false
			if err == nil {
				all := append(playlists(made), playlists(saved)...)
				h.state.SetPlaylists(all)
				h.mu.Lock()
				h.savedIDs = savedIDs
				h.mu.Unlock()
			}
			h.likedAlbums, h.likedAlbumsLoaded = albumCards(albums), true
			h.host.Invalidate()
		})
	}()
	h.likedLoading = true
	go func() {
		out, err := backend.LikedTracks(ctx, uid)
		list := tracks(out)
		h.postResult(ctx, func() {
			h.liked, h.likedLoaded, h.likedLoading = list, true, false
			if err != nil && len(list) == 0 {
				h.likedErr = "加载失败，请稍后重试"
			} else {
				h.likedErr = ""
			}
			h.state.SetLikedIDs(trackIDs(list))
			h.host.Invalidate()
		})
	}()
}

func (h *Home) Route() Route        { return h.route }
func (h *Home) CanBack() bool       { return h != nil && len(h.history) > 0 }
func (h *Home) CanForward() bool    { return h != nil && len(h.forward) > 0 }
func (h *Home) SearchText() *string { return &h.query }

func (h *Home) Open(r Route) {
	if h == nil {
		return
	}
	if r.Page == "" {
		r.Page = PageHome
	}
	if r == h.route {
		return
	}
	h.history = append(h.history, h.route)
	h.forward = nil
	h.route = r
	h.ensure(r)
}
func (h *Home) ResetNavigation(r Route) {
	if r.Page == "" || r.Page == "library" {
		r.Page = PageHome
	}
	h.history, h.forward = nil, nil
	h.route = r
	h.resetPageLayout()
	h.ensure(r)
}
func (h *Home) Back() {
	if len(h.history) == 0 {
		return
	}
	n := len(h.history) - 1
	r := h.history[n]
	h.history = h.history[:n]
	h.forward = append(h.forward, h.route)
	h.route = r
	h.ensure(r)
}
func (h *Home) Forward() {
	if len(h.forward) == 0 {
		return
	}
	n := len(h.forward) - 1
	r := h.forward[n]
	h.forward = h.forward[:n]
	h.history = append(h.history, h.route)
	h.route = r
	h.ensure(r)
}
func (h *Home) activate(ctx context.Context) {
	h.scope = ctx
	if h.search.songs != nil {
		h.search.songs.SetScope(ctx)
		h.search.albums.SetScope(ctx)
		h.search.artists.SetScope(ctx)
	}
	h.ResetNavigation(Route{Page: PageHome})
}

func (h *Home) CloseTransient() {
	h.cancelLogin()
	h.accountOpen = false
	h.nameOpen, h.removeOpen, h.removeBusy = false, false, false
	h.dismissSuggestions()
	h.closePlaybackView()

	if h.bootPending {
		h.booted, h.bootPending = false, false
	} else if h.homeLoading {
		h.reloadHome = true
	}
	h.reloadLibrary = h.reloadLibrary || h.libraryLoading || h.likedLoading
	h.libraryLoading, h.likedLoading, h.likedAlbumsBusy = false, false, false
	h.shelfLoading, h.fmStarting = false, false
	h.purchasedLoading, h.purchasedMoreBusy = false, false

	h.mu.Lock()
	for id, page := range h.playlists {
		if page.loading {
			delete(h.playlists, id)
		}
	}
	for id, page := range h.albums {
		if page.loading {
			delete(h.albums, id)
		}
		page.likeBusy = false
	}
	for id, page := range h.artists {
		if page.loading {
			delete(h.artists, id)
		}
	}
	for id, page := range h.users {
		if page.loading > 0 || page.memberBusy {
			delete(h.users, id)
		}
	}
	h.mu.Unlock()
}

func (h *Home) ensure(r Route) {
	switch r.Page {
	case PagePlaylist:
		h.loadPlaylist(r.ID)
	case PageAlbum:
		h.loadAlbum(r.ID)
	case PageArtist:
		h.loadArtist(r.ID)
	case PagePurchased:
		h.loadPurchased()
	case PageLiked:
		h.loadLiked()
	case PageLikedAlbums:
		h.loadLikedAlbums()
	case PageUser:
		h.loadUser(r.ID)
	}
}

func (h *Home) loadPlaylist(id int64) {
	ctx := h.scope
	h.mu.Lock()
	if p := h.playlists[id]; p != nil {
		h.mu.Unlock()
		return
	}
	p := &playlistPage{loading: true}
	h.playlists[id] = p
	h.mu.Unlock()
	go func() {
		pl, err := backend.PlaylistDetail(ctx, id)
		h.postResult(ctx, func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if err != nil {
				p.loading, p.err = false, err
			} else {
				p.loading, p.pl = false, playlist(*pl)
			}
			h.host.Invalidate()
		})
	}()
}

func (h *Home) loadAlbum(id int64) {
	ctx := h.scope
	h.mu.Lock()
	if p := h.albums[id]; p != nil {
		h.mu.Unlock()
		return
	}
	p := &albumPage{loading: true}
	h.albums[id] = p
	h.mu.Unlock()
	go func() {
		al, err := backend.AlbumDetail(ctx, id)
		liked, likeErr := false, error(nil)
		if err == nil && al.MID != "" {
			liked, likeErr = backend.AlbumLiked(ctx, al.MID)
		}
		h.postResult(ctx, func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if err != nil {
				p.loading, p.err = false, err
			} else {
				p.al, p.tracks, p.loading = *al, tracks(al.Songs), false
				p.liked, p.likeKnown = liked, likeErr == nil
			}
			h.host.Invalidate()
		})
	}()
}

func (h *Home) loadArtist(id int64) {
	ctx := h.scope
	h.mu.Lock()
	if p := h.artists[id]; p != nil {
		h.mu.Unlock()
		return
	}
	p := &artistPage{loading: true}
	h.artists[id] = p
	h.mu.Unlock()
	go func() {
		ar, err := backend.ArtistDetail(ctx, id)
		var albums []pluginapi.Playlist
		var latest []pluginapi.Track
		if err == nil {
			if list, _, aerr := backend.ArtistAlbums(ctx, id, 100, 0); aerr == nil {
				albums = albumCards(list)
			}
			if list, lerr := backend.ArtistLatestSongs(ctx, id, 100); lerr == nil {
				latest = tracks(list)
			}
		}
		h.postResult(ctx, func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if err != nil {
				p.loading, p.err = false, err
			} else {
				p.ar, p.tracks, p.albums, p.latest, p.loading = *ar, tracks(ar.Songs), albums, latest, false
			}
			h.host.Invalidate()
		})
	}()
}

// loadLiked reads the account's hearted songs, unless they are already on
// their way or here.
func (h *Home) loadLiked() {
	ctx := h.scope
	if h.likedLoaded || h.likedLoading || h.state.Profile() == nil {
		return
	}
	h.likedLoading = true
	uid := h.state.Profile().UserID
	go func() {
		out, err := backend.LikedTracks(ctx, uid)
		list := tracks(out)
		h.postResult(ctx, func() {
			h.liked, h.likedLoaded, h.likedLoading = list, true, false
			if err != nil && len(list) == 0 {
				h.likedErr = "加载失败，请稍后重试"
			} else {
				h.likedErr = ""
			}
			h.state.SetLikedIDs(trackIDs(list))
			h.host.Invalidate()
		})
	}()
}

// loadLikedAlbums refreshes the album collection each time its page opens,
// since albums are collected from album pages.
func (h *Home) loadLikedAlbums() {
	ctx := h.scope
	if h.state.Profile() == nil {
		return
	}
	go func() {
		albums, more, err := backend.LikedAlbums(ctx, 60, 0)
		if err != nil {
			return
		}
		list := albumCards(albums)
		h.postResult(ctx, func() {
			h.mu.Lock()
			h.likedAlbums, h.likedAlbumsLoaded, h.likedAlbumsMore = list, true, more
			h.mu.Unlock()
			h.host.Invalidate()
		})
	}()
}

// moreLikedAlbums appends the next page of collected albums.
func (h *Home) moreLikedAlbums() {
	ctx := h.scope
	if h.likedAlbumsBusy || !h.likedAlbumsMore {
		return
	}
	h.likedAlbumsBusy = true
	h.host.Invalidate()
	offset := len(h.likedAlbums)
	go func() {
		albums, more, err := backend.LikedAlbums(ctx, 60, offset)
		h.postResult(ctx, func() {
			h.likedAlbumsBusy = false
			if err == nil {
				h.likedAlbums = append(h.likedAlbums, albumCards(albums)...)
				h.likedAlbumsMore = more
			}
			h.host.Invalidate()
		})
	}()
}

func (h *Home) loadUser(id int64) {
	ctx := h.scope
	h.mu.Lock()
	p := h.users[id]
	if p != nil && (p.user != nil || p.loading > 0) {
		h.mu.Unlock()
		return
	}
	if p == nil {
		p = &userPage{}
		h.users[id] = p
	}
	p.loading, p.err = 2, nil
	h.mu.Unlock()
	go func() {
		u, err := backend.UserDetail(ctx, id)
		h.postResult(ctx, func() {
			h.mu.Lock()
			p.loading--
			if err != nil {
				p.err = err
			} else {
				p.user = u
			}
			h.mu.Unlock()
			if err == nil {
				h.loadMembership(p)
			}
			h.host.Invalidate()
		})
	}()
	go func() {
		made, _, err := backend.UserPlaylists(ctx, id)
		h.postResult(ctx, func() {
			h.mu.Lock()
			p.loading--
			if err == nil {
				var cards []pluginapi.Playlist
				for _, pl := range made {
					if pl.Creator.UserID == id && pl.Privacy != 10 {
						cards = append(cards, playlist(pl))
					}
				}
				p.playlists = cards
			}
			h.mu.Unlock()
			h.host.Invalidate()
		})
	}()
}

func (h *Home) loadMembership(p *userPage) {
	ctx := h.scope
	h.mu.Lock()
	if p.memberBusy {
		h.mu.Unlock()
		return
	}
	p.memberBusy, p.memberErr = true, nil
	h.mu.Unlock()
	go func() {
		level, err := backend.UserMembership(ctx)
		h.postResult(ctx, func() {
			h.mu.Lock()
			p.memberBusy, p.memberErr = false, err
			if err == nil {
				p.membership = level
			}
			h.mu.Unlock()
			h.host.Invalidate()
		})
	}()
}

func (h *Home) loadPurchased() {
	ctx := h.scope
	if h.purchasedLoaded || h.purchasedLoading {
		return
	}
	h.purchasedLoading = true
	go func() {
		songs, moreSongs, err1 := backend.PurchasedSongs(ctx, 100, 0)
		albums, moreAlbums, err2 := backend.PurchasedAlbums(ctx, 100, 0)
		h.postResult(ctx, func() {
			h.purchasedLoading = false
			if err1 != nil && err2 != nil {
				h.purchasedErr = err1
			} else {
				h.purchasedLoaded = true
				h.purchasedSongs = tracks(songs)
				h.purchasedAlbums = albumCards(albums)
				h.purchasedSongsMore, h.purchasedAlbumsMore = moreSongs, moreAlbums
			}
			h.host.Invalidate()
		})
	}()
}

// morePurchased appends the next page of purchases, songs or albums.
func (h *Home) morePurchased(albums bool) {
	ctx := h.scope
	if h.purchasedMoreBusy {
		return
	}
	h.purchasedMoreBusy = true
	h.host.Invalidate()
	go func() {
		var songs []backend.Track
		var cards []pluginapi.Playlist
		var more bool
		var err error
		if albums {
			var list []backend.AlbumProfile
			list, more, err = backend.PurchasedAlbums(ctx, 100, len(h.purchasedAlbums))
			cards = albumCards(list)
		} else {
			songs, more, err = backend.PurchasedSongs(ctx, 100, len(h.purchasedSongs))
		}
		h.postResult(ctx, func() {
			h.purchasedMoreBusy = false
			if err != nil {
				h.host.Toast("加载失败，请稍后再试")
				h.host.Invalidate()
				return
			}
			if albums {
				h.purchasedAlbums = append(h.purchasedAlbums, cards...)
				h.purchasedAlbumsMore = more
			} else {
				h.purchasedSongs = append(h.purchasedSongs, tracks(songs)...)
				h.purchasedSongsMore = more
			}
			h.host.Invalidate()
		})
	}()
}

// refreshPurchased fetches the account's purchases again, keeping what is on
// screen until the new lists arrive.
func (h *Home) refreshPurchased() {
	if h.purchasedLoading {
		return
	}
	h.purchasedLoaded = false
	h.purchasedErr = nil
	h.loadPurchased()
}

func (h *Home) started(tr *pluginapi.Track) {
	if tr == nil || tr.Source != Source {
		return
	}
	// QQ records normal streamed playback itself; this hook is kept for parity
	// with other providers and future history refreshes.
}

func (h *Home) play(list []pluginapi.Track, at int) {
	if p := h.host.Player(); p != nil && len(list) > 0 {
		p.PlayQueue(list, at)
	}
}

func (h *Home) shuffle(list []pluginapi.Track) {
	if len(list) == 0 {
		return
	}
	out := append([]pluginapi.Track(nil), list...)
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	h.play(out, 0)
}

// playAlbum starts an album from its card, loading the track list first when
// the album page has not been opened yet.
func (h *Home) playAlbum(id int64) {
	ctx := h.scope
	h.mu.Lock()
	p := h.albums[id]
	var loaded []pluginapi.Track
	if p != nil {
		loaded = p.tracks
	}
	h.mu.Unlock()
	if len(loaded) > 0 {
		h.play(loaded, 0)
		return
	}
	go func() {
		al, err := backend.AlbumDetail(ctx, id)
		h.postResult(ctx, func() {
			if err != nil || len(al.Songs) == 0 {
				h.host.Toast("专辑加载失败，请稍后再试")
				return
			}
			h.play(tracks(al.Songs), 0)
		})
	}()
}

// playArtist starts an artist's songs from a card.
func (h *Home) playArtist(id int64) {
	ctx := h.scope
	h.mu.Lock()
	p := h.artists[id]
	var loaded []pluginapi.Track
	if p != nil {
		loaded = p.tracks
	}
	h.mu.Unlock()
	if len(loaded) > 0 {
		h.play(loaded, 0)
		return
	}
	go func() {
		ar, err := backend.ArtistDetail(ctx, id)
		h.postResult(ctx, func() {
			if err != nil || len(ar.Songs) == 0 {
				h.host.Toast("歌手歌曲加载失败，请稍后再试")
				return
			}
			h.play(tracks(ar.Songs), 0)
		})
	}()
}

// playPlaylist starts a playlist from its card.
func (h *Home) playPlaylist(id int64) {
	ctx := h.scope
	h.mu.Lock()
	p := h.playlists[id]
	var loaded []pluginapi.Track
	if p != nil {
		loaded = p.pl.Tracks
	}
	h.mu.Unlock()
	if len(loaded) > 0 {
		h.play(loaded, 0)
		return
	}
	go func() {
		pl, err := backend.PlaylistDetail(ctx, id)
		h.postResult(ctx, func() {
			if err != nil || len(pl.Tracks) == 0 {
				h.host.Toast("歌单加载失败，请稍后再试")
				return
			}
			h.play(tracks(pl.Tracks), 0)
		})
	}()
}

func (h *Home) navigate(route Route) {
	h.Open(route)
	h.nowPlaying = false
	h.host.Invalidate()
}

func (h *Home) content(c *ui.Context) {
	if h.laidOutRoute != h.route {
		h.resetPageLayout()
	}
	h.ensureBoot()
	switch h.route.Page {
	case PageHome:
		h.homePage(c)
	case PageDaily:
		h.dailyPage(c)
	case PageFM:
		h.fmPage(c)
	case PageCharts:
		h.chartsPage(c)
	case PagePlaylist:
		h.playlistPage(c)
	case PageAlbum:
		h.albumPage(c)
	case PageArtist:
		h.artistPage(c)
	case PageSearch:
		h.searchPage(c)
	case PagePurchased:
		h.purchasedPage(c)
	case PageLiked:
		h.likedPage(c)
	case PageLikedAlbums:
		h.likedPage(c)
	case PageUser:
		h.userPage(c)
	}
	h.loginDialog(c)
	h.playlistDialog(c)
	h.removeDialog(c)
}

func (h *Home) sidebar(c *ui.Context) {
	if h == nil || h.host == nil {
		return
	}
	h.ensureBoot()
	h.searchField(c)
	ui.Scroll(c.Key("qq-nav")).TrackScroll(&h.navScroll).Grow(1).Margin(0, -6).Padding(0, 6).Gap(4).Children(func() {
		h.navItem(c, iconHome, "为你推荐", Route{Page: PageHome})
		h.navItem(c, iconDaily, "每日推荐", Route{Page: PageDaily})
		h.navItem(c, iconRadio, "电台", Route{Page: PageFM})
		h.navItem(c, iconCharts, "排行榜", Route{Page: PageCharts})
		h.navItem(c, iconPurchased, "已购音乐", Route{Page: PagePurchased})
		if h.query != "" {
			h.navItem(c, iconSearch, "搜索结果", Route{Page: PageSearch, Query: h.query})
		}
		h.sidebarPlaylists(c)
	})
	if tr := h.currentTrack(); tr != nil {
		h.sidebarNowPlaying(c, tr)
	}
	profile := h.state.Profile()
	name, status, avatar := "登录 QQ 音乐", "未登录", ""
	if profile != nil {
		name, status, avatar = profile.Nickname, "", profile.AvatarURL
	}
	actions := []AccountAction{
		{Icon: iconSettings, Label: "设置…", Shortcut: "Ctrl+,", Action: func() { h.host.OpenSettings() }},
		{Icon: iconUser, Label: "登录 QQ 音乐…", Separator: true, Action: h.openLogin},
	}
	if profile != nil {
		uid := profile.UserID
		actions = []AccountAction{
			{Icon: iconUser, Label: "我的主页", Action: func() { h.Open(Route{Page: PageUser, ID: uid}) }},
			{Icon: iconSettings, Label: "设置…", Shortcut: "Ctrl+,", Action: func() { h.host.OpenSettings() }},
			{Icon: iconSignOut, Label: "退出登录", Separator: true, Danger: true, Action: func() {
				backend.Logout()
				h.signedIn(nil)
			}},
		}
	}
	h.accountRow(c, avatar, name, status, actions)
}

// sidebarPlaylists splits the account's lists into the ones it made and the
// ones it saved, each under a heading that folds away.
func (h *Home) sidebarPlaylists(c *ui.Context) {
	h.mu.Lock()
	saved := h.savedIDs
	h.mu.Unlock()
	var mine, fav []pluginapi.Playlist
	for _, pl := range h.state.Playlists() {
		if saved[pl.ID] {
			fav = append(fav, pl)
		} else {
			mine = append(mine, pl)
		}
	}
	var create func()
	if h.state.Profile() != nil {
		create = h.openNewPlaylist
	}
	h.sidebarGroup(c, "我的歌单", &h.mineOpen, create)
	if h.mineOpen {
		h.sidebarLiked(c)
		for _, pl := range mine {
			pl := pl
			r := Route{Page: PagePlaylist, ID: pl.ID}
			h.playlistItem(c, "qq-playlist-"+strconvID(pl.ID), pl, h.route == r, func(m *ui.Menu) { h.playlistMenu(m, pl) }, func() { h.Open(r) })
		}
	}
	if len(fav) > 0 {
		h.sidebarGroup(c, "收藏的歌单", &h.savedOpen, nil)
		if h.savedOpen {
			for _, pl := range fav {
				pl := pl
				r := Route{Page: PagePlaylist, ID: pl.ID}
				h.playlistItem(c, "qq-saved-"+strconvID(pl.ID), pl, h.route == r, func(m *ui.Menu) {
					if m.Item("取消收藏…").Chosen() {
						h.unfavoritePlaylist(pl)
					}
				}, func() { h.Open(r) })
			}
		}
	}
}

// sidebarLiked is the heart's own list, first of the ones the account made.
func (h *Home) sidebarLiked(c *ui.Context) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key("qq-liked")).Height(34).PaddingX(8).Gap(10).Radius(8).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label("我喜欢")
	selected := h.route.Page == PageLiked || h.route.Page == PageLikedAlbums
	if selected {
		b.Background(t.Text.Alpha(0.08))
	} else if b.Hovered() {
		b.Background(t.Text.Alpha(0.04))
	}
	b.Children(func() {
		ui.Box(c).Size(22, 22).Radius(5).Background(ui.Hex("#e5484d").Alpha(0.16)).Center().Children(func() {
			ui.Icon(c, iconHeartFill).Size(12, 12).TextColor(ui.Hex("#e5484d"))
		})
		ui.Text(c, "我喜欢").FontSize(13).SingleLine()
	})
	if b.Clicked() {
		h.Open(Route{Page: PageLiked})
	}
}

func (h *Home) sidebarGroup(c *ui.Context, label string, open *bool, create func()) {
	t := c.Theme()
	ui.Row(c).Height(30).PaddingX(8).AlignItems(ui.Center).Gap(6).Children(func() {
		b := ui.ButtonBase(c.Key("qq-group-" + label)).Grow(1).Height(24).Gap(6).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(label)
		b.Children(func() {
			icon := iconDown
			if !*open {
				icon = iconForward
			}
			ui.Icon(c, icon).Size(11, 11).TextColor(t.TextMuted)
			ui.Text(c, label).FontSize(11.5).FontWeight(600).TextColor(t.TextMuted)
		})
		if b.Clicked() {
			*open = !*open
		}
		if create != nil {
			plus := ui.ButtonBase(c.Key("qq-group-add-"+label)).Size(22, 22).Radius(6).Center().Label("新建歌单")
			plus.Children(func() { ui.Icon(c, iconPlus).Size(13, 13).TextColor(t.TextMuted) })
			if plus.Clicked() {
				create()
			}
		}
	})
}

func strconvID(id int64) string {
	return strconv.FormatInt(id, 10)
}

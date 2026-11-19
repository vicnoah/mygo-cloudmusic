package fnmusic

import (
	"context"
	"errors"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/paging"
	"waygu-music/internal/plugins/fnmusic/internal/client"
)

const (
	PageSize  = 100
	PageAhead = 2
	CardsPage = 60
)

// ListKind describes a Feiniu catalog at the data layer. It does not select a
// UI; the platform home chooses how to present each list.
type ListKind string

const (
	ListLibrary   ListKind = "library"
	ListFavorites ListKind = "favorites"
	ListRecent    ListKind = "recent"
	ListPlaylist  ListKind = "playlist"
	ListAlbum     ListKind = "album"
	ListArtist    ListKind = "artist"
	ListSearch    ListKind = "search"
	ListGenre     ListKind = "genre"
)

// Spec identifies a service list without referring to a root app route.
type Spec struct {
	Kind  ListKind
	Ref   string
	Query string
	Sort  string
}

// List is a lazily loaded Feiniu catalog. Its pager is platform state, not UI
// state, so a view can be rebuilt without losing loaded pages.
type List struct {
	ID           int64
	Spec         Spec
	Kind         string
	HeaderKind   string
	Name         string
	Sub          string
	CoverURL     string
	Pages        *paging.Pager[pluginapi.Track]
	shuffleScope context.Context
}

func (l *List) shuffling() bool {
	return l.shuffleScope != nil && l.shuffleScope.Err() == nil
}

func (l *List) Cover() string {
	if l == nil {
		return ""
	}
	if l.CoverURL != "" {
		return l.CoverURL
	}
	if t := l.Pages.At(0); t != nil {
		return t.Album.PicURL
	}
	return ""
}

// SetSort changes the order of a loaded list without dropping its geometry.
func (l *List) SetSort(sort string) {
	if l == nil || l.Spec.Sort == sort {
		return
	}
	l.Spec.Sort = sort
	if l.Pages != nil {
		l.Pages.Reload()
	}
}

// Refresh asks for the current list again instead of trusting its loaded
// pages. Retry alone only touches failed pages and leaves cached rows in place.
func (l *List) Refresh() {
	if l == nil || l.Pages == nil {
		return
	}
	l.Pages.Reload()
	l.Pages.At(0)
}

// Catalog owns list and card state for one Feiniu session.
type Catalog struct {
	Session  *State
	scope    context.Context
	Post     func(func())
	OnTracks func([]pluginapi.Track)

	Lists                       map[int64]*List
	Meta                        map[int64]pluginapi.Playlist
	SearchList                  *List
	SearchAlbums, SearchArtists *paging.Pager[pluginapi.Playlist]
	Albums, Artists, Genres     *paging.Pager[pluginapi.Playlist]
	ArtistAlbums                map[int64]*paging.Pager[pluginapi.Playlist]

	LibrarySort, FavoriteSort, AlbumSort, AlbumDetailSort string
	ArtistSort, ArtistTrackSort, ArtistAlbumSort          string
	GenreSort, GenreTrackSort                             string
}

// NewCatalog creates empty platform catalog state.
func NewCatalog(session *State, post func(func())) *Catalog {
	if post == nil {
		post = func(fn func()) { fn() }
	}
	return &Catalog{
		Session: session, Post: post, scope: context.Background(),
		Lists: map[int64]*List{}, Meta: map[int64]pluginapi.Playlist{},
		ArtistAlbums:    map[int64]*paging.Pager[pluginapi.Playlist]{},
		LibrarySort:     fnmusic.LibrarySortPolicy.Default,
		FavoriteSort:    fnmusic.FavoriteSortPolicy.Default,
		AlbumSort:       fnmusic.AlbumListSortPolicy.Default,
		AlbumDetailSort: fnmusic.AlbumTrackSortPolicy.Default,
		ArtistSort:      fnmusic.ArtistListSortPolicy.Default,
		ArtistTrackSort: fnmusic.ArtistTrackSortPolicy.Default,
		ArtistAlbumSort: fnmusic.ArtistAlbumSortPolicy.Default,
		GenreSort:       fnmusic.GenreListSortPolicy.Default,
		GenreTrackSort:  fnmusic.GenreTrackSortPolicy.Default,
	}
}

func (c *Catalog) trackPager(fetch func(ctx context.Context, page int) ([]pluginapi.Track, int, error)) *paging.Pager[pluginapi.Track] {
	p := paging.NewScoped(c.scope, PageSize, PageAhead, c.Post, fetch)
	p.SetLanded(func(_ int, items []pluginapi.Track) {
		if c.OnTracks != nil {
			c.OnTracks(items)
		}
	})
	return p
}

func (c *Catalog) cardPager(fetch func(ctx context.Context, page int) ([]pluginapi.Playlist, int, error)) *paging.Pager[pluginapi.Playlist] {
	p := paging.NewScoped(c.scope, CardsPage, 1, c.Post, fetch)
	p.SetLanded(func(_ int, items []pluginapi.Playlist) {
		for _, item := range items {
			c.Meta[item.ID] = item
		}
	})
	return p
}

func catalogPage[T any](c *Catalog, fetch func(*fnmusic.Client, int) ([]T, int, error)) func(context.Context, int) ([]T, int, error) {
	return func(ctx context.Context, page int) ([]T, int, error) {
		client, gen, err := c.Session.activeClient(ctx)
		if err != nil {
			return nil, 0, err
		}
		items, total, err := fetch(client.WithContext(ctx), page)
		if err == nil && !c.Session.clientCurrent(client, gen) {
			return nil, 0, context.Canceled
		}
		return items, total, err
	}
}

// NewList builds a list from a service-neutral spec.
func (c *Catalog) NewList(spec Spec) *List {
	if c == nil || c.Session == nil {
		return nil
	}
	if _, _, err := c.Session.activeClient(c.scope); err != nil {
		return nil
	}
	l := &List{Spec: spec, ID: listID(spec)}
	var fetch func(*fnmusic.Client, int, int) ([]pluginapi.Track, int, error)
	switch spec.Kind {
	case ListLibrary:
		l.Kind, l.Name = "飞牛音乐", "音乐库"
		l.HeaderKind = "音乐库"
		fetch = func(client *fnmusic.Client, page, size int) ([]pluginapi.Track, int, error) {
			return client.LibraryPage(l.Spec.Sort, page, size)
		}
	case ListFavorites:
		l.Kind, l.Name = "飞牛音乐", "我喜欢"
		l.HeaderKind = "歌单"
		fetch = func(client *fnmusic.Client, page, size int) ([]pluginapi.Track, int, error) {
			return client.FavoritesPage(l.Spec.Sort, page, size)
		}
	case ListRecent:
		l.Kind, l.Name = "飞牛音乐", "最近播放"
		l.HeaderKind = "播放记录"
		fetch = (*fnmusic.Client).RecentPage
	case ListAlbum:
		l.Kind, l.HeaderKind = fnmusic.KindAlbum, "专辑"
		fetch = func(client *fnmusic.Client, page, size int) ([]pluginapi.Track, int, error) {
			return client.AlbumPage(spec.Ref, l.Spec.Sort, page, size)
		}
	case ListArtist:
		l.Kind, l.HeaderKind = fnmusic.KindArtist, "歌手"
		fetch = func(client *fnmusic.Client, page, size int) ([]pluginapi.Track, int, error) {
			return client.ArtistPage(spec.Ref, l.Spec.Sort, page, size)
		}
	case ListPlaylist:
		l.Kind, l.HeaderKind = "歌单", "歌单"
		fetch = func(client *fnmusic.Client, page, size int) ([]pluginapi.Track, int, error) {
			return client.PlaylistPage(spec.Ref, page, size)
		}
	case ListGenre:
		l.Kind, l.HeaderKind = fnmusic.KindGenre, "风格"
		fetch = func(client *fnmusic.Client, page, size int) ([]pluginapi.Track, int, error) {
			res, err := client.GenreTracksPage(spec.Ref, l.Spec.Sort, page, size)
			if err != nil {
				return nil, 0, err
			}
			tracks := make([]pluginapi.Track, len(res.List))
			for i, item := range res.List {
				tracks[i] = item.Track
			}
			return tracks, res.Total, nil
		}
	case ListSearch:
		l.Kind, l.HeaderKind, l.Name = "搜索", "搜索", spec.Query
		fetch = func(client *fnmusic.Client, page, size int) ([]pluginapi.Track, int, error) {
			return client.SearchPage(spec.Query, page, size)
		}
	default:
		return nil
	}
	l.Pages = c.trackPager(catalogPage(c, func(client *fnmusic.Client, page int) ([]pluginapi.Track, int, error) {
		return fetch(client, page, PageSize)
	}))
	return l
}

func listID(spec Spec) int64 {
	switch spec.Kind {
	case ListLibrary:
		return fnmusic.LibraryID
	case ListFavorites:
		return fnmusic.FavoritesID
	case ListRecent:
		return fnmusic.RecentID
	case ListAlbum:
		return fnmusic.AlbumID(spec.Ref)
	case ListArtist:
		return fnmusic.ArtistID(spec.Ref)
	case ListPlaylist:
		return fnmusic.ID(spec.Ref)
	case ListGenre:
		return fnmusic.GenreID(spec.Ref)
	default:
		return -3
	}
}

// ListFor returns or constructs the list represented by id.
func (c *Catalog) ListFor(id int64) *List {
	return c.ListForRoute(id, "", "")
}

// ListForRoute resolves a card using the provider-neutral route metadata.
// Cards carry a stable app ID for rendering, while Ref preserves the service
// identifier needed when the card's metadata has not reached Catalog.Meta
// yet (for example immediately after a reconnect).
func (c *Catalog) ListForRoute(id int64, kind, ref string) *List {
	if c == nil {
		return nil
	}
	if l := c.Lists[id]; l != nil {
		l.SetSort(c.listSort(l))
		return l
	}
	var spec Spec
	var meta pluginapi.Playlist
	switch id {
	case fnmusic.LibraryID:
		spec = Spec{Kind: ListLibrary, Sort: c.LibrarySort}
	case fnmusic.FavoritesID:
		spec = Spec{Kind: ListFavorites, Sort: c.FavoriteSort}
	case fnmusic.RecentID:
		spec = Spec{Kind: ListRecent}
	default:
		var ok bool
		meta, ok = c.Meta[id]
		if !ok && ref != "" {
			meta = pluginapi.Playlist{ID: id, Ref: ref, Kind: kind}
			ok = true
		}
		if !ok && c.Session != nil {
			for _, pl := range c.Session.Playlists() {
				if pl.ID == id {
					meta, ok = pl, true
					break
				}
			}
		}
		if !ok {
			return nil
		}
		// Album and artist routes require the provider GUID. Never turn a
		// partially resolved card into a playlist request: that endpoint can
		// return an unrelated list and was the source of the card crash path.
		if meta.Ref == "" {
			return nil
		}
		spec.Ref = meta.Ref
		switch meta.Kind {
		case fnmusic.KindAlbum:
			spec.Kind, spec.Sort = ListAlbum, c.AlbumDetailSort
		case fnmusic.KindArtist:
			spec.Kind, spec.Sort = ListArtist, c.ArtistTrackSort
		case fnmusic.KindGenre:
			spec.Kind, spec.Sort = ListGenre, c.GenreTrackSort
		default:
			spec.Kind = ListPlaylist
		}
	}
	l := c.NewList(spec)
	if l == nil {
		return nil
	}
	if meta.Name != "" {
		l.Name, l.Sub, l.CoverURL = meta.Name, meta.Sub, meta.Cover()
	}
	c.Lists[id] = l
	return l
}

func (c *Catalog) listSort(l *List) string {
	if l == nil {
		return ""
	}
	switch l.Spec.Kind {
	case ListLibrary:
		return fnmusic.LibrarySortPolicy.Normalize(c.LibrarySort)
	case ListFavorites:
		return fnmusic.FavoriteSortPolicy.Normalize(c.FavoriteSort)
	case ListAlbum:
		return fnmusic.AlbumTrackSortPolicy.Normalize(c.AlbumDetailSort)
	case ListArtist:
		return fnmusic.ArtistTrackSortPolicy.Normalize(c.ArtistTrackSort)
	case ListGenre:
		return fnmusic.GenreTrackSortPolicy.Normalize(c.GenreTrackSort)
	default:
		return ""
	}
}

// FavoriteChanged drops or updates the cached favorites list after a
// successful like change.
func (c *Catalog) FavoriteChanged(id int64, liked bool) {
	if c == nil {
		return
	}
	if !liked && c.RemoveFavorite(id) {
		return
	}
	c.Refresh(fnmusic.FavoritesID)
}

// RemoveFavorite takes a song out of the loaded favorites list without
// refetching every page.
func (c *Catalog) RemoveFavorite(id int64) bool {
	if c == nil {
		return false
	}
	l := c.Lists[fnmusic.FavoritesID]
	if l == nil || l.Pages == nil {
		return false
	}
	index := l.Pages.IndexOf(func(tr *pluginapi.Track) bool { return tr != nil && tr.ID == id })
	if index < 0 {
		return false
	}
	l.Pages.RemoveAt(index)
	return true
}

// Refresh drops one list so the next view asks the service again.
func (c *Catalog) Refresh(id int64) {
	if c == nil {
		return
	}
	delete(c.Lists, id)
}

// Search creates the song, album and artist catalogs for one query.
func (c *Catalog) Search(query string) error {
	if c == nil || c.Session == nil || c.Session.Client() == nil {
		return errors.New("fnmusic: not connected")
	}
	if _, _, err := c.Session.activeClient(c.scope); err != nil {
		return err
	}
	c.SearchList = c.NewList(Spec{Kind: ListSearch, Query: query})
	c.SearchAlbums = c.cardPager(catalogPage(c, func(client *fnmusic.Client, page int) ([]pluginapi.Playlist, int, error) {
		return client.AlbumsPage(fnmusic.SearchAlbumsPath, "", query, page, CardsPage)
	}))
	c.SearchArtists = c.cardPager(catalogPage(c, func(client *fnmusic.Client, page int) ([]pluginapi.Playlist, int, error) {
		return client.ArtistsPage(fnmusic.SearchArtistsPath, "", query, page, CardsPage)
	}))
	return nil
}

func (c *Catalog) AlbumsList() *paging.Pager[pluginapi.Playlist] {
	if c.Albums == nil && c.Session != nil && c.Session.Client() != nil {
		c.Albums = c.cardPager(catalogPage(c, func(client *fnmusic.Client, page int) ([]pluginapi.Playlist, int, error) {
			return client.AlbumsPage(fnmusic.AlbumsPath, fnmusic.AlbumListSortPolicy.Normalize(c.AlbumSort), "", page, CardsPage)
		}))
	}
	return c.Albums
}

func (c *Catalog) ArtistsList() *paging.Pager[pluginapi.Playlist] {
	if c.Artists == nil && c.Session != nil && c.Session.Client() != nil {
		c.Artists = c.cardPager(catalogPage(c, func(client *fnmusic.Client, page int) ([]pluginapi.Playlist, int, error) {
			return client.ArtistsPage(fnmusic.ArtistsPath, fnmusic.ArtistListSortPolicy.Normalize(c.ArtistSort), "", page, CardsPage)
		}))
	}
	return c.Artists
}

// ArtistAlbumsList is the albums of one artist, in the order of
// ArtistAlbumSort.
func (c *Catalog) ArtistAlbumsList(id int64, ref string) *paging.Pager[pluginapi.Playlist] {
	if c == nil || ref == "" || c.Session == nil || c.Session.Client() == nil {
		return nil
	}
	if c.ArtistAlbums == nil {
		c.ArtistAlbums = map[int64]*paging.Pager[pluginapi.Playlist]{}
	}
	if p := c.ArtistAlbums[id]; p != nil {
		return p
	}
	p := c.cardPager(catalogPage(c, func(client *fnmusic.Client, page int) ([]pluginapi.Playlist, int, error) {
		return client.ArtistAlbumsPage(ref, fnmusic.ArtistAlbumSortPolicy.Normalize(c.ArtistAlbumSort), page, CardsPage)
	}))
	c.ArtistAlbums[id] = p
	return p
}

// GenresList is the style cards, in the order of GenreSort.
func (c *Catalog) GenresList() *paging.Pager[pluginapi.Playlist] {
	if c.Genres == nil && c.Session != nil && c.Session.Client() != nil {
		c.Genres = c.cardPager(catalogPage(c, func(client *fnmusic.Client, page int) ([]pluginapi.Playlist, int, error) {
			res, err := client.GenresPage(fnmusic.ParseGenreSort(c.GenreSort), page, CardsPage)
			if err != nil {
				return nil, 0, err
			}
			cards := make([]pluginapi.Playlist, 0, len(res.List))
			for _, g := range res.List {
				if g.GUID != "" {
					cards = append(cards, g.Card())
				}
			}
			return cards, res.Total, nil
		}))
	}
	return c.Genres
}

func (c *Catalog) activate(ctx context.Context) {
	c.scope = ctx
	for _, list := range c.Lists {
		if list.Pages != nil {
			list.Pages.SetScope(ctx)
		}
	}
	if c.SearchList != nil {
		c.SearchList.Pages.SetScope(ctx)
	}
	for _, p := range c.ArtistAlbums {
		p.SetScope(ctx)
	}
	for _, p := range []*paging.Pager[pluginapi.Playlist]{c.Albums, c.Artists, c.Genres, c.SearchAlbums, c.SearchArtists} {
		if p != nil {
			p.SetScope(ctx)
		}
	}
}

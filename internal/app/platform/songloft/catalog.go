package songloft

import (
	"context"

	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform"
	backend "netease-music-mygo/internal/platform/songloft"
	"netease-music-mygo/internal/platform/paging"
)

const (
	PageSize  = 100
	PageAhead = 2
	CardsPage = 30
)

type ListKind string

const (
	ListLibrary   ListKind = "library"
	ListFavorites ListKind = "favorites"
	ListRecent    ListKind = "recent"
	ListPlaylist  ListKind = "playlist"
	ListAlbum     ListKind = "album"
	ListArtist    ListKind = "artist"
	ListSearch    ListKind = "search"
)

type Spec struct {
	Kind  ListKind
	Ref   string
	Query string
	Sort  string
}

type List struct {
	ID         int64
	Spec       Spec
	Kind       string
	Name       string
	Sub        string
	CoverURL   string
	Shuffle    bool
	Pages      *paging.Pager[netease.Track]
}

func (l *List) SetSort(sort string) {
	if l == nil || l.Spec.Sort == sort {
		return
	}
	l.Spec.Sort = sort
	if l.Pages != nil {
		l.Pages.Reload()
	}
}

func (l *List) View() *appplatform.TrackList {
	if l == nil || l.Pages == nil {
		return nil
	}
	return &appplatform.TrackList{
		ID: l.ID, Kind: l.Kind, Name: l.Name, Sub: l.Sub, Cover: l.Cover(),
		Total: l.Pages.Total, Len: l.Pages.Len, At: l.Pages.At, Around: l.Pages.Around,
		When: l.Pages.When, Retry: func() { l.Pages.Retry() }, PageSize: PageSize,
		Failed: func() bool { return l.Pages.FailedAt(0) }, Error: func() error { return l.Pages.Error() },
	}
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

func (l *List) Refresh() {
	if l == nil || l.Pages == nil {
		return
	}
	l.Pages.Reload()
	l.Pages.At(0)
}

type Catalog struct {
	Session  *State
	Post     func(func())
	OnTracks func([]netease.Track)

	Lists           map[int64]*List
	Meta            map[int64]netease.Playlist
	Recent          *RecentStore
	SearchListVal   *List
	SearchAlbumsP   *paging.Pager[netease.Playlist]
	SearchArtistsP  *paging.Pager[netease.Playlist]
	Albums          *paging.Pager[netease.Playlist]
	Artists         *paging.Pager[netease.Playlist]
	AlbumSort       string
	ArtistSort      string
	LibrarySort     string
	FavoriteSort    string
	AlbumTrackSort  string
	ArtistTrackSort string
}

func NewCatalog(session *State, post func(func())) *Catalog {
	if post == nil {
		post = func(fn func()) { fn() }
	}
	cat := &Catalog{
		Session:         session,
		Post:            post,
		Lists:           map[int64]*List{},
		Meta:            map[int64]netease.Playlist{},
		Recent:          NewRecentStore(),
		AlbumSort:       "count:desc",
		ArtistSort:      "count:desc",
		LibrarySort:     "added_at:desc",
		FavoriteSort:    "added_at:desc",
		AlbumTrackSort:  "title:asc",
		ArtistTrackSort: "added_at:desc",
	}
	cat.OnTracks = func(items []netease.Track) {
		if cat.Session != nil {
			cat.Session.markLiked(items)
		}
	}
	return cat
}

func (c *Catalog) Reset() {
	if c == nil {
		return
	}
	c.Lists = make(map[int64]*List)
	c.Meta = make(map[int64]netease.Playlist)
	c.SearchListVal = nil
	c.SearchAlbumsP = nil
	c.SearchArtistsP = nil
	c.Albums = nil
	c.Artists = nil
}

func (c *Catalog) trackPager(fetch func(page int) ([]netease.Track, int, error)) *paging.Pager[netease.Track] {
	p := paging.New(PageSize, PageAhead, c.Post, fetch)
	p.SetLanded(func(_ int, items []netease.Track) {
		if c.OnTracks != nil {
			c.OnTracks(items)
		}
	})
	return p
}

func (c *Catalog) cardPager(fetch func(page int) ([]netease.Playlist, int, error)) *paging.Pager[netease.Playlist] {
	p := paging.New(CardsPage, 1, c.Post, fetch)
	p.SetLanded(func(_ int, items []netease.Playlist) {
		for _, item := range items {
			c.Meta[item.ID] = item
		}
	})
	return p
}

func (c *Catalog) ListForRoute(id int64, kind, query string) *List {
	name := "歌单"
	ref := query

	if meta, ok := backend.LookupMeta(id); ok {
		if kind == "" {
			kind = meta.Kind
		}
		if ref == "" {
			ref = meta.Ref
		}
		if meta.Name != "" {
			name = meta.Name
		}
	}
	if meta, ok := c.Meta[id]; ok {
		if kind == "" {
			kind = meta.Kind
		}
		if ref == "" {
			ref = meta.Ref
		}
		if meta.Name != "" {
			name = meta.Name
		}
	}

	lk := ListPlaylist
	switch {
	case id == backend.LibraryID || kind == "library":
		id = backend.LibraryID
		lk = ListLibrary
		name = "音乐库"
	case id == backend.FavoritesID || kind == "favorites":
		id = backend.FavoritesID
		lk = ListFavorites
		name = "我喜欢"
	case id == backend.RecentID || kind == "recent":
		id = backend.RecentID
		lk = ListRecent
		name = "最近播放"
	case kind == "album":
		lk = ListAlbum
		if name == "歌单" {
			name = "专辑"
		}
	case kind == "artist":
		lk = ListArtist
		if name == "歌单" {
			name = "歌手"
		}
	case kind == "search":
		lk = ListSearch
		name = "搜索: " + query
	}

	if c.Session != nil && ref == "" {
		for _, pl := range c.Session.Playlists() {
			if pl.ID == id {
				name = pl.Name
				ref = pl.Ref
				break
			}
		}
	}

	return c.ListFor(id, lk, ref, name)
}

func (c *Catalog) listSortForKind(kind ListKind) string {
	if c == nil {
		return "added_at:desc"
	}
	switch kind {
	case ListLibrary:
		if c.LibrarySort != "" {
			return c.LibrarySort
		}
		return "added_at:desc"
	case ListFavorites:
		if c.FavoriteSort != "" {
			return c.FavoriteSort
		}
		return "added_at:desc"
	case ListAlbum:
		if c.AlbumTrackSort != "" {
			return c.AlbumTrackSort
		}
		return "title:asc"
	case ListArtist:
		if c.ArtistTrackSort != "" {
			return c.ArtistTrackSort
		}
		return "added_at:desc"
	default:
		return "added_at:desc"
	}
}

func (c *Catalog) ListFor(id int64, kind ListKind, ref, name string) *List {
	initialSort := c.listSortForKind(kind)
	if l, ok := c.Lists[id]; ok {
		if l.Spec.Ref == "" && ref != "" {
			l.Spec.Ref = ref
		}
		if l.Spec.Sort == "" && initialSort != "" {
			l.Spec.Sort = initialSort
		}
		return l
	}

	if kind == ListRecent || id == backend.RecentID {
		id = backend.RecentID
		kind = ListRecent
		if name == "" || name == "歌单" {
			name = "最近播放"
		}
		spec := Spec{Kind: kind, Ref: ref}
		list := &List{
			ID:   id,
			Spec: spec,
			Kind: string(kind),
			Name: name,
			Sub:  "本地播放记录",
		}
		fetch := func(page int) ([]netease.Track, int, error) {
			if c.Recent == nil {
				return nil, 0, nil
			}
			offset := (page - 1) * PageSize
			tracks, total := c.Recent.Slice(offset, PageSize)
			return tracks, total, nil
		}
		list.Pages = c.trackPager(fetch)
		c.Lists[id] = list
		return list
	}

	spec := Spec{Kind: kind, Ref: ref, Sort: initialSort}
	list := &List{
		ID:   id,
		Spec: spec,
		Kind: string(kind),
		Name: name,
	}

	fetch := func(page int) ([]netease.Track, int, error) {
		provider := c.Session.Provider()
		if provider == nil {
			return nil, 0, backend.ErrNotConnected
		}
		pReq := platform.PageRequest{
			Kind: platform.ListKind(kind),
			Ref:  ref,
			Sort: list.Spec.Sort,
			Page: page - 1,
			Size: PageSize,
		}
		res, err := provider.Page(context.Background(), pReq)
		if err != nil {
			return nil, 0, err
		}
		return res.Tracks, res.Total, nil
	}

	list.Pages = c.trackPager(fetch)
	c.Lists[id] = list
	return list
}

func (c *Catalog) Search(query string) error {
	// 1. Search Songs
	sList := &List{
		ID:   -5000,
		Spec: Spec{Kind: ListSearch, Query: query},
		Kind: "search",
		Name: "搜索: " + query,
	}
	sList.Pages = c.trackPager(func(page int) ([]netease.Track, int, error) {
		provider := c.Session.Provider()
		if provider == nil {
			return nil, 0, backend.ErrNotConnected
		}
		res, err := provider.Search(context.Background(), query, platform.SearchSongs, page-1, PageSize)
		if err != nil {
			return nil, 0, err
		}
		return res.Tracks, res.Total, nil
	})
	sList.Pages.At(0)
	c.SearchListVal = sList

	// 2. Search Albums
	c.SearchAlbumsP = c.cardPager(func(page int) ([]netease.Playlist, int, error) {
		provider := c.Session.Provider()
		if provider == nil {
			return nil, 0, backend.ErrNotConnected
		}
		res, err := provider.Search(context.Background(), query, platform.SearchAlbums, page-1, CardsPage)
		if err != nil {
			return nil, 0, err
		}
		return res.Playlists, res.Total, nil
	})
	c.SearchAlbumsP.At(0)

	// 3. Search Artists
	c.SearchArtistsP = c.cardPager(func(page int) ([]netease.Playlist, int, error) {
		provider := c.Session.Provider()
		if provider == nil {
			return nil, 0, backend.ErrNotConnected
		}
		res, err := provider.Search(context.Background(), query, platform.SearchArtists, page-1, CardsPage)
		if err != nil {
			return nil, 0, err
		}
		return res.Playlists, res.Total, nil
	})
	c.SearchArtistsP.At(0)

	if c.Session == nil || c.Session.Provider() == nil {
		return backend.ErrNotConnected
	}
	return nil
}

func (c *Catalog) SearchList(query string) *List {
	if c.SearchListVal == nil || c.SearchListVal.Spec.Query != query {
		_ = c.Search(query)
	}
	return c.SearchListVal
}

func (c *Catalog) SearchAlbums(query string) *paging.Pager[netease.Playlist] {
	if c.SearchAlbumsP == nil || c.SearchListVal == nil || c.SearchListVal.Spec.Query != query {
		_ = c.Search(query)
	}
	return c.SearchAlbumsP
}

func (c *Catalog) SearchArtists(query string) *paging.Pager[netease.Playlist] {
	if c.SearchArtistsP == nil || c.SearchListVal == nil || c.SearchListVal.Spec.Query != query {
		_ = c.Search(query)
	}
	return c.SearchArtistsP
}

func (c *Catalog) SetAlbumSort(sort string) {
	if c.AlbumSort == sort && c.Albums != nil {
		return
	}
	c.AlbumSort = sort
	c.Albums = nil
}

func (c *Catalog) SetArtistSort(sort string) {
	if c.ArtistSort == sort && c.Artists != nil {
		return
	}
	c.ArtistSort = sort
	c.Artists = nil
}

func (c *Catalog) AlbumsPager() *paging.Pager[netease.Playlist] {
	if c.Albums != nil {
		return c.Albums
	}
	sort := c.AlbumSort
	if sort == "" {
		sort = "count:desc"
		c.AlbumSort = sort
	}
	c.Albums = c.cardPager(func(page int) ([]netease.Playlist, int, error) {
		provider := c.Session.Provider()
		if provider == nil {
			return nil, 0, backend.ErrNotConnected
		}
		res, err := provider.Page(context.Background(), platform.PageRequest{
			Kind: platform.ListAlbums,
			Sort: sort,
			Page: page - 1,
			Size: CardsPage,
		})
		if err != nil {
			return nil, 0, err
		}
		return res.Playlists, res.Total, nil
	})
	return c.Albums
}

func (c *Catalog) ArtistsPager() *paging.Pager[netease.Playlist] {
	if c.Artists != nil {
		return c.Artists
	}
	sort := c.ArtistSort
	if sort == "" {
		sort = "count:desc"
		c.ArtistSort = sort
	}
	c.Artists = c.cardPager(func(page int) ([]netease.Playlist, int, error) {
		provider := c.Session.Provider()
		if provider == nil {
			return nil, 0, backend.ErrNotConnected
		}
		res, err := provider.Page(context.Background(), platform.PageRequest{
			Kind: platform.ListArtists,
			Sort: sort,
			Page: page - 1,
			Size: CardsPage,
		})
		if err != nil {
			return nil, 0, err
		}
		return res.Playlists, res.Total, nil
	})
	return c.Artists
}

// FavoriteChanged updates all loaded track lists and invalidates the favorites list.
func (c *Catalog) FavoriteChanged(id int64, liked bool) {
	if c == nil {
		return
	}
	c.UpdateTrackFav(id, liked)
	if !liked {
		c.RemoveFavorite(id)
	}
	if l := c.Lists[backend.FavoritesID]; l != nil {
		l.Refresh()
	}
	c.Refresh(backend.FavoritesID)
}

// UpdateTrackFav mutates the in-memory track across all loaded lists.
func (c *Catalog) UpdateTrackFav(id int64, liked bool) {
	if c == nil {
		return
	}
	for _, l := range c.Lists {
		if l == nil || l.Pages == nil {
			continue
		}
		idx := l.Pages.IndexOf(func(tr *netease.Track) bool { return tr != nil && tr.ID == id })
		if idx >= 0 {
			if t := l.Pages.At(idx); t != nil {
				t.Fav = liked
			}
		}
	}
}

// RemoveFavorite removes a song from the favorites list immediately.
func (c *Catalog) RemoveFavorite(id int64) bool {
	if c == nil {
		return false
	}
	l := c.Lists[backend.FavoritesID]
	if l == nil || l.Pages == nil {
		return false
	}
	index := l.Pages.IndexOf(func(tr *netease.Track) bool { return tr != nil && tr.ID == id })
	if index < 0 {
		return false
	}
	l.Pages.RemoveAt(index)
	return true
}

// Refresh drops a cached list so it will be fetched anew from the server.
func (c *Catalog) Refresh(id int64) {
	if c == nil {
		return
	}
	delete(c.Lists, id)
}

func (c *Catalog) RemoveRecent(id int64) {
	if c == nil || c.Recent == nil {
		return
	}
	c.Recent.Remove(id)
	if l := c.Lists[backend.RecentID]; l != nil {
		l.Refresh()
	}
}

func (c *Catalog) ClearRecent() {
	if c == nil || c.Recent == nil {
		return
	}
	c.Recent.Clear()
	if l := c.Lists[backend.RecentID]; l != nil {
		l.Refresh()
	}
}

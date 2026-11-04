package subsonic

import (
	"context"

	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform"
	backend "netease-music-mygo/internal/platform/subsonic"
	"netease-music-mygo/internal/platform/paging"
)

const (
	PageSize  = 100
	PageAhead = 2
	CardsPage = 30
)

// ListKind describes a Subsonic catalog list.
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

// Spec identifies a list query.
type Spec struct {
	Kind  ListKind
	Ref   string
	Query string
	Sort  string
}

// List is a lazily paged list of tracks.
type List struct {
	ID       int64
	Spec     Spec
	Kind     string
	Name     string
	Sub      string
	CoverURL string
	Pages    *paging.Pager[netease.Track]
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

// Catalog manages cached pagers for lists and card grids.
type Catalog struct {
	Session  *State
	Post     func(func())
	OnTracks func([]netease.Track)

	Lists         map[int64]*List
	Meta          map[int64]netease.Playlist
	Recent        *RecentStore
	SearchList    *List
	SearchAlbums  *paging.Pager[netease.Playlist]
	SearchArtists *paging.Pager[netease.Playlist]
	Albums        *paging.Pager[netease.Playlist]
	Artists       *paging.Pager[netease.Playlist]

	AlbumSort string
}

func NewCatalog(session *State, post func(func())) *Catalog {
	if post == nil {
		post = func(fn func()) { fn() }
	}
	var profID string
	if session != nil {
		profID = string(session.Profile().ID)
	}
	cat := &Catalog{
		Session:   session,
		Post:      post,
		Lists:     map[int64]*List{},
		Meta:      map[int64]netease.Playlist{},
		AlbumSort: "newest",
		Recent:    NewRecentStore(profID),
	}
	cat.OnTracks = func(items []netease.Track) {
		if cat.Session != nil {
			cat.Session.RecordTracks(items)
		}
		for _, tr := range items {
			if tr.Album.ID != 0 {
				if meta, ok := backend.LookupMeta(tr.Album.ID); ok {
					cat.Meta[tr.Album.ID] = netease.Playlist{
						ID: tr.Album.ID, Name: tr.Album.Name, CoverURL: tr.Album.PicURL,
						PlatformPlaylist: netease.PlatformPlaylist{Ref: meta.Ref, Kind: "album", Sub: meta.Name},
					}
				}
			}
			for _, ar := range tr.Artists {
				if ar.ID != 0 {
					if meta, ok := backend.LookupMeta(ar.ID); ok {
						cat.Meta[ar.ID] = netease.Playlist{
							ID: ar.ID, Name: ar.Name,
							PlatformPlaylist: netease.PlatformPlaylist{Ref: meta.Ref, Kind: "artist"},
						}
					}
				}
			}
		}
	}
	return cat
}

func (c *Catalog) SetAlbumSort(sort string) {
	if c.AlbumSort == sort && c.Albums != nil {
		return
	}
	c.AlbumSort = sort
	c.Albums = nil
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

// ListForRoute resolves or creates a list based on route parameters.
func (c *Catalog) ListForRoute(id int64, kind, ref string) *List {
	name := "歌单"
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
	case id == backend.LibraryID:
		lk = ListLibrary
		name = "音乐库"
	case id == backend.FavoritesID:
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
	if ref == "" {
		if meta, ok := c.Meta[id]; ok && meta.Ref != "" {
			ref = meta.Ref
			if name == "" || name == "专辑" || name == "歌手" {
				name = meta.Name
			}
		} else if meta, ok := backend.LookupMeta(id); ok && meta.Ref != "" {
			ref = meta.Ref
			if name == "" || name == "专辑" || name == "歌手" {
				name = meta.Name
			}
		}
	}

	l := c.ListFor(id, lk, ref, name)
	if meta, ok := c.Meta[id]; ok {
		l.Sub = meta.Sub
		if l.CoverURL == "" {
			l.CoverURL = meta.CoverURL
		}
	} else if meta, ok := backend.LookupMeta(id); ok {
		if l.CoverURL == "" {
			l.CoverURL = meta.CoverURL
		}
	}
	return l
}

// ListFor returns or instantiates a track list for the given ID and kind.
func (c *Catalog) ListFor(id int64, kind ListKind, ref, name string) *List {
	if l, ok := c.Lists[id]; ok {
		if l.Spec.Ref == "" && ref != "" {
			l.Spec.Ref = ref
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

	spec := Spec{Kind: kind, Ref: ref}
	list := &List{
		ID:   id,
		Spec: spec,
		Kind: string(kind),
		Name: name,
	}

	fetch := func(page int) ([]netease.Track, int, error) {
		provider := c.Session.Provider()
		if provider == nil {
			return nil, 0, backend.ErrSignedOut
		}
		// page is 1-based in paging.Pager
		pReq := platform.PageRequest{
			Kind: platform.ListKind(kind),
			Ref:  ref,
			Page: page - 1,
			Size: PageSize,
		}
		res, err := provider.Page(context.Background(), pReq)
		if err != nil {
			return nil, 0, err
		}
		if kind == ListAlbum && c.Session != nil && provider.Client() != nil {
			if starred, known := provider.Client().AlbumStarredKnown(ref); known {
				c.Session.SetAlbumLikedMemory(ref, starred)
			}
		}
		return res.Tracks, res.Total, nil
	}

	list.Pages = c.trackPager(fetch)
	c.Lists[id] = list
	return list
}

// AlbumsPager returns the album cards pager.
func (c *Catalog) AlbumsPager() *paging.Pager[netease.Playlist] {
	if c.Albums != nil {
		return c.Albums
	}
	c.Albums = c.cardPager(func(page int) ([]netease.Playlist, int, error) {
		provider := c.Session.Provider()
		if provider == nil {
			return nil, 0, backend.ErrSignedOut
		}
		res, err := provider.Page(context.Background(), platform.PageRequest{
			Kind: platform.ListAlbums,
			Sort: c.AlbumSort,
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

// ArtistsPager returns the artist cards pager.
func (c *Catalog) ArtistsPager() *paging.Pager[netease.Playlist] {
	if c.Artists != nil {
		return c.Artists
	}
	c.Artists = c.cardPager(func(page int) ([]netease.Playlist, int, error) {
		provider := c.Session.Provider()
		if provider == nil {
			return nil, 0, backend.ErrSignedOut
		}
		res, err := provider.Page(context.Background(), platform.PageRequest{
			Kind: platform.ListArtists,
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

// Search executes search across songs, albums, and artists.
func (c *Catalog) Search(query string) error {
	provider := c.Session.Provider()
	if provider == nil {
		return backend.ErrSignedOut
	}

	// 1. Search Songs
	sList := &List{
		ID:   -100,
		Spec: Spec{Kind: ListSearch, Query: query},
		Kind: "search",
		Name: "搜索歌曲",
	}
	sList.Pages = c.trackPager(func(page int) ([]netease.Track, int, error) {
		res, err := provider.Search(context.Background(), query, platform.SearchSongs, page-1, PageSize)
		if err != nil {
			return nil, 0, err
		}
		return res.Tracks, res.Total, nil
	})
	c.SearchList = sList

	// 2. Search Albums
	c.SearchAlbums = c.cardPager(func(page int) ([]netease.Playlist, int, error) {
		res, err := provider.Search(context.Background(), query, platform.SearchAlbums, page-1, CardsPage)
		if err != nil {
			return nil, 0, err
		}
		return res.Playlists, res.Total, nil
	})

	// 3. Search Artists
	c.SearchArtists = c.cardPager(func(page int) ([]netease.Playlist, int, error) {
		res, err := provider.Search(context.Background(), query, platform.SearchArtists, page-1, CardsPage)
		if err != nil {
			return nil, 0, err
		}
		return res.Playlists, res.Total, nil
	})

	// Unified SearchAll populates songs, albums, and artists all at once with stable exact totals.
	go func() {
		all, err := provider.SearchAll(context.Background(), query)
		if err != nil {
			return
		}
		c.Post(func() {
			for _, item := range all.Albums {
				c.Meta[item.ID] = item
			}
			for _, item := range all.Artists {
				c.Meta[item.ID] = item
			}
			if c.SearchList != nil && c.SearchList.Spec.Query == query {
				c.SearchList.Pages.Land(0, all.Tracks, len(all.Tracks))
			}
			if c.SearchAlbums != nil {
				c.SearchAlbums.Land(0, all.Albums, len(all.Albums))
			}
			if c.SearchArtists != nil {
				c.SearchArtists.Land(0, all.Artists, len(all.Artists))
			}
		})
	}()

	return nil
}

// Reset clears loaded pagers and cached metadata.
func (c *Catalog) Reset() {
	c.Lists = map[int64]*List{}
	c.Meta = map[int64]netease.Playlist{}
	c.SearchList = nil
	c.SearchAlbums = nil
	c.SearchArtists = nil
	c.Albums = nil
	c.Artists = nil
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

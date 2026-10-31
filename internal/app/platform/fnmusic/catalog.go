package fnmusic

import (
	"errors"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform/fnmusic"
	"netease-music-mygo/internal/platform/paging"
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
	ID       int64
	Spec     Spec
	Kind     string
	Name     string
	Sub      string
	CoverURL string
	Fetch    func(page, size int) ([]netease.Track, int, error)
	Pages    *paging.Pager[netease.Track]
	Shuffle  bool
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

// Catalog owns list and card state for one Feiniu session.
type Catalog struct {
	Session  *State
	Post     func(func())
	OnTracks func([]netease.Track)

	Lists                              map[int64]*List
	Meta                               map[int64]netease.Playlist
	SearchList                         *List
	SearchAlbums, SearchArtists        *paging.Pager[netease.Playlist]
	Albums, Artists                    *paging.Pager[netease.Playlist]
	AlbumSort, ArtistSort, LibrarySort string
}

// NewCatalog creates empty platform catalog state.
func NewCatalog(session *State, post func(func())) *Catalog {
	if post == nil {
		post = func(fn func()) { fn() }
	}
	return &Catalog{Session: session, Post: post, Lists: map[int64]*List{}, Meta: map[int64]netease.Playlist{}}
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

// NewList builds a list from a service-neutral spec.
func (c *Catalog) NewList(spec Spec) *List {
	if c == nil || c.Session == nil || c.Session.Client() == nil {
		return nil
	}
	client := c.Session.Client()
	l := &List{Spec: spec, ID: listID(spec), Fetch: nil}
	switch spec.Kind {
	case ListLibrary:
		sort := spec.Sort
		if sort == "" {
			sort = fnmusic.SortRecent
		}
		l.Kind, l.Name = "飞牛音乐", "音乐库"
		l.Fetch = func(page, size int) ([]netease.Track, int, error) { return client.LibraryPage(sort, page, size) }
	case ListFavorites:
		l.Kind, l.Name = "飞牛音乐", "我喜欢"
		l.Fetch = client.FavoritesPage
	case ListRecent:
		l.Kind, l.Name = "飞牛音乐", "最近播放"
		l.Fetch = client.RecentPage
	case ListAlbum:
		l.Kind = fnmusic.KindAlbum
		l.Fetch = func(page, size int) ([]netease.Track, int, error) { return client.AlbumPage(spec.Ref, page, size) }
	case ListArtist:
		l.Kind = fnmusic.KindArtist
		l.Fetch = func(page, size int) ([]netease.Track, int, error) { return client.ArtistPage(spec.Ref, page, size) }
	case ListPlaylist:
		l.Kind = "歌单"
		l.Fetch = func(page, size int) ([]netease.Track, int, error) { return client.PlaylistPage(spec.Ref, page, size) }
	case ListSearch:
		l.Kind, l.Name = "搜索", spec.Query
		l.Fetch = func(page, size int) ([]netease.Track, int, error) { return client.SearchPage(spec.Query, page, size) }
	default:
		return nil
	}
	l.Pages = c.trackPager(func(page int) ([]netease.Track, int, error) { return l.Fetch(page, PageSize) })
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
	default:
		return -3
	}
}

// ListFor returns or constructs the list represented by id.
func (c *Catalog) ListFor(id int64) *List {
	if c == nil {
		return nil
	}
	if l := c.Lists[id]; l != nil {
		return l
	}
	var spec Spec
	var meta netease.Playlist
	switch id {
	case fnmusic.LibraryID:
		spec = Spec{Kind: ListLibrary, Sort: c.LibrarySort}
	case fnmusic.FavoritesID:
		spec = Spec{Kind: ListFavorites}
	case fnmusic.RecentID:
		spec = Spec{Kind: ListRecent}
	default:
		var ok bool
		meta, ok = c.Meta[id]
		if !ok {
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
		spec.Ref = meta.Ref
		switch meta.Kind {
		case fnmusic.KindAlbum:
			spec.Kind = ListAlbum
		case fnmusic.KindArtist:
			spec.Kind = ListArtist
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
	client := c.Session.Client()
	c.SearchList = c.NewList(Spec{Kind: ListSearch, Query: query})
	c.SearchAlbums = c.cardPager(func(page int) ([]netease.Playlist, int, error) {
		return client.AlbumsPage(fnmusic.SearchAlbumsPath, "", query, page, CardsPage)
	})
	c.SearchArtists = c.cardPager(func(page int) ([]netease.Playlist, int, error) {
		return client.ArtistsPage(fnmusic.SearchArtistsPath, "", query, page, CardsPage)
	})
	return nil
}

func (c *Catalog) AlbumsList() *paging.Pager[netease.Playlist] {
	if c.Albums == nil && c.Session != nil && c.Session.Client() != nil {
		client, sort := c.Session.Client(), c.AlbumSort
		c.Albums = c.cardPager(func(page int) ([]netease.Playlist, int, error) {
			return client.AlbumsPage(fnmusic.AlbumsPath, sort, "", page, CardsPage)
		})
	}
	return c.Albums
}

func (c *Catalog) ArtistsList() *paging.Pager[netease.Playlist] {
	if c.Artists == nil && c.Session != nil && c.Session.Client() != nil {
		client, sort := c.Session.Client(), c.ArtistSort
		c.Artists = c.cardPager(func(page int) ([]netease.Playlist, int, error) {
			return client.ArtistsPage(fnmusic.ArtistsPath, sort, "", page, CardsPage)
		})
	}
	return c.Artists
}

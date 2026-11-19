package subsonic

import (
	"context"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/paging"
	"waygu-music/internal/pluginapi/platform"
	backend "waygu-music/internal/plugins/subsonic/internal/client"
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
	Pages    *paging.Pager[pluginapi.Track]
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
	scope    context.Context
	Post     func(func())
	OnTracks func([]pluginapi.Track)

	Lists         map[int64]*List
	Meta          map[int64]pluginapi.Playlist
	Recent        *RecentStore
	SearchList    *List
	SearchAlbums  *paging.Pager[pluginapi.Playlist]
	SearchArtists *paging.Pager[pluginapi.Playlist]
	Albums        *paging.Pager[pluginapi.Playlist]
	Artists       *paging.Pager[pluginapi.Playlist]

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
		scope:     context.Background(),
		Post:      post,
		Lists:     map[int64]*List{},
		Meta:      map[int64]pluginapi.Playlist{},
		AlbumSort: "newest",
		Recent:    NewRecentStore(profID),
	}
	cat.OnTracks = func(items []pluginapi.Track) {
		if cat.Session != nil {
			cat.Session.RecordTracks(items)
		}
		for _, tr := range items {
			if tr.Album.ID != 0 {
				if meta, ok := backend.LookupMeta(tr.Album.ID); ok {
					cat.Meta[tr.Album.ID] = pluginapi.Playlist{
						ID: tr.Album.ID, Name: tr.Album.Name, CoverURL: tr.Album.PicURL,
						PlatformPlaylist: pluginapi.PlatformPlaylist{Ref: meta.Ref, Kind: "album", Sub: meta.Name},
					}
				}
			}
			for _, ar := range tr.Artists {
				if ar.ID != 0 {
					if meta, ok := backend.LookupMeta(ar.ID); ok {
						cat.Meta[ar.ID] = pluginapi.Playlist{
							ID: ar.ID, Name: ar.Name,
							PlatformPlaylist: pluginapi.PlatformPlaylist{Ref: meta.Ref, Kind: "artist"},
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
		fetch := func(ctx context.Context, page int) ([]pluginapi.Track, int, error) {
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

	fetch := func(ctx context.Context, page int) ([]pluginapi.Track, int, error) {
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
		res, err := provider.Page(ctx, pReq)
		if err != nil {
			return nil, 0, err
		}
		if err := ctx.Err(); err != nil {
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
func (c *Catalog) AlbumsPager() *paging.Pager[pluginapi.Playlist] {
	if c.Albums != nil {
		return c.Albums
	}
	c.Albums = c.cardPager(func(ctx context.Context, page int) ([]pluginapi.Playlist, int, error) {
		provider := c.Session.Provider()
		if provider == nil {
			return nil, 0, backend.ErrSignedOut
		}
		res, err := provider.Page(ctx, platform.PageRequest{
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
func (c *Catalog) ArtistsPager() *paging.Pager[pluginapi.Playlist] {
	if c.Artists != nil {
		return c.Artists
	}
	c.Artists = c.cardPager(func(ctx context.Context, page int) ([]pluginapi.Playlist, int, error) {
		provider := c.Session.Provider()
		if provider == nil {
			return nil, 0, backend.ErrSignedOut
		}
		res, err := provider.Page(ctx, platform.PageRequest{
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
	sList.Pages = c.trackPager(func(ctx context.Context, page int) ([]pluginapi.Track, int, error) {
		res, err := provider.Search(ctx, query, platform.SearchSongs, page-1, PageSize)
		if err != nil {
			return nil, 0, err
		}
		return res.Tracks, res.Total, nil
	})
	c.SearchList = sList

	// 2. Search Albums
	c.SearchAlbums = c.cardPager(func(ctx context.Context, page int) ([]pluginapi.Playlist, int, error) {
		res, err := provider.Search(ctx, query, platform.SearchAlbums, page-1, CardsPage)
		if err != nil {
			return nil, 0, err
		}
		return res.Playlists, res.Total, nil
	})

	// 3. Search Artists
	c.SearchArtists = c.cardPager(func(ctx context.Context, page int) ([]pluginapi.Playlist, int, error) {
		res, err := provider.Search(ctx, query, platform.SearchArtists, page-1, CardsPage)
		if err != nil {
			return nil, 0, err
		}
		return res.Playlists, res.Total, nil
	})

	ctx, albums, artists := c.scope, c.SearchAlbums, c.SearchArtists
	generation := c.Session.Generation()
	// Unified SearchAll populates songs, albums, and artists all at once with stable exact totals.
	go func() {
		all, err := provider.SearchAll(ctx, query)
		if err != nil {
			return
		}
		c.Post(func() {
			if ctx.Err() != nil || generation != c.Session.Generation() || c.SearchList != sList {
				return
			}
			for _, item := range all.Albums {
				c.Meta[item.ID] = item
			}
			for _, item := range all.Artists {
				c.Meta[item.ID] = item
			}
			sList.Pages.Land(0, all.Tracks, len(all.Tracks))
			albums.Land(0, all.Albums, len(all.Albums))
			artists.Land(0, all.Artists, len(all.Artists))
		})
	}()

	return nil
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
	for _, p := range []*paging.Pager[pluginapi.Playlist]{c.Albums, c.Artists, c.SearchAlbums, c.SearchArtists} {
		if p != nil {
			p.SetScope(ctx)
		}
	}
}

// Reset clears loaded pagers and cached metadata.
func (c *Catalog) Reset() {
	c.Lists = map[int64]*List{}
	c.Meta = map[int64]pluginapi.Playlist{}
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

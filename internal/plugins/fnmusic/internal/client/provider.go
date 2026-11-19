package fnmusic

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/network"
	"waygu-music/internal/pluginapi/platform"
)

const PlatformID pluginapi.ID = "fnmusic"

// Feiniu provider implementation.
//
// Keep protocol, authentication and service capabilities here. Translate to
// shared platform values at the provider seam instead of coupling this
// package to app UI or player state.

// Provider wraps one connected Feiniu client without changing its lifetime.
type Provider struct{ client *Client }

var (
	_ platform.Provider       = (*Provider)(nil)
	_ platform.Liker          = (*Provider)(nil)
	_ platform.PlaylistEditor = (*Provider)(nil)
)

// Wrap wraps an existing client. It does not connect or log in.
func Wrap(client *Client) *Provider { return &Provider{client: client} }

// Client returns the wrapped client for transition code.
func (p *Provider) Client() *Client {
	if p == nil {
		return nil
	}
	return p.client
}

func (p *Provider) ID() pluginapi.ID { return PlatformID }
func (p *Provider) Name() string     { return "飞牛音乐" }

func (p *Provider) check(ctx context.Context) error {
	if p == nil || p.client == nil {
		return ErrSignedOut
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// Page maps the common zero-based page to Feiniu's one-based API.
func (p *Provider) Page(ctx context.Context, req platform.PageRequest) (platform.Page, error) {
	if err := p.check(ctx); err != nil {
		return platform.Page{}, err
	}
	if req.Page < 0 || req.Size <= 0 {
		return platform.Page{}, errors.New("platform/fnmusic: invalid page")
	}
	pg, size := req.Page+1, req.Size
	var (
		tracks []pluginapi.Track
		cards  []pluginapi.Playlist
		total  int
		err    error
	)
	switch req.Kind {
	case platform.ListLibrary:
		sort := req.Sort
		if sort == "" {
			sort = SortRecent
		}
		tracks, total, err = p.client.WithContext(ctx).LibraryPage(sort, pg, size)
	case platform.ListFavorites:
		tracks, total, err = p.client.WithContext(ctx).FavoritesPage(req.Sort, pg, size)
	case platform.ListRecent:
		tracks, total, err = p.client.WithContext(ctx).RecentPage(pg, size)
	case platform.ListPlaylist:
		tracks, total, err = p.client.WithContext(ctx).PlaylistPage(req.Ref, pg, size)
	case platform.ListAlbum:
		tracks, total, err = p.client.WithContext(ctx).AlbumPage(req.Ref, req.Sort, pg, size)
	case platform.ListArtist:
		tracks, total, err = p.client.WithContext(ctx).ArtistPage(req.Ref, req.Sort, pg, size)
	case platform.ListAlbums:
		cards, total, err = p.client.WithContext(ctx).AlbumsPage(AlbumsPath, req.Sort, req.Query, pg, size)
	case platform.ListArtists:
		cards, total, err = p.client.WithContext(ctx).ArtistsPage(ArtistsPath, req.Sort, req.Query, pg, size)
	default:
		return platform.Page{}, platform.ErrUnsupported
	}
	if err != nil {
		return platform.Page{}, err
	}
	return platform.Page{Tracks: tracks, Playlists: cards, Total: total}, nil
}

// Search maps common search kinds to Feiniu's separate endpoints.
func (p *Provider) Search(ctx context.Context, query string, kind platform.SearchKind, page, size int) (platform.Page, error) {
	if err := p.check(ctx); err != nil {
		return platform.Page{}, err
	}
	if page < 0 || size <= 0 {
		return platform.Page{}, errors.New("platform/fnmusic: invalid page")
	}
	pg := page + 1
	var (
		tracks []pluginapi.Track
		cards  []pluginapi.Playlist
		total  int
		err    error
	)
	switch kind {
	case platform.SearchSongs:
		tracks, total, err = p.client.WithContext(ctx).SearchPage(query, pg, size)
	case platform.SearchAlbums:
		cards, total, err = p.client.WithContext(ctx).AlbumsPage(SearchAlbumsPath, "", query, pg, size)
	case platform.SearchArtists:
		cards, total, err = p.client.WithContext(ctx).ArtistsPage(SearchArtistsPath, "", query, pg, size)
	default:
		return platform.Page{}, platform.ErrUnsupported
	}
	if err != nil {
		return platform.Page{}, err
	}
	return platform.Page{Tracks: tracks, Playlists: cards, Total: total}, nil
}

func (p *Provider) Stream(ctx context.Context, tr *pluginapi.Track) (*pluginapi.Stream, error) {
	if err := p.check(ctx); err != nil {
		return nil, err
	}
	if tr == nil {
		return nil, errors.New("platform/fnmusic: nil track")
	}
	return p.client.WithContext(ctx).Stream(tr)
}

func (p *Provider) Lyrics(ctx context.Context, tr *pluginapi.Track) (platform.Lyrics, error) {
	if err := p.check(ctx); err != nil {
		return platform.Lyrics{}, err
	}
	if tr == nil {
		return platform.Lyrics{}, errors.New("platform/fnmusic: nil track")
	}
	lrc, err := p.client.WithContext(ctx).Lyrics(tr.Ref)
	return platform.Lyrics{LRC: lrc}, err
}

func (p *Provider) Cover(ctx context.Context, raw string, size int) ([]byte, error) {
	if err := p.check(ctx); err != nil {
		return nil, err
	}
	const prefix = "fnmusic://cover/"
	rest, ok := strings.CutPrefix(raw, prefix)
	if !ok {
		return network.Fetch(raw)
	}
	id, query, _ := strings.Cut(rest, "?")
	id, err := url.PathUnescape(id)
	if err != nil {
		return nil, err
	}
	if values, parseErr := url.ParseQuery(query); parseErr == nil {
		if n, parseErr := strconv.Atoi(values.Get("size")); parseErr == nil && n > 0 {
			size = n
		}
	}
	if size <= 0 {
		size = 640
	}
	return p.client.WithContext(ctx).Cover(id, size)
}

func (p *Provider) SetLiked(ctx context.Context, tr *pluginapi.Track, liked bool) error {
	if err := p.check(ctx); err != nil {
		return err
	}
	if tr == nil {
		return errors.New("platform/fnmusic: nil track")
	}
	return p.client.WithContext(ctx).SetFavorite(tr.Ref, liked)
}

func (p *Provider) CreatePlaylist(ctx context.Context, name string) (pluginapi.Playlist, error) {
	if err := p.check(ctx); err != nil {
		return pluginapi.Playlist{}, err
	}
	return p.client.WithContext(ctx).CreatePlaylist(name)
}

func (p *Provider) RenamePlaylist(ctx context.Context, pl pluginapi.Playlist, name string) error {
	if err := p.check(ctx); err != nil {
		return err
	}
	return p.client.WithContext(ctx).RenamePlaylist(pl.Ref, name)
}

func (p *Provider) DeletePlaylist(ctx context.Context, pl pluginapi.Playlist) error {
	if err := p.check(ctx); err != nil {
		return err
	}
	return p.client.WithContext(ctx).DeletePlaylist(pl.Ref)
}

func (p *Provider) AddToPlaylist(ctx context.Context, pl pluginapi.Playlist, tracks []pluginapi.Track) error {
	if err := p.check(ctx); err != nil {
		return err
	}
	refs := make([]string, len(tracks))
	for i := range tracks {
		refs[i] = tracks[i].Ref
	}
	return p.client.WithContext(ctx).AddToPlaylist(pl.Ref, refs)
}

func (p *Provider) RemoveFromPlaylist(ctx context.Context, pl pluginapi.Playlist, tracks []pluginapi.Track) error {
	if err := p.check(ctx); err != nil {
		return err
	}
	refs := make([]string, len(tracks))
	for i := range tracks {
		refs[i] = tracks[i].Ref
	}
	return p.client.WithContext(ctx).RemoveFromPlaylist(pl.Ref, refs)
}

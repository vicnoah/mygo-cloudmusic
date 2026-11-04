package songloft

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform"
)

var _ platform.Provider = (*Provider)(nil)
var _ platform.Connector = (*Provider)(nil)
var _ platform.Liker = (*Provider)(nil)
var _ platform.PlaylistEditor = (*Provider)(nil)

type Provider struct {
	mu            sync.RWMutex
	cfg           Config
	client        *Client
	favPlaylistID int64
	favMu         sync.RWMutex
	favSongs      map[int64]bool
}

func NewProvider(cfg Config) *Provider {
	return &Provider{
		cfg:           cfg,
		favPlaylistID: 1,
		favSongs:      make(map[int64]bool),
	}
}

func (p *Provider) ID() platform.ID {
	return platform.SongloftID
}

func (p *Provider) Name() string {
	return DefaultProfile.Name
}

func (p *Provider) Config() Config {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cfg
}

func (p *Provider) SetConfig(cfg Config) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg = cfg
}

func (p *Provider) Client() *Client {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.client
}

func (p *Provider) SetClientForTest(cli *Client) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.client = cli
}

func (p *Provider) Connected() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.client != nil && p.client.AccessToken() != ""
}

func (p *Provider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.client = nil
	p.favMu.Lock()
	p.favSongs = make(map[int64]bool)
	p.favMu.Unlock()
	return nil
}

func (p *Provider) Connect(ctx context.Context) error {
	p.mu.RLock()
	cfg := p.cfg
	p.mu.RUnlock()

	if cfg.Address == "" {
		return errors.New("songloft: server address required")
	}

	cli, err := NewClient(cfg.Address, &http.Client{Timeout: 30 * time.Second})
	if err != nil {
		return err
	}

	if err := cli.Login(ctx, cfg.Username, cfg.Password); err != nil {
		return err
	}

	p.mu.Lock()
	p.client = cli
	p.mu.Unlock()

	p.initFavorites(ctx)
	return nil
}

func (p *Provider) initFavorites(ctx context.Context) {
	cli := p.Client()
	if cli == nil {
		return
	}

	// 1. Discover favorite playlist (built-in normal playlist or ID 1)
	favID := int64(1)
	plResp, err := cli.ListPlaylists(ctx, 100, 0)
	if err == nil {
		for _, pl := range plResp.Playlists {
			if pl.Type != "radio" && (pl.ID == 1 || hasString(pl.Labels, "built_in")) {
				favID = pl.ID
				break
			}
		}
	}

	p.mu.Lock()
	p.favPlaylistID = favID
	p.mu.Unlock()

	// 2. Load favorite song IDs into cache
	songsResp, err := cli.GetPlaylistSongs(ctx, favID, "", "", 2000, 0)
	if err == nil {
		p.favMu.Lock()
		p.favSongs = make(map[int64]bool, len(songsResp.Songs))
		for _, s := range songsResp.Songs {
			p.favSongs[s.ID] = true
		}
		p.favMu.Unlock()
	}
}

func hasString(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func (p *Provider) IsFav(songID int64) bool {
	p.favMu.RLock()
	defer p.favMu.RUnlock()
	return p.favSongs[songID]
}

func (p *Provider) isFav(songID int64) bool {
	return p.IsFav(songID)
}

func (p *Provider) FavSongIDs() []int64 {
	p.favMu.RLock()
	defer p.favMu.RUnlock()
	ids := make([]int64, 0, len(p.favSongs))
	for id, fav := range p.favSongs {
		if fav {
			ids = append(ids, id)
		}
	}
	return ids
}

func (p *Provider) setFav(songID int64, fav bool) {
	p.favMu.Lock()
	if fav {
		p.favSongs[songID] = true
	} else {
		delete(p.favSongs, songID)
	}
	p.favMu.Unlock()
}

// SplitSort parses sort string in "field:order" or "field,order" format.
func SplitSort(sort string) (field, order string, ok bool) {
	sort = strings.TrimSpace(sort)
	if sort == "" {
		return "", "", false
	}
	sep := ","
	if strings.Contains(sort, ":") {
		sep = ":"
	}
	parts := strings.Split(sort, sep)
	if len(parts) != 2 || (parts[1] != "asc" && parts[1] != "desc") {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func parseSortOrder(sortStr string) (string, string) {
	if field, order, ok := SplitSort(sortStr); ok {
		return field, order
	}
	if sortStr == "" {
		return "added_at", "desc"
	}
	return sortStr, "desc"
}

func (p *Provider) Page(ctx context.Context, req platform.PageRequest) (platform.Page, error) {
	cli := p.Client()
	if cli == nil {
		return platform.Page{}, ErrNotConnected
	}

	switch req.Kind {
	case platform.ListLibrary:
		sortField, sortOrder := parseSortOrder(req.Sort)
		res, err := cli.ListSongs(ctx, SongFilter{
			Sort:   sortField,
			Order:  sortOrder,
			Limit:  req.Size,
			Offset: req.Page * req.Size,
		})
		if err != nil {
			return platform.Page{}, err
		}
		tracks := make([]netease.Track, 0, len(res.Songs))
		for _, s := range res.Songs {
			if tr, ok := ToTrack(&s, p.isFav(s.ID)); ok {
				tracks = append(tracks, tr)
			}
		}
		return platform.Page{Tracks: tracks, Total: res.Total}, nil

	case platform.ListFavorites:
		p.mu.RLock()
		favID := p.favPlaylistID
		p.mu.RUnlock()
		if favID <= 0 {
			favID = 1
		}

		sortField, sortOrder := parseSortOrder(req.Sort)
		res, err := cli.GetPlaylistSongs(ctx, favID, sortField, sortOrder, req.Size, req.Page*req.Size)
		if err != nil {
			return platform.Page{}, err
		}
		tracks := make([]netease.Track, 0, len(res.Songs))
		for _, s := range res.Songs {
			if tr, ok := ToTrack(&s, true); ok {
				tracks = append(tracks, tr)
			}
		}
		return platform.Page{Tracks: tracks, Total: res.Total}, nil

	case platform.ListAlbums:
		sortField, sortOrder := "count", "desc"
		if req.Sort != "" {
			sortField, sortOrder = parseSortOrder(req.Sort)
		}
		res, err := cli.ListFacets(ctx, "album", req.Query, sortField, sortOrder, req.Size, req.Page*req.Size)
		if err != nil {
			return platform.Page{}, err
		}
		cards := make([]netease.Playlist, 0, len(res.Facets))
		for _, f := range res.Facets {
			cards = append(cards, ToAlbumPlaylist(&f))
		}
		return platform.Page{Playlists: cards, Total: res.Total}, nil

	case platform.ListArtists:
		sortField, sortOrder := "count", "desc"
		if req.Sort != "" {
			sortField, sortOrder = parseSortOrder(req.Sort)
		}
		res, err := cli.ListFacets(ctx, "artist", req.Query, sortField, sortOrder, req.Size, req.Page*req.Size)
		if err != nil {
			return platform.Page{}, err
		}
		cards := make([]netease.Playlist, 0, len(res.Facets))
		for _, f := range res.Facets {
			cards = append(cards, ToArtistPlaylist(&f))
		}
		return platform.Page{Playlists: cards, Total: res.Total}, nil

	case platform.ListAlbum:
		sortField, sortOrder := parseSortOrder(req.Sort)
		res, err := cli.ListSongs(ctx, SongFilter{
			Album:  req.Ref,
			Sort:   sortField,
			Order:  sortOrder,
			Limit:  req.Size,
			Offset: req.Page * req.Size,
		})
		if err != nil {
			return platform.Page{}, err
		}
		tracks := make([]netease.Track, 0, len(res.Songs))
		for _, s := range res.Songs {
			if tr, ok := ToTrack(&s, p.isFav(s.ID)); ok {
				tracks = append(tracks, tr)
			}
		}
		return platform.Page{Tracks: tracks, Total: res.Total}, nil

	case platform.ListArtist:
		sortField, sortOrder := parseSortOrder(req.Sort)
		res, err := cli.ListSongs(ctx, SongFilter{
			Artist: req.Ref,
			Sort:   sortField,
			Order:  sortOrder,
			Limit:  req.Size,
			Offset: req.Page * req.Size,
		})
		if err != nil {
			return platform.Page{}, err
		}
		tracks := make([]netease.Track, 0, len(res.Songs))
		for _, s := range res.Songs {
			if tr, ok := ToTrack(&s, p.isFav(s.ID)); ok {
				tracks = append(tracks, tr)
			}
		}
		return platform.Page{Tracks: tracks, Total: res.Total}, nil

	case platform.ListPlaylist:
		plID, err := strconv.ParseInt(req.Ref, 10, 64)
		if err != nil {
			return platform.Page{}, fmt.Errorf("songloft: invalid playlist id %q: %w", req.Ref, err)
		}
		sortField, sortOrder := parseSortOrder(req.Sort)
		res, err := cli.GetPlaylistSongs(ctx, plID, sortField, sortOrder, req.Size, req.Page*req.Size)
		if err != nil {
			return platform.Page{}, err
		}
		tracks := make([]netease.Track, 0, len(res.Songs))
		for _, s := range res.Songs {
			if tr, ok := ToTrack(&s, p.isFav(s.ID)); ok {
				tracks = append(tracks, tr)
			}
		}
		return platform.Page{Tracks: tracks, Total: res.Total}, nil

	default:
		return platform.Page{}, platform.ErrUnsupported
	}
}

func (p *Provider) Search(ctx context.Context, query string, kind platform.SearchKind, page, size int) (platform.Page, error) {
	cli := p.Client()
	if cli == nil {
		return platform.Page{}, ErrNotConnected
	}

	offset := page * size
	switch kind {
	case platform.SearchSongs:
		res, err := cli.ListSongs(ctx, SongFilter{
			Keyword: query,
			Limit:   size,
			Offset:  offset,
		})
		if err != nil {
			return platform.Page{}, err
		}
		tracks := make([]netease.Track, 0, len(res.Songs))
		for _, s := range res.Songs {
			if tr, ok := ToTrack(&s, p.isFav(s.ID)); ok {
				tracks = append(tracks, tr)
			}
		}
		return platform.Page{Tracks: tracks, Total: res.Total}, nil

	case platform.SearchAlbums:
		res, err := cli.ListFacets(ctx, "album", query, "count", "desc", size, offset)
		if err != nil {
			return platform.Page{}, err
		}
		cards := make([]netease.Playlist, 0, len(res.Facets))
		for _, f := range res.Facets {
			cards = append(cards, ToAlbumPlaylist(&f))
		}
		return platform.Page{Playlists: cards, Total: res.Total}, nil

	case platform.SearchArtists:
		res, err := cli.ListFacets(ctx, "artist", query, "count", "desc", size, offset)
		if err != nil {
			return platform.Page{}, err
		}
		cards := make([]netease.Playlist, 0, len(res.Facets))
		for _, f := range res.Facets {
			cards = append(cards, ToArtistPlaylist(&f))
		}
		return platform.Page{Playlists: cards, Total: res.Total}, nil

	default:
		return platform.Page{}, platform.ErrUnsupported
	}
}

func (p *Provider) Stream(ctx context.Context, tr *netease.Track) (*netease.Stream, error) {
	if tr == nil {
		return nil, errors.New("songloft: nil track")
	}
	cli := p.Client()
	if cli == nil {
		return nil, ErrNotConnected
	}
	songID, err := strconv.ParseInt(tr.Ref, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("songloft: invalid song id %q: %w", tr.Ref, err)
	}

	streamType := tr.Format
	if streamType == "" {
		streamType = "mp3"
	}

	return &netease.Stream{
		URL:  cli.StreamURL(songID),
		Type: streamType,
	}, nil
}

func (p *Provider) Lyrics(ctx context.Context, tr *netease.Track) (platform.Lyrics, error) {
	if tr == nil {
		return platform.Lyrics{}, errors.New("songloft: nil track")
	}
	cli := p.Client()
	if cli == nil {
		return platform.Lyrics{}, ErrNotConnected
	}
	songID, err := strconv.ParseInt(tr.Ref, 10, 64)
	if err != nil {
		return platform.Lyrics{}, fmt.Errorf("songloft: invalid song id %q: %w", tr.Ref, err)
	}

	resp, err := cli.GetLyrics(ctx, songID)
	if err != nil {
		return platform.Lyrics{}, err
	}
	return ToLyrics(resp), nil
}

func (p *Provider) Cover(ctx context.Context, raw string, size int) ([]byte, error) {
	cli := p.Client()
	if cli == nil {
		return nil, ErrNotConnected
	}
	return cli.FetchCover(ctx, raw)
}

func (p *Provider) SetLiked(ctx context.Context, tr *netease.Track, liked bool) error {
	if tr == nil {
		return errors.New("songloft: nil track")
	}
	cli := p.Client()
	if cli == nil {
		return ErrNotConnected
	}
	songID, err := strconv.ParseInt(tr.Ref, 10, 64)
	if err != nil {
		return fmt.Errorf("songloft: invalid song id %q: %w", tr.Ref, err)
	}

	p.mu.RLock()
	favID := p.favPlaylistID
	p.mu.RUnlock()
	if favID <= 0 {
		favID = 1
	}

	if liked {
		if err := cli.AddSongsToPlaylist(ctx, favID, []int64{songID}); err != nil {
			return err
		}
		p.setFav(songID, true)
	} else {
		if err := cli.RemoveSongFromPlaylist(ctx, favID, songID); err != nil {
			return err
		}
		p.setFav(songID, false)
	}
	return nil
}

func (p *Provider) CreatePlaylist(ctx context.Context, name string) (netease.Playlist, error) {
	cli := p.Client()
	if cli == nil {
		return netease.Playlist{}, ErrNotConnected
	}
	pl, err := cli.CreatePlaylist(ctx, name, "")
	if err != nil {
		return netease.Playlist{}, err
	}
	return ToPlaylist(&pl), nil
}

func (p *Provider) RenamePlaylist(ctx context.Context, pl netease.Playlist, name string) error {
	cli := p.Client()
	if cli == nil {
		return ErrNotConnected
	}
	plID, err := strconv.ParseInt(pl.Ref, 10, 64)
	if err != nil {
		return fmt.Errorf("songloft: invalid playlist ref %q: %w", pl.Ref, err)
	}
	return cli.UpdatePlaylist(ctx, plID, name, pl.Description)
}

func (p *Provider) DeletePlaylist(ctx context.Context, pl netease.Playlist) error {
	cli := p.Client()
	if cli == nil {
		return ErrNotConnected
	}
	plID, err := strconv.ParseInt(pl.Ref, 10, 64)
	if err != nil {
		return fmt.Errorf("songloft: invalid playlist ref %q: %w", pl.Ref, err)
	}
	return cli.DeletePlaylist(ctx, plID)
}

func (p *Provider) AddToPlaylist(ctx context.Context, pl netease.Playlist, tracks []netease.Track) error {
	cli := p.Client()
	if cli == nil {
		return ErrNotConnected
	}
	plID, err := strconv.ParseInt(pl.Ref, 10, 64)
	if err != nil {
		return fmt.Errorf("songloft: invalid playlist ref %q: %w", pl.Ref, err)
	}
	songIDs := make([]int64, 0, len(tracks))
	for _, tr := range tracks {
		if sid, err := strconv.ParseInt(tr.Ref, 10, 64); err == nil {
			songIDs = append(songIDs, sid)
		}
	}
	if len(songIDs) == 0 {
		return nil
	}
	return cli.AddSongsToPlaylist(ctx, plID, songIDs)
}

func (p *Provider) RemoveFromPlaylist(ctx context.Context, pl netease.Playlist, tracks []netease.Track) error {
	cli := p.Client()
	if cli == nil {
		return ErrNotConnected
	}
	plID, err := strconv.ParseInt(pl.Ref, 10, 64)
	if err != nil {
		return fmt.Errorf("songloft: invalid playlist ref %q: %w", pl.Ref, err)
	}
	for _, tr := range tracks {
		if sid, err := strconv.ParseInt(tr.Ref, 10, 64); err == nil {
			if err := cli.RemoveSongFromPlaylist(ctx, plID, sid); err != nil {
				return err
			}
		}
	}
	return nil
}

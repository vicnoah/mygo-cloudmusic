package subsonic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	gosubsonic "github.com/supersonic-app/go-subsonic/subsonic"
	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform"
)

// Provider implements the shared platform.Provider, platform.Connector,
// platform.Liker, and platform.PlaylistEditor interfaces for Subsonic.
type Provider struct {
	client  *Client
	profile Profile
}

var (
	_ platform.Provider       = (*Provider)(nil)
	_ platform.Connector      = (*Provider)(nil)
	_ platform.Liker          = (*Provider)(nil)
	_ platform.PlaylistEditor = (*Provider)(nil)
)

// Wrap creates a Provider wrapping a Subsonic Client with optional platform profile.
func Wrap(client *Client, profiles ...Profile) *Provider {
	prof := ProfileSubsonic
	if len(profiles) > 0 && profiles[0].ID != "" {
		prof = profiles[0]
	}
	return &Provider{client: client, profile: prof}
}

// Client returns the underlying Client wrapper.
func (p *Provider) Client() *Client {
	if p == nil {
		return nil
	}
	return p.client
}

func (p *Provider) ID() platform.ID {
	if p != nil && p.profile.ID != "" {
		return p.profile.ID
	}
	return platform.SubsonicID
}

func (p *Provider) Name() string {
	if p != nil && p.profile.Name != "" {
		return p.profile.Name
	}
	return "Subsonic"
}

func (p *Provider) Connect(ctx context.Context) error {
	if p == nil || p.client == nil {
		return ErrSignedOut
	}
	return p.client.Connect(ctx)
}

func (p *Provider) Close() error {
	if p == nil || p.client == nil {
		return nil
	}
	return p.client.Close()
}

func (p *Provider) Connected() bool {
	return p != nil && p.client != nil && p.client.Connected()
}

func (p *Provider) subsonic() (*gosubsonic.Client, error) {
	if p == nil || p.client == nil || !p.client.Connected() {
		return nil, ErrSignedOut
	}
	sc := p.client.Subsonic()
	if sc == nil {
		return nil, ErrSignedOut
	}
	return sc, nil
}

func (p *Provider) toTrack(c *gosubsonic.Child) (netease.Track, bool) {
	return ToTrackWithPlatform(p.ID(), c)
}

func (p *Provider) toAlbumPlaylist(a *gosubsonic.AlbumID3) netease.Playlist {
	return ToAlbumPlaylistWithPlatform(p.ID(), a)
}

func (p *Provider) toArtistPlaylist(ar *gosubsonic.ArtistID3) netease.Playlist {
	return ToArtistPlaylistWithPlatform(p.ID(), ar)
}

func (p *Provider) toPlaylist(pl *gosubsonic.Playlist) netease.Playlist {
	return ToPlaylistWithPlatform(p.ID(), pl)
}

// Page maps platform PageRequest queries to the appropriate Subsonic endpoints.
func (p *Provider) Page(ctx context.Context, req platform.PageRequest) (platform.Page, error) {
	sc, err := p.subsonic()
	if err != nil {
		return platform.Page{}, err
	}
	if req.Page < 0 || req.Size <= 0 {
		return platform.Page{}, errors.New("subsonic: invalid page parameters")
	}

	switch req.Kind {
	case platform.ListLibrary:
		offset := req.Page * req.Size
		params := map[string]string{
			"songCount":  strconv.Itoa(req.Size),
			"songOffset": strconv.Itoa(offset),
		}
		res, err := sc.Search3("", params)
		if err != nil {
			return platform.Page{}, err
		}
		var tracks []netease.Track
		for _, c := range res.Song {
			if t, ok := p.toTrack(c); ok {
				tracks = append(tracks, t)
			}
		}
		total := p.client.SongCount()
		if len(tracks) < req.Size {
			total = offset + len(tracks)
			p.client.SetCounts(total, 0)
		} else if total <= 0 {
			total = offset + len(tracks)
		}
		return platform.Page{Tracks: tracks, Total: total}, nil

	case platform.ListFavorites:
		starred, err := sc.GetStarred2(nil)
		if err != nil {
			return platform.Page{}, err
		}
		total := len(starred.Song)
		start := req.Page * req.Size
		if start > total {
			start = total
		}
		end := start + req.Size
		if end > total {
			end = total
		}
		var tracks []netease.Track
		for _, c := range starred.Song[start:end] {
			if t, ok := p.toTrack(c); ok {
				t.Fav = true
				tracks = append(tracks, t)
			}
		}
		return platform.Page{Tracks: tracks, Total: total}, nil

	case platform.ListRecent:
		// Album list sorted by recently played
		params := map[string]string{
			"size":   strconv.Itoa(req.Size),
			"offset": strconv.Itoa(req.Page * req.Size),
		}
		albums, err := sc.GetAlbumList2("recent", params)
		if err != nil {
			return platform.Page{}, err
		}
		var cards []netease.Playlist
		for _, a := range albums {
			if p.client != nil {
				p.client.SetAlbumStarred(a.ID, !a.Starred.IsZero())
			}
			cards = append(cards, p.toAlbumPlaylist(a))
		}
		total := req.Page*req.Size + len(cards)
		if len(cards) == req.Size {
			total = (req.Page + 2) * req.Size
		}
		return platform.Page{Playlists: cards, Total: total}, nil

	case platform.ListPlaylist:
		pl, err := sc.GetPlaylist(req.Ref)
		if err != nil {
			return platform.Page{}, err
		}
		total := len(pl.Entry)
		start := req.Page * req.Size
		if start > total {
			start = total
		}
		end := start + req.Size
		if end > total {
			end = total
		}
		var tracks []netease.Track
		for _, c := range pl.Entry[start:end] {
			if t, ok := p.toTrack(c); ok {
				tracks = append(tracks, t)
			}
		}
		return platform.Page{Tracks: tracks, Total: total}, nil

	case platform.ListAlbum:
		album, err := sc.GetAlbum(req.Ref)
		if err != nil {
			return platform.Page{}, err
		}
		if p.client != nil {
			p.client.SetAlbumStarred(req.Ref, !album.Starred.IsZero())
		}
		total := len(album.Song)
		start := req.Page * req.Size
		if start > total {
			start = total
		}
		end := start + req.Size
		if end > total {
			end = total
		}
		var tracks []netease.Track
		for _, c := range album.Song[start:end] {
			if t, ok := p.toTrack(c); ok {
				tracks = append(tracks, t)
			}
		}
		return platform.Page{Tracks: tracks, Total: total}, nil

	case platform.ListArtist:
		artist, err := sc.GetArtist(req.Ref)
		if err != nil {
			return platform.Page{}, err
		}
		var allTracks []netease.Track
		for _, alb := range artist.Album {
			albData, err := sc.GetAlbum(alb.ID)
			if err != nil {
				continue
			}
			for _, c := range albData.Song {
				if t, ok := p.toTrack(c); ok {
					allTracks = append(allTracks, t)
				}
			}
		}
		if len(allTracks) == 0 && artist.Name != "" {
			if top, err := sc.GetTopSongs(artist.Name, map[string]string{"count": "100"}); err == nil && len(top) > 0 {
				for _, c := range top {
					if t, ok := p.toTrack(c); ok {
						allTracks = append(allTracks, t)
					}
				}
			}
		}
		if len(allTracks) == 0 && artist.Name != "" {
			if res, err := sc.Search3(artist.Name, map[string]string{"songCount": "200"}); err == nil && len(res.Song) > 0 {
				for _, c := range res.Song {
					if t, ok := p.toTrack(c); ok {
						allTracks = append(allTracks, t)
					}
				}
			}
		}
		total := len(allTracks)
		start := req.Page * req.Size
		if start > total {
			start = total
		}
		end := start + req.Size
		if end > total {
			end = total
		}
		return platform.Page{Tracks: allTracks[start:end], Total: total}, nil

	case platform.ListAlbums:
		sortType := "newest"
		if req.Sort != "" {
			sortType = req.Sort
		}
		params := map[string]string{
			"size":   strconv.Itoa(req.Size),
			"offset": strconv.Itoa(req.Page * req.Size),
		}
		if sortType == "byYear" {
			params["fromYear"] = "1900"
			params["toYear"] = "3000"
		}
		albums, err := sc.GetAlbumList2(sortType, params)
		if err != nil {
			return platform.Page{}, err
		}
		var cards []netease.Playlist
		for _, a := range albums {
			if p.client != nil {
				p.client.SetAlbumStarred(a.ID, !a.Starred.IsZero())
			}
			cards = append(cards, p.toAlbumPlaylist(a))
		}
		total := p.client.AlbumCountForSort(sc, sortType)
		if len(cards) < req.Size {
			total = req.Page*req.Size + len(cards)
			p.client.SetSortAlbumCount(sortType, total)
		} else if total <= 0 {
			total = req.Page*req.Size + len(cards)
		}
		return platform.Page{Playlists: cards, Total: total}, nil

	case platform.ListArtists:
		artists, err := sc.GetArtists(nil)
		if err != nil {
			return platform.Page{}, err
		}
		var all []*gosubsonic.ArtistID3
		for _, idx := range artists.Index {
			all = append(all, idx.Artist...)
		}
		total := len(all)
		start := req.Page * req.Size
		if start > total {
			start = total
		}
		end := start + req.Size
		if end > total {
			end = total
		}
		var cards []netease.Playlist
		for _, a := range all[start:end] {
			cards = append(cards, p.toArtistPlaylist(a))
		}
		return platform.Page{Playlists: cards, Total: total}, nil

	default:
		return platform.Page{}, platform.ErrUnsupported
	}
}

// SearchResultAll collects song, album, and artist results for a single search.
type SearchResultAll struct {
	Tracks  []netease.Track
	Albums  []netease.Playlist
	Artists []netease.Playlist
}

// SearchAll searches songs, albums, and artists in one Subsonic API call.
func (p *Provider) SearchAll(ctx context.Context, query string) (SearchResultAll, error) {
	sc, err := p.subsonic()
	if err != nil {
		return SearchResultAll{}, err
	}
	params := map[string]string{
		"songCount":   "500",
		"albumCount":  "100",
		"artistCount": "100",
	}
	res, err := sc.Search3(query, params)
	if err != nil {
		return SearchResultAll{}, err
	}
	var out SearchResultAll
	for _, c := range res.Song {
		if t, ok := p.toTrack(c); ok {
			out.Tracks = append(out.Tracks, t)
		}
	}
	for _, a := range res.Album {
		out.Albums = append(out.Albums, p.toAlbumPlaylist(a))
	}
	for _, ar := range res.Artist {
		out.Artists = append(out.Artists, p.toArtistPlaylist(ar))
	}
	return out, nil
}

// Search queries songs, albums, or artists matching the search string.
func (p *Provider) Search(ctx context.Context, query string, kind platform.SearchKind, page, size int) (platform.Page, error) {
	sc, err := p.subsonic()
	if err != nil {
		return platform.Page{}, err
	}
	if page < 0 || size <= 0 {
		return platform.Page{}, errors.New("subsonic: invalid page parameters")
	}

	offset := page * size
	params := map[string]string{}

	switch kind {
	case platform.SearchSongs:
		params["songCount"] = strconv.Itoa(size)
		params["songOffset"] = strconv.Itoa(offset)
		res, err := sc.Search3(query, params)
		if err != nil {
			return platform.Page{}, err
		}
		var tracks []netease.Track
		for _, c := range res.Song {
			if t, ok := p.toTrack(c); ok {
				tracks = append(tracks, t)
			}
		}
		total := offset + len(tracks)
		return platform.Page{Tracks: tracks, Total: total}, nil

	case platform.SearchAlbums:
		params["albumCount"] = strconv.Itoa(size)
		params["albumOffset"] = strconv.Itoa(offset)
		res, err := sc.Search3(query, params)
		if err != nil {
			return platform.Page{}, err
		}
		var cards []netease.Playlist
		for _, a := range res.Album {
			cards = append(cards, p.toAlbumPlaylist(a))
		}
		total := offset + len(cards)
		return platform.Page{Playlists: cards, Total: total}, nil

	case platform.SearchArtists:
		params["artistCount"] = strconv.Itoa(size)
		params["artistOffset"] = strconv.Itoa(offset)
		res, err := sc.Search3(query, params)
		if err != nil {
			return platform.Page{}, err
		}
		var cards []netease.Playlist
		for _, a := range res.Artist {
			cards = append(cards, p.toArtistPlaylist(a))
		}
		total := offset + len(cards)
		return platform.Page{Playlists: cards, Total: total}, nil

	default:
		return platform.Page{}, platform.ErrUnsupported
	}
}

// Stream generates a direct streaming URL with authentication credentials.
func (p *Provider) Stream(ctx context.Context, tr *netease.Track) (*netease.Stream, error) {
	if tr == nil {
		return nil, errors.New("subsonic: nil track")
	}
	sc, err := p.subsonic()
	if err != nil {
		return nil, err
	}
	params := map[string]string{}
	streamType := tr.Format
	if streamType == "" {
		streamType = "mp3"
	}

	mode := "auto"
	bitrate := 0
	if p.client != nil {
		mode = p.client.TranscodeMode()
		bitrate = p.client.MaxBitRate()
	}

	needTranscode := false
	switch mode {
	case "always":
		needTranscode = true
	case "native":
		// Only transcode if client player cannot decode format directly
		if tr.Format != "mp3" && tr.Format != "flac" {
			needTranscode = true
		}
	default: // "auto"
		if tr.Format != "mp3" && tr.Format != "flac" {
			needTranscode = true
		}
	}

	if needTranscode {
		params["format"] = "mp3"
		streamType = "mp3"
	}

	if bitrate > 0 {
		params["maxBitRate"] = strconv.Itoa(bitrate)
		if params["format"] == "" {
			params["format"] = "mp3"
			streamType = "mp3"
		}
	}

	u, err := sc.GetStreamURL(tr.Ref, params)
	if err != nil {
		return nil, err
	}
	return &netease.Stream{
		URL:  u.String(),
		Type: streamType,
	}, nil
}

var (
	defaultThumbOnce sync.Once
	defaultThumbData []byte
)

func defaultAlbumThumbnail(raw []byte) []byte {
	defaultThumbOnce.Do(func() {
		src, err := webp.Decode(bytes.NewReader(raw))
		if err != nil {
			defaultThumbData = raw
			return
		}
		// Scale down high-resolution 1024x1024 WebP to crisp 600x600 JPEG with Catmull-Rom bicubic filter.
		// 600x600 matches MyGo's standard artwork resolution (cards, headers, and Now Playing),
		// retaining high sharpness on HiDPI/Retina screens while decoding ~15x faster than pure-Go WebP.
		const target = 600
		dst := image.NewRGBA(image.Rect(0, 0, target, target))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 92}); err == nil {
			defaultThumbData = buf.Bytes()
		} else {
			defaultThumbData = raw
		}
	})
	if len(defaultThumbData) > 0 {
		return defaultThumbData
	}
	return raw
}

// Cover retrieves the album art or artist image as raw image bytes.
func (p *Provider) Cover(ctx context.Context, raw string, size int) ([]byte, error) {
	const prefix = "subsonic://cover/"
	rest, ok := strings.CutPrefix(raw, prefix)
	if !ok {
		return netease.Fetch(raw)
	}
	coverID, _, _ := strings.Cut(rest, "?")
	coverID, err := url.PathUnescape(coverID)
	if err != nil {
		return nil, err
	}
	if coverID == "" {
		return nil, errors.New("subsonic: empty cover id")
	}
	sc, err := p.subsonic()
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("id", coverID)
	// Always request standard size 300 from Subsonic/Navidrome.
	// Navidrome pre-caches 300px thumbnails during library scan (40ms).
	// Odd sizes like 96 or 636 force on-the-fly resampling taking 1.4s per image!
	params.Set("size", "300")

	resp, err := sc.Request("GET", "getCoverArt", params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("subsonic: cover art returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(data, []byte("<subsonic-response")) || bytes.HasPrefix(data, []byte("<?xml")) {
		return nil, errors.New("subsonic: artwork not found")
	}

	// Optimize Navidrome's default placeholder:
	// Navidrome serves a 69228-byte unscaled 1024x1024 WebP for all albums without embedded art.
	// Decoding hundreds of 1024x1024 WebP images takes 37.5ms each in software Go, exhausting the 64MB cache.
	if len(data) == 69228 && bytes.HasPrefix(data, []byte("RIFFd\x0e\x01\x00WEBPVP8X")) {
		return defaultAlbumThumbnail(data), nil
	}

	return data, nil
}

// SetLiked stars or unstars a track on the server.
func (p *Provider) SetLiked(ctx context.Context, tr *netease.Track, liked bool) error {
	if tr == nil {
		return errors.New("subsonic: nil track")
	}
	sc, err := p.subsonic()
	if err != nil {
		return err
	}
	params := gosubsonic.StarParameters{
		SongIDs: []string{tr.Ref},
	}
	if liked {
		return sc.Star(params)
	}
	return sc.Unstar(params)
}

// SetAlbumLiked stars or unstars an album on the server.
func (p *Provider) SetAlbumLiked(ctx context.Context, albumRef string, liked bool) error {
	if albumRef == "" {
		return errors.New("subsonic: empty album ref")
	}
	sc, err := p.subsonic()
	if err != nil {
		return err
	}
	params := gosubsonic.StarParameters{
		AlbumIDs: []string{albumRef},
	}
	var opErr error
	if liked {
		opErr = sc.Star(params)
	} else {
		opErr = sc.Unstar(params)
	}
	if opErr == nil && p.client != nil {
		p.client.SetAlbumStarred(albumRef, liked)
		p.client.InvalidateSortCount("starred")
	}
	return opErr
}

// CreatePlaylist creates a new empty playlist on the Subsonic server.
func (p *Provider) CreatePlaylist(ctx context.Context, name string) (netease.Playlist, error) {
	sc, err := p.subsonic()
	if err != nil {
		return netease.Playlist{}, err
	}
	pl, err := sc.CreatePlaylist(map[string]string{"name": name})
	if err != nil {
		return netease.Playlist{}, err
	}
	return p.toPlaylist(pl), nil
}

// RenamePlaylist renames an existing playlist.
func (p *Provider) RenamePlaylist(ctx context.Context, pl netease.Playlist, name string) error {
	sc, err := p.subsonic()
	if err != nil {
		return err
	}
	return sc.UpdatePlaylist(pl.Ref, map[string]string{"name": name})
}

// DeletePlaylist deletes a playlist by its ID.
func (p *Provider) DeletePlaylist(ctx context.Context, pl netease.Playlist) error {
	sc, err := p.subsonic()
	if err != nil {
		return err
	}
	return sc.DeletePlaylist(pl.Ref)
}

// AddToPlaylist appends tracks to a playlist.
func (p *Provider) AddToPlaylist(ctx context.Context, pl netease.Playlist, tracks []netease.Track) error {
	sc, err := p.subsonic()
	if err != nil {
		return err
	}
	ids := make([]string, len(tracks))
	for i := range tracks {
		ids[i] = tracks[i].Ref
	}
	return sc.UpdatePlaylistTracks(pl.Ref, ids, nil)
}

// RemoveFromPlaylist removes specified tracks from a playlist.
func (p *Provider) RemoveFromPlaylist(ctx context.Context, pl netease.Playlist, tracks []netease.Track) error {
	sc, err := p.subsonic()
	if err != nil {
		return err
	}
	// Fetch the playlist entries to determine indices to remove
	current, err := sc.GetPlaylist(pl.Ref)
	if err != nil {
		return err
	}
	toRemove := make(map[string]bool)
	for _, t := range tracks {
		toRemove[t.Ref] = true
	}
	var indexes []int
	for i, entry := range current.Entry {
		if toRemove[entry.ID] {
			indexes = append(indexes, i)
		}
	}
	if len(indexes) == 0 {
		return nil
	}
	return sc.UpdatePlaylistTracks(pl.Ref, nil, indexes)
}

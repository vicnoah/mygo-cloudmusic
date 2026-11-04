package songloft

import (
	"fmt"
	"hash/fnv"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform"
)

const (
	Source = "songloft"

	// Special playlist IDs for built-in virtual lists.
	LibraryID   int64 = -10
	FavoritesID int64 = -20
	RecentID    int64 = -40

	CoverScheme         = "songloft://cover/"
	PlaylistCoverScheme = "songloft://playlist-cover/"
	FacetCoverScheme    = "songloft://facet-cover/"
)

type Song struct {
	ID         int64   `json:"id"`
	Type       string  `json:"type"`
	Title      string  `json:"title"`
	Artist     string  `json:"artist"`
	Album      string  `json:"album"`
	Year       int     `json:"year"`
	Genre      string  `json:"genre"`
	Duration   float64 `json:"duration"`
	FilePath   string  `json:"file_path"`
	URL        string  `json:"url"`
	CoverURL   string  `json:"cover_url"`
	LyricURL   string  `json:"lyric_url"`
	FileSize   int64   `json:"file_size"`
	Format     string  `json:"format"`
	BitRate    int     `json:"bit_rate"`
	SampleRate int     `json:"sample_rate"`
}

type SongListResponse struct {
	Songs  []Song `json:"songs"`
	Total  int    `json:"total"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

type FacetItem struct {
	Value    string `json:"value"`
	Count    int    `json:"count"`
	CoverURL string `json:"cover_url"`
}

type FacetListResponse struct {
	Facets []FacetItem `json:"facets"`
	Field  string      `json:"field"`
	Total  int         `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}

type Playlist struct {
	ID          int64    `json:"id"`
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	CoverURL    string   `json:"cover_url"`
	Labels      []string `json:"labels"`
	SongCount   int      `json:"song_count"`
}

type PlaylistListResponse struct {
	Playlists []Playlist `json:"playlists"`
	Total     int        `json:"total"`
	Limit     int        `json:"limit"`
	Offset    int        `json:"offset"`
}

type LyricResponse struct {
	Lyric   string `json:"lyric"`
	Tlyric  string `json:"tlyric,omitempty"`
	Rlyric  string `json:"rlyric,omitempty"`
	Lxlyric string `json:"lxlyric,omitempty"`
}

type FolderItem struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	SongCount int    `json:"song_count"`
}

type FolderResponse struct {
	Path         string       `json:"path"`
	ParentPath   string       `json:"parent_path"`
	MusicPath    string       `json:"music_path"`
	Folders      []FolderItem `json:"folders"`
	TotalFolders int          `json:"total_folders"`
	Songs        []Song       `json:"songs"`
	TotalSongs   int          `json:"total_songs"`
}

type FolderPage struct {
	Path       string
	ParentPath string
	Folders    []FolderItem
	Songs      []netease.Track
}

type EntityMeta struct {
	ID       int64
	Ref      string
	Kind     string
	Name     string
	CoverURL string
}

var (
	metaMu    sync.RWMutex
	metaStore = make(map[int64]EntityMeta)
)

// IDWithPlatform calculates a stable negative int64 identifier scoped by platform ID.
func IDWithPlatform(platformID platform.ID, guid string) int64 {
	if guid == "" {
		return 0
	}
	h := fnv.New64a()
	if platformID != "" {
		h.Write([]byte(string(platformID) + ":"))
	}
	h.Write([]byte(guid))
	return -int64(h.Sum64()>>2) - 4000
}

func ID(guid string) int64 {
	return IDWithPlatform(platform.SongloftID, guid)
}

func SongCoverURL(songID int64) string {
	return fmt.Sprintf("%s%d", CoverScheme, songID)
}

func PlaylistCoverURL(playlistID int64) string {
	return fmt.Sprintf("%s%d", PlaylistCoverScheme, playlistID)
}

func FacetCoverURL(facetURL string) string {
	if facetURL == "" {
		return ""
	}
	return FacetCoverScheme + url.PathEscape(facetURL)
}

func RegisterMeta(platformID platform.ID, guid, kind, name, coverURL string) int64 {
	if guid == "" {
		return 0
	}
	id := IDWithPlatform(platformID, guid)
	metaMu.Lock()
	metaStore[id] = EntityMeta{
		ID:       id,
		Ref:      guid,
		Kind:     kind,
		Name:     name,
		CoverURL: coverURL,
	}
	metaMu.Unlock()
	return id
}

func LookupMeta(id int64) (EntityMeta, bool) {
	metaMu.RLock()
	m, ok := metaStore[id]
	metaMu.RUnlock()
	return m, ok
}

func NormalizeFormat(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimPrefix(s, ".")
	switch s {
	case "mpeg", "audio/mpeg", "audio/mp3":
		return "mp3"
	case "mp4a", "audio/mp4", "audio/m4a":
		return "m4a"
	case "audio/flac", "x-flac":
		return "flac"
	case "vorbis", "audio/ogg", "application/ogg":
		return "ogg"
	case "wave", "audio/wav", "audio/x-wav":
		return "wav"
	case "audio/opus":
		return "opus"
	case "audio/aac":
		return "aac"
	}
	return s
}

func ToTrack(s *Song, isFav bool) (netease.Track, bool) {
	if s == nil || s.ID <= 0 {
		return netease.Track{}, false
	}
	title := strings.TrimSpace(s.Title)
	if title == "" {
		title = strings.TrimSpace(s.FilePath)
		if idx := strings.LastIndex(title, "/"); idx >= 0 {
			title = title[idx+1:]
		}
	}
	if title == "" {
		title = "未知歌曲"
	}

	durationMs := int64(s.Duration * 1000)
	format := NormalizeFormat(s.Format)

	guid := strconv.FormatInt(s.ID, 10)
	trackID := RegisterMeta(platform.SongloftID, guid, "song", title, SongCoverURL(s.ID))

	albumName := strings.TrimSpace(s.Album)
	var albumID int64
	if albumName != "" {
		albumID = RegisterMeta(platform.SongloftID, albumName, "album", albumName, SongCoverURL(s.ID))
	}

	tr := netease.Track{
		ID:       trackID,
		Name:     title,
		Album:    netease.Album{ID: albumID, Name: albumName, PicURL: SongCoverURL(s.ID)},
		Fav:      isFav,
		Duration: durationMs,
		Source:   Source,
		Ref:      guid,
		Format:   format,
		Privilege: &netease.Privilege{
			ID: trackID,
			Pl: 1,
		},
	}

	artistName := strings.TrimSpace(s.Artist)
	if artistName != "" {
		arID := RegisterMeta(platform.SongloftID, artistName, "artist", artistName, "")
		tr.Artists = append(tr.Artists, netease.Artist{
			ID:   arID,
			Name: artistName,
		})
	}

	return tr, true
}

func ToAlbumPlaylist(f *FacetItem) netease.Playlist {
	if f == nil {
		return netease.Playlist{}
	}
	cover := FacetCoverURL(f.CoverURL)
	id := RegisterMeta(platform.SongloftID, f.Value, "album", f.Value, cover)
	return netease.Playlist{
		PlatformPlaylist: netease.PlatformPlaylist{Ref: f.Value, Kind: "album"},
		ID:               id,
		Name:             f.Value,
		CoverURL:         cover,
		TrackCount:       f.Count,
	}
}

func ToArtistPlaylist(f *FacetItem) netease.Playlist {
	if f == nil {
		return netease.Playlist{}
	}
	cover := FacetCoverURL(f.CoverURL)
	id := RegisterMeta(platform.SongloftID, f.Value, "artist", f.Value, cover)
	desc := ""
	if f.Count > 0 {
		desc = fmt.Sprintf("%d 首歌曲", f.Count)
	}
	return netease.Playlist{
		PlatformPlaylist: netease.PlatformPlaylist{Ref: f.Value, Kind: "artist"},
		ID:               id,
		Name:             f.Value,
		CoverURL:         cover,
		TrackCount:       f.Count,
		Description:      desc,
	}
}

func ToPlaylist(pl *Playlist) netease.Playlist {
	if pl == nil {
		return netease.Playlist{}
	}
	guid := strconv.FormatInt(pl.ID, 10)
	cover := PlaylistCoverURL(pl.ID)
	id := RegisterMeta(platform.SongloftID, guid, "playlist", pl.Name, cover)
	return netease.Playlist{
		PlatformPlaylist: netease.PlatformPlaylist{Ref: guid, Kind: "playlist"},
		ID:               id,
		Name:             pl.Name,
		CoverURL:         cover,
		TrackCount:       pl.SongCount,
		Description:      pl.Description,
	}
}

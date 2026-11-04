package subsonic

import (
	"fmt"
	"hash/fnv"
	"net/url"
	"strings"
	"sync"

	gosubsonic "github.com/supersonic-app/go-subsonic/subsonic"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform"
)

// Profile configures the branding and hints for a Subsonic-compatible platform.
type Profile struct {
	ID          platform.ID
	Name        string
	DefaultHint string
	Description string
	DefaultPort string
}

var (
	ProfileNavidrome = Profile{
		ID:          platform.NavidromeID,
		Name:        "Navidrome",
		DefaultHint: "例如 http://192.168.1.10:4533",
		Description: "开源现代音乐服务器 (OpenSubsonic)",
		DefaultPort: "4533",
	}
	ProfileJellyfin = Profile{
		ID:          platform.JellyfinID,
		Name:        "Jellyfin",
		DefaultHint: "例如 http://192.168.1.10:8096",
		Description: "家庭媒体中心 (需启用 Subsonic 插件)",
		DefaultPort: "8096",
	}
	ProfileGonic = Profile{
		ID:          platform.GonicID,
		Name:        "Gonic",
		DefaultHint: "例如 http://192.168.1.10:4747",
		Description: "轻量级自建音乐服务",
		DefaultPort: "4747",
	}
	ProfileSubsonic = Profile{
		ID:          platform.SubsonicID,
		Name:        "Subsonic 兼容",
		DefaultHint: "例如 http://192.168.1.10:4040",
		Description: "通用 Subsonic / OpenSubsonic 兼容平台",
		DefaultPort: "4040",
	}
)

// Source names generic Subsonic tracks in the shared player and cache.
const Source = "subsonic"

// Special playlist IDs for built-in virtual lists.
const (
	LibraryID   int64 = -10
	FavoritesID int64 = -20
	RecentID    int64 = -40
)

// CoverScheme is the URI scheme for Subsonic cover art.
const CoverScheme = "subsonic://cover/"

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
	return -int64(h.Sum64()>>2) - 3000
}

// ID calculates a stable negative int64 identifier for Subsonic GUIDs.
func ID(guid string) int64 {
	return IDWithPlatform(platform.SubsonicID, guid)
}

// CoverURL formats a cover art URI with the subsonic:// scheme.
func CoverURL(coverArtID string) string {
	if coverArtID == "" {
		return ""
	}
	return CoverScheme + url.PathEscape(coverArtID)
}

// EntityMeta keeps the Subsonic GUID and kind for a hashed int64 identifier.
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

// RegisterMeta associates a hashed int64 ID with its original Subsonic GUID and kind.
func RegisterMeta(platformID platform.ID, guid, kind, name, coverArtID string) int64 {
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
		CoverURL: CoverURL(coverArtID),
	}
	metaMu.Unlock()
	return id
}

// LookupMeta returns the registered metadata for an int64 identifier.
func LookupMeta(id int64) (EntityMeta, bool) {
	metaMu.RLock()
	m, ok := metaStore[id]
	metaMu.RUnlock()
	return m, ok
}

// NormalizeFormat cleans up MIME types and file extensions into standard audio extensions.
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

// ToTrackWithPlatform converts a Subsonic Child entry into a netease.Track model scoped by platform ID.
func ToTrackWithPlatform(platformID platform.ID, c *gosubsonic.Child) (netease.Track, bool) {
	if c == nil || c.ID == "" {
		return netease.Track{}, false
	}
	title := strings.TrimSpace(c.Title)
	if title == "" {
		title = strings.TrimSpace(c.Path)
		if idx := strings.LastIndex(title, "/"); idx >= 0 {
			title = title[idx+1:]
		}
	}
	if title == "" {
		title = "未知歌曲"
	}

	durationMs := int64(c.Duration) * 1000
	format := NormalizeFormat(c.Suffix)
	if format == "" {
		format = NormalizeFormat(c.ContentType)
	}

	src := string(platformID)
	if src == "" {
		src = Source
	}

	trackID := RegisterMeta(platformID, c.ID, "song", title, c.CoverArt)
	albumID := RegisterMeta(platformID, c.AlbumID, "album", c.Album, c.CoverArt)

	tr := netease.Track{
		ID:       trackID,
		Name:     title,
		Album:    netease.Album{ID: albumID, Name: c.Album, PicURL: CoverURL(c.CoverArt)},
		DiscNo:   c.DiscNumber,
		TrackNo:  c.Track,
		Fav:      !c.Starred.IsZero(),
		Duration: durationMs,
		Source:   src,
		Ref:      c.ID,
		Format:   format,
		Privilege: &netease.Privilege{
			ID: trackID,
			Pl: 1,
		},
	}

	if len(c.Artists) > 0 {
		for _, a := range c.Artists {
			arID := RegisterMeta(platformID, a.ID, "artist", a.Name, "")
			tr.Artists = append(tr.Artists, netease.Artist{
				ID:   arID,
				Name: a.Name,
			})
		}
	} else if c.Artist != "" {
		arID := RegisterMeta(platformID, c.ArtistID, "artist", c.Artist, "")
		tr.Artists = append(tr.Artists, netease.Artist{
			ID:   arID,
			Name: c.Artist,
		})
	}

	return tr, true
}

// ToTrack converts a Subsonic Child entry into a netease.Track model with default Subsonic ID.
func ToTrack(c *gosubsonic.Child) (netease.Track, bool) {
	return ToTrackWithPlatform(platform.SubsonicID, c)
}

// ToAlbumPlaylistWithPlatform converts a Subsonic AlbumID3 into a netease.Playlist card scoped by platform ID.
func ToAlbumPlaylistWithPlatform(platformID platform.ID, a *gosubsonic.AlbumID3) netease.Playlist {
	if a == nil {
		return netease.Playlist{}
	}
	id := RegisterMeta(platformID, a.ID, "album", a.Name, a.CoverArt)
	return netease.Playlist{
		PlatformPlaylist: netease.PlatformPlaylist{Ref: a.ID, Kind: "album", Sub: a.Artist},
		ID:               id,
		Name:             a.Name,
		CoverURL:         CoverURL(a.CoverArt),
		TrackCount:       a.SongCount,
		Description:      a.Artist,
		Creator: struct {
			UserID   int64  `json:"userId"`
			Nickname string `json:"nickname"`
		}{Nickname: a.Artist},
	}
}

// ToAlbumPlaylist converts a Subsonic AlbumID3 into a netease.Playlist card.
func ToAlbumPlaylist(a *gosubsonic.AlbumID3) netease.Playlist {
	return ToAlbumPlaylistWithPlatform(platform.SubsonicID, a)
}

// ToArtistPlaylistWithPlatform converts a Subsonic ArtistID3 into a netease.Playlist card scoped by platform ID.
func ToArtistPlaylistWithPlatform(platformID platform.ID, a *gosubsonic.ArtistID3) netease.Playlist {
	if a == nil {
		return netease.Playlist{}
	}
	id := RegisterMeta(platformID, a.ID, "artist", a.Name, a.CoverArt)
	desc := ""
	if a.AlbumCount > 0 {
		desc = fmt.Sprintf("%d 张专辑", a.AlbumCount)
	}
	return netease.Playlist{
		PlatformPlaylist: netease.PlatformPlaylist{Ref: a.ID, Kind: "artist", Sub: desc},
		ID:               id,
		Name:             a.Name,
		CoverURL:         CoverURL(a.CoverArt),
		TrackCount:       a.AlbumCount,
		Description:      desc,
		Creator: struct {
			UserID   int64  `json:"userId"`
			Nickname string `json:"nickname"`
		}{Nickname: a.Name},
	}
}

// ToArtistPlaylist converts a Subsonic ArtistID3 into a netease.Playlist card.
func ToArtistPlaylist(a *gosubsonic.ArtistID3) netease.Playlist {
	return ToArtistPlaylistWithPlatform(platform.SubsonicID, a)
}

// ToPlaylistWithPlatform converts a Subsonic Playlist into a netease.Playlist card scoped by platform ID.
func ToPlaylistWithPlatform(platformID platform.ID, p *gosubsonic.Playlist) netease.Playlist {
	if p == nil {
		return netease.Playlist{}
	}
	id := RegisterMeta(platformID, p.ID, "playlist", p.Name, p.CoverArt)
	return netease.Playlist{
		PlatformPlaylist: netease.PlatformPlaylist{Ref: p.ID, Kind: "playlist", Sub: p.Comment},
		ID:               id,
		Name:             p.Name,
		CoverURL:         CoverURL(p.CoverArt),
		TrackCount:       p.SongCount,
		Description:      p.Comment,
		Creator: struct {
			UserID   int64  `json:"userId"`
			Nickname string `json:"nickname"`
		}{Nickname: p.Owner},
	}
}

// ToPlaylist converts a Subsonic Playlist into a netease.Playlist card.
func ToPlaylist(p *gosubsonic.Playlist) netease.Playlist {
	return ToPlaylistWithPlatform(platform.SubsonicID, p)
}

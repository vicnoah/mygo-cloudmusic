package qqmusic

import appplatform "waygu-music/internal/pluginapi"

type Page = string

const (
	PageHome        Page = "home"
	PageDaily       Page = "daily"
	PageFM          Page = "fm"
	PageCharts      Page = "charts"
	PagePlaylist    Page = "playlist"
	PageAlbum       Page = "album"
	PageArtist      Page = "artist"
	PageSearch      Page = "search"
	PagePurchased   Page = "purchased"
	PageLiked       Page = "liked"
	PageUser        Page = "user"
	PageLikedAlbums Page = "liked-albums"
)

type Route struct {
	Page  Page
	ID    int64
	Query string
	Kind  string
}
type PlayerPort = appplatform.PlayerPort
type Host = appplatform.Services

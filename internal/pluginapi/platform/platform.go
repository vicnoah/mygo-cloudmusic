package platform

import (
	"context"
	"errors"
	"net/http"

	"waygu-music/internal/pluginapi"
)

// ListKind describes a list a provider can page through. The values are
// deliberately service-neutral; a provider maps them to its own endpoints.
type ListKind string

const (
	ListLibrary   ListKind = "library"
	ListFavorites ListKind = "favorites"
	ListRecent    ListKind = "recent"
	ListPlaylist  ListKind = "playlist"
	ListAlbum     ListKind = "album"
	ListArtist    ListKind = "artist"
	ListAlbums    ListKind = "albums"
	ListArtists   ListKind = "artists"
)

// PageRequest selects one page of a provider list. Page is zero based at the
// seam; adapters convert it when their service uses one based pages.
type PageRequest struct {
	Kind  ListKind
	Ref   string
	Query string
	Sort  string
	Page  int
	Size  int
}

// Page is the common result for song and card lists. A provider fills only
// the fields relevant to the requested kind and always reports Total when
// the service provides it.
type Page struct {
	Tracks    []pluginapi.Track
	Playlists []pluginapi.Playlist
	Total     int
}

// SearchKind selects the result type returned by Search.
type SearchKind ListKind

const (
	SearchSongs   SearchKind = "songs"
	SearchAlbums  SearchKind = "albums"
	SearchArtists SearchKind = "artists"
)

// Lyrics is the common lyric payload. Translation and word-timed lyrics are
// optional because not every service supplies them.
type Lyrics struct {
	LRC         string
	Translation string
	YRC         string
}

// Provider describes the common operations of a music service client.
// Connection, playlist editing, likes and transcoding are optional
// capabilities below, so a caller does not need to type-switch on a platform
// name to discover what it supports.
type Provider interface {
	ID() pluginapi.ID
	Name() string

	Page(context.Context, PageRequest) (Page, error)
	Search(context.Context, string, SearchKind, int, int) (Page, error)
	Stream(context.Context, *pluginapi.Track) (*pluginapi.Stream, error)
	Lyrics(context.Context, *pluginapi.Track) (Lyrics, error)
	Cover(context.Context, string, int) ([]byte, error)
}

// Connector is implemented by providers that have a sign-in or connection
// lifecycle, such as a local music server.
type Connector interface {
	Provider
	Connect(context.Context) error
	Close() error
	Connected() bool
}

// Liker is an optional capability for platforms with a liked-song list.
type Liker interface {
	SetLiked(context.Context, *pluginapi.Track, bool) error
}

// AlbumLiker is an optional capability for platforms with starred/collected albums.
type AlbumLiker interface {
	SetAlbumLiked(context.Context, string, bool) error
}

// PlaylistEditor is an optional capability for platforms whose playlists can
// be changed by the user.
type PlaylistEditor interface {
	CreatePlaylist(context.Context, string) (pluginapi.Playlist, error)
	RenamePlaylist(context.Context, pluginapi.Playlist, string) error
	DeletePlaylist(context.Context, pluginapi.Playlist) error
	AddToPlaylist(context.Context, pluginapi.Playlist, []pluginapi.Track) error
	RemoveFromPlaylist(context.Context, pluginapi.Playlist, []pluginapi.Track) error
}

// Transcoder is an optional capability. The returned stream uses the shared
// player stream model; the concrete HLS/session implementation stays inside
// the provider adapter.
type Transcoder interface {
	ShouldTranscode(*pluginapi.Track) bool
	TranscodedStream(context.Context, *pluginapi.Track, int) (*pluginapi.Stream, error)
}

// ErrUnsupported is returned when a provider cannot represent a requested
// operation. It lets the UI hide an unsupported capability instead of
// guessing from a platform ID.
var ErrUnsupported = errors.New("platform: unsupported operation")

// HTTPHeaders is an optional capability for a stream that needs signed
// request headers. It is kept separate from Provider so ordinary providers
// remain simple.
type HTTPHeaders interface {
	StreamHeaders(*pluginapi.Track) http.Header
}

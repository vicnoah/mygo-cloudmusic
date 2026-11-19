// Package pluginapi defines contracts used by music platform plugins and their host.
package pluginapi

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrUnavailable indicates that the service cannot stream a track.
var ErrUnavailable = errors.New("this song isn't available to stream")

type Artist struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Album struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	PicURL string `json:"picUrl"`
}

type Track struct {
	PlatformTrack
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	Artists  []Artist `json:"ar"`
	Album    Album    `json:"al"`
	Duration int64    `json:"dt"` // milliseconds
	Fee      int      `json:"fee"`
	Alias    []string `json:"alia"`
	// Reason says why a daily recommendation was picked.
	Reason string `json:"reason"`
	// Privilege is what the listener may do with the track, if known.
	Privilege *Privilege `json:"privilege"`
	// A podcast's episode plays as a track of its audio, ID, with the
	// episode's ProgramID, the PodcastID it is in, and what it is about.
	ProgramID   int64  `json:"programId,omitempty"`
	PodcastID   int64  `json:"podcastId,omitempty"`
	Description string `json:"description,omitempty"`
}

// Privilege is what NetEase lets the listener signed in, or nobody, do
// with a track.
type Privilege struct {
	ID int64 `json:"id"`
	St int   `json:"st"` // below 0: taken down
	Pl int   `json:"pl"` // the best bit rate the listener may play; 0 for none
}

// ArtistNames joins the track's artists as the credits read.
func (t *Track) ArtistNames() string {
	names := make([]string, len(t.Artists))
	for i, a := range t.Artists {
		names[i] = a.Name
	}
	return strings.Join(names, ", ")
}

// Playable reports whether the listener can play the whole track: as their
// privilege says, or else as its fee does for anyone, VIP songs (1) and
// paid albums (4) giving only a preview.
func (t *Track) Playable() bool {
	if t.ProgramID > 0 {
		// An episode's audio has no privilege: whether it streams, as a
		// paid one bought, is up to its stream.
		return true
	}
	if t.Privilege != nil {
		return t.Privilege.St >= 0 && t.Privilege.Pl > 0
	}
	return t.Fee != 1 && t.Fee != 4
}

func (t *Track) Length() time.Duration { return time.Duration(t.Duration) * time.Millisecond }

type Playlist struct {
	PlatformPlaylist
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	CoverURL    string  `json:"coverImgUrl"`
	PicURL      string  `json:"picUrl"`
	Description string  `json:"description"`
	PlayCount   float64 `json:"playCount"`
	TrackCount  int     `json:"trackCount"`
	UpdateFreq  string  `json:"updateFrequency"`
	SpecialType int     `json:"specialType"` // 5: the listener's liked songs
	Privacy     int     `json:"privacy"`     // 10: only its creator sees it
	Subscribed  bool    `json:"subscribed"`  // the listener saved it
	Creator     struct {
		UserID   int64  `json:"userId"`
		Nickname string `json:"nickname"`
	} `json:"creator"`
	Tracks   []Track `json:"tracks"`
	TrackIDs []struct {
		ID int64 `json:"id"`
	} `json:"trackIds"`
	// Preview holds a chart's top songs, as its card shows them.
	Preview []ChartLine `json:"-"`
}

// ChartLine is a song at the top of a chart: its name and its artist.
type ChartLine struct {
	First  string `json:"first"`
	Second string `json:"second"`
}

func (p *Playlist) Cover() string {
	if p.CoverURL != "" {
		return p.CoverURL
	}
	return p.PicURL
}

// Stream is where a track streams from, and what it is.
type Stream struct {
	PlatformStream
	URL  string `json:"-"`
	Size int64  `json:"size"` // in bytes
	// VerifyCachedContent is optional. It checks the bytes against an exact
	// content digest supplied by the service for this stream.
	VerifyCachedContent func(io.Reader) bool `json:"-"`
	// Type is the format: mp3, or flac for the lossless levels.
	Type string `json:"type"`
	// Level is the level served, which may be below the one asked for.
	Level string `json:"level"`
	// Gain is how many dB to change the track by to sound as loud as
	// others, and Peak its loudest sample, 1 at full scale, or 0 if
	// unknown.
	Gain float64 `json:"gain"`
	Peak float64 `json:"peak"`
}

// Levels are the legacy playback quality identifiers, from the lowest.
var Levels = []string{"standard", "higher", "exhigh", "lossless", "hires"}

type PlatformTrack struct {
	Source  string `json:"src,omitempty"`
	Ref     string `json:"ref,omitempty"`
	Format  string `json:"fmt,omitempty"`
	Cue     bool   `json:"cue,omitempty"`
	Fav     bool   `json:"fav,omitempty"`
	DiscNo  int    `json:"disc,omitempty"`
	TrackNo int    `json:"no,omitempty"`
}

type PlatformPlaylist struct {
	Ref  string `json:"-"`
	Kind string `json:"-"`
	Sub  string `json:"-"`
}

type PlatformStream struct {
	Header func() http.Header `json:"-"`
}

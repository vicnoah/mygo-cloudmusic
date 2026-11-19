package qqmusic

import (
	"strconv"
	"time"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/paging"
	backend "waygu-music/internal/plugins/qqmusic/internal/client"
)

func track(t backend.Track) pluginapi.Track {
	artists := make([]pluginapi.Artist, len(t.Artists))
	for i, a := range t.Artists {
		artists[i] = pluginapi.Artist{ID: a.ID, Name: a.Name}
	}
	ref := t.MID
	if ref == "" {
		ref = strconv.FormatInt(t.ID, 10)
	}
	out := pluginapi.Track{
		PlatformTrack: pluginapi.PlatformTrack{Source: Source, Ref: ref},
		ID:            t.ID, Name: t.Name, Artists: artists,
		Album:    pluginapi.Album{ID: t.Album.ID, Name: t.Album.Name, PicURL: t.Album.PicURL},
		Duration: t.Duration, Fee: t.Fee, Alias: t.Alias, Reason: t.Reason,
		ProgramID: t.ProgramID, PodcastID: t.PodcastID, Description: t.Description,
	}
	if t.Privilege != nil {
		out.Privilege = &pluginapi.Privilege{ID: t.Privilege.ID, St: t.Privilege.St, Pl: t.Privilege.Pl}
	}
	return out
}

func tracks(in []backend.Track) []pluginapi.Track {
	out := make([]pluginapi.Track, len(in))
	for i := range in {
		out[i] = track(in[i])
	}
	return out
}

func playlist(p backend.Playlist) pluginapi.Playlist {
	out := pluginapi.Playlist{
		PlatformPlaylist: pluginapi.PlatformPlaylist{Ref: strconv.FormatInt(p.ID, 10), Kind: "playlist"},
		ID:               p.ID, Name: p.Name, CoverURL: p.CoverURL, PicURL: p.PicURL,
		Description: p.Description, PlayCount: p.PlayCount, TrackCount: p.TrackCount,
		UpdateFreq: p.UpdateFreq, SpecialType: p.SpecialType, Privacy: p.Privacy,
		Tracks: tracks(p.Tracks),
	}
	out.Creator.UserID, out.Creator.Nickname = p.Creator.UserID, p.Creator.Nickname
	for _, line := range p.Preview {
		out.Preview = append(out.Preview, pluginapi.ChartLine{First: line.First, Second: line.Second})
	}
	return out
}

func trackIDs(in []pluginapi.Track) []int64 {
	out := make([]int64, len(in))
	for i, tr := range in {
		out[i] = tr.ID
	}
	return out
}

func playlistCover(p pluginapi.Playlist) string {
	if p.CoverURL != "" {
		return p.CoverURL
	}
	return p.PicURL
}

func playlists(in []backend.Playlist) []pluginapi.Playlist {
	out := make([]pluginapi.Playlist, len(in))
	for i := range in {
		out[i] = playlist(in[i])
	}
	return out
}

func albumCard(a backend.AlbumProfile) pluginapi.Playlist {
	sub := a.ArtistNames()
	if a.PublishTime > 0 {
		year := time.UnixMilli(a.PublishTime).Format("2006")
		if sub != "" {
			sub += " · "
		}
		sub += year
	}
	return pluginapi.Playlist{
		PlatformPlaylist: pluginapi.PlatformPlaylist{Ref: a.MID, Kind: "album", Sub: a.ArtistNames()},
		ID:               a.ID, Name: a.Name, CoverURL: a.PicURL, TrackCount: a.Size,
		UpdateFreq: sub,
	}
}

func albumCards(in []backend.AlbumProfile) []pluginapi.Playlist {
	out := make([]pluginapi.Playlist, len(in))
	for i := range in {
		out[i] = albumCard(in[i])
	}
	return out
}

func artistCard(a backend.ArtistProfile) pluginapi.Playlist {
	cover := a.PicURL
	if cover == "" {
		cover = a.Avatar
	}
	return pluginapi.Playlist{
		PlatformPlaylist: pluginapi.PlatformPlaylist{Ref: a.MID, Kind: "artist"},
		ID:               a.ID, Name: a.Name, CoverURL: cover, TrackCount: a.MusicSize,
		UpdateFreq: strconv.Itoa(a.MusicSize) + " 首",
	}
}

func artistCards(in []backend.ArtistProfile) []pluginapi.Playlist {
	out := make([]pluginapi.Playlist, len(in))
	for i := range in {
		out[i] = artistCard(in[i])
	}
	return out
}

func qqComments(in *backend.Comments) *pluginapi.Comments {
	if in == nil {
		return &pluginapi.Comments{}
	}
	return &pluginapi.Comments{Hot: qqCommentList(in.Hot), Latest: qqCommentList(in.Latest), Total: in.Total, More: in.More}
}

func qqCommentList(in []backend.Comment) []pluginapi.Comment {
	if len(in) == 0 {
		return nil
	}
	out := make([]pluginapi.Comment, len(in))
	for i, cm := range in {
		out[i] = pluginapi.Comment{
			ID:         cm.ID,
			Content:    cm.Content,
			Time:       cm.Time,
			LikedCount: cm.LikedCount,
			Liked:      cm.Liked,
		}
		out[i].User.ID = cm.User.ID
		out[i].User.Nickname = cm.User.Nickname
		out[i].User.AvatarURL = cm.User.AvatarURL
		out[i].IPLocation.Location = cm.IPLocation.Location
		for _, r := range cm.BeReplied {
			out[i].BeReplied = append(out[i].BeReplied, struct {
				User struct {
					Nickname string `json:"nickname"`
				} `json:"user"`
				Content string `json:"content"`
			}{User: struct {
				Nickname string `json:"nickname"`
			}{Nickname: r.User.Nickname}, Content: r.Content})
		}
	}
	return out
}

func staticTracks(in []pluginapi.Track) *trackList {
	total := len(in)
	return &trackList{
		Total: func() int { return total },
		Len:   func() int { return total },
		At: func(i int) *pluginapi.Track {
			if i < 0 || i >= total {
				return nil
			}
			return &in[i]
		},
	}
}

// lazyTracks wraps a pager as the shared list, which loads pages as rows
// come into view.
func lazyTracks(p *paging.Pager[pluginapi.Track]) *trackList {
	return &trackList{
		Total: p.Total, Len: p.Len, At: p.At,
		Retry: p.Retry, Failed: func() bool { return p.FailedAt(0) },
	}
}

func staticCards(in []pluginapi.Playlist) *paging.Pager[pluginapi.Playlist] {
	p := paging.New[pluginapi.Playlist](max(len(in), 1), 0, nil, nil)
	p.Land(0, in, len(in))
	return p
}

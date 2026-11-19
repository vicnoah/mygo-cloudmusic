package netease

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"waygu-music/internal/pluginapi"
)

// FM modes: what the personal FM picks songs by.
const (
	FMDefault  = "DEFAULT"  // the listener's taste
	FMFamiliar = "FAMILIAR" // songs like those they know
	FMExplore  = "EXPLORE"  // songs new to them
)

// PersonalFM returns the next few songs of the listener's personal FM, in
// mode. It asks the desktop client's API, which carries the session.
func PersonalFM(ctx context.Context, mode string) ([]Track, error) {
	var res struct {
		Code int `json:"code"`
		Data []struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	data := map[string]any{"mode": mode, "limit": 3}
	if err := eapiPost(ctx, "/api/v1/radio/get", data, &res); err != nil {
		return nil, err
	}
	if res.Code != 200 {
		return nil, fmt.Errorf("netease: personal FM: code %d", res.Code)
	}
	// The radio names its songs' fields as the older APIs do: their
	// details say them, with what the listener may play.
	ids := make([]int64, len(res.Data))
	for i, d := range res.Data {
		ids[i] = d.ID
	}
	return SongDetails(ctx, ids)
}

// FMTrash tells the personal FM the listener does not like a song, which
// it plays no more.
func FMTrash(ctx context.Context, id int64) error {
	var res struct {
		Code int `json:"code"`
	}
	data := map[string]any{"songId": strconv.FormatInt(id, 10), "alg": "RT", "time": "25"}
	if err := eapiPost(ctx, "/api/radio/trash/add", data, &res); err != nil {
		return err
	}
	if res.Code != 200 {
		return fmt.Errorf("netease: FM trash: code %d", res.Code)
	}
	return nil
}

// HeartMode returns songs like one of the listener's liked songs, by the
// heart mode (心动模式) of their liked songs, the playlist pid: from song
// on, picked after start, the song the mode began with.
func HeartMode(ctx context.Context, song, start, pid int64) ([]Track, error) {
	var res struct {
		Code int `json:"code"`
		Data []struct {
			SongInfo *Track `json:"songInfo"`
		} `json:"data"`
	}
	data := map[string]any{
		"songId":       strconv.FormatInt(song, 10),
		"startMusicId": strconv.FormatInt(start, 10),
		"playlistId":   strconv.FormatInt(pid, 10),
		"type":         "fromPlayOne",
		"count":        1,
	}
	if err := eapiPost(ctx, "/api/playmode/intelligence/list", data, &res); err != nil {
		return nil, err
	}
	if res.Code != 200 {
		return nil, fmt.Errorf("netease: heart mode: code %d", res.Code)
	}
	var tracks []Track
	for _, d := range res.Data {
		if d.SongInfo != nil {
			tracks = append(tracks, *d.SongInfo)
		}
	}
	detailPrivileges(ctx, tracks)
	return tracks, nil
}

type Comment = pluginapi.Comment
type Comments = pluginapi.Comments

// ThreadComments returns limit of the latest comments of a thread, a
// track's (see TrackThread), from offset, and on the first page its hot
// comments.
func ThreadComments(ctx context.Context, thread string, offset, limit int) (*Comments, error) {
	var res Comments
	q := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
	if err := get(ctx, "/api/v1/resource/comments/"+thread, q, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// LikeComment likes a comment of a thread for the listener, or unlikes it.
func LikeComment(ctx context.Context, thread string, id int64, like bool) error {
	path := "/api/v1/comment/unlike"
	if like {
		path = "/api/v1/comment/like"
	}
	return eapiOK(ctx, path, map[string]any{"threadId": thread, "commentId": strconv.FormatInt(id, 10)})
}

package netease

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
)

// Event is something posted to an account's feed: words, pictures, and
// maybe what it shares.
type Event struct {
	ID       int64 `json:"id"`
	Time     int64 `json:"eventTime"` // Unix milliseconds
	User     User  `json:"user"`
	Pictures []struct {
		SquareURL string `json:"squareUrl"`
	} `json:"pics"`
	Info struct {
		LikedCount   int `json:"likedCount"`
		CommentCount int `json:"commentCount"`
		ShareCount   int `json:"shareCount"`
	} `json:"info"`
	Content EventContent `json:"-"`
}

// EventContent is what an event says and shares.
type EventContent struct {
	Msg      string
	Song     *Track
	Album    *AlbumProfile
	Artist   *ArtistProfile
	Playlist *Playlist
	Video    bool // an MV or video, which the app can't play
	// Forwarded is the event it passes on, as its poster's name and words.
	Forwarded string
}

// eventJSON is the content of an event, which comes as a string of JSON.
type eventJSON struct {
	Msg      string         `json:"msg"`
	Song     *legacyTrack   `json:"song"`
	Album    *AlbumProfile  `json:"album"`
	Artist   *ArtistProfile `json:"artist"`
	Playlist *Playlist      `json:"playlist"`
	MV       any            `json:"mv"`
	Video    any            `json:"video"`
	Event    *struct {
		User User   `json:"user"`
		JSON string `json:"json"`
	} `json:"event"`
}

func decodeEvent(s string) EventContent {
	var j eventJSON
	if json.Unmarshal([]byte(s), &j) != nil {
		return EventContent{}
	}
	c := EventContent{Msg: j.Msg, Album: j.Album, Artist: j.Artist, Playlist: j.Playlist, Video: j.MV != nil || j.Video != nil}
	if j.Song != nil && j.Song.ID > 0 {
		t := j.Song.track()
		c.Song = &t
	}
	if j.Event != nil {
		var fw eventJSON
		if json.Unmarshal([]byte(j.Event.JSON), &fw) == nil && fw.Msg != "" {
			c.Forwarded = fw.Msg
			if name := j.Event.User.Nickname; name != "" {
				c.Forwarded = name + "：" + fw.Msg
			}
		}
	}
	return c
}

// Events returns up to limit of an account's events posted before a time
// in Unix milliseconds, or the latest ones for -1, with the time of the
// last returned to go on from, and whether there are more.
func Events(ctx context.Context, uid int64, limit int, before int64) (events []Event, last int64, more bool, err error) {
	var res struct {
		Events []struct {
			Event
			JSON string `json:"json"`
		} `json:"events"`
		LastTime int64 `json:"lasttime"`
		More     bool  `json:"more"`
	}
	q := url.Values{"time": {strconv.FormatInt(before, 10)}, "limit": {strconv.Itoa(limit)}, "getcounts": {"true"}, "total": {"false"}}
	if err := get(ctx, "/api/event/get/"+strconv.FormatInt(uid, 10), q, &res); err != nil {
		return nil, 0, false, err
	}
	for _, e := range res.Events {
		e.Content = decodeEvent(e.JSON)
		events = append(events, e.Event)
	}
	return events, res.LastTime, res.More, nil
}

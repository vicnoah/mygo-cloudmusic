package netease

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/network"
)

// The public NetEase Cloud Music API, as the web player calls it, without
// an account: what anyone may listen to.

const apiBase = "https://music.163.com"

const UserAgent = network.BrowserUserAgent

var httpClient = &http.Client{Timeout: 20 * time.Second}

// Media aliases preserve the wire and on-disk representations used by the client.
type (
	Artist           = pluginapi.Artist
	Album            = pluginapi.Album
	Track            = pluginapi.Track
	Privilege        = pluginapi.Privilege
	Playlist         = pluginapi.Playlist
	ChartLine        = pluginapi.ChartLine
	Stream           = pluginapi.Stream
	PlatformTrack    = pluginapi.PlatformTrack
	PlatformPlaylist = pluginapi.PlatformPlaylist
	PlatformStream   = pluginapi.PlatformStream
)

// withPrivileges gives tracks their privileges, matched by ID.
func withPrivileges(tracks []Track, privileges []Privilege) {
	byID := make(map[int64]*Privilege, len(privileges))
	for i := range privileges {
		byID[privileges[i].ID] = &privileges[i]
	}
	for i := range tracks {
		if p := byID[tracks[i].ID]; p != nil {
			tracks[i].Privilege = p
		}
	}
}

// TrackURL is the track's page on the NetEase website.
func TrackURL(t *Track) string {
	if !usesNetEaseURL(t) {
		return ""
	}
	if t.ProgramID > 0 {
		return apiBase + "/program?id=" + strconv.FormatInt(t.ProgramID, 10)
	}
	return apiBase + "/song?id=" + strconv.FormatInt(t.ID, 10)
}

// TrackThread names the track's comments, and what NetEase likes it by, for an
// episode: the episode's, not its audio's.
func TrackThread(t *Track) string {
	if t.ProgramID > 0 {
		return programThread(t.ProgramID)
	}
	return "R_SO_4_" + strconv.FormatInt(t.ID, 10)
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Msg     string `json:"msg"`
}

func get(ctx context.Context, path string, query url.Values, out any) error {
	u := apiBase + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	return do(req, out)
}

func post(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, "POST", apiBase+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return do(req, out)
}

func do(req *http.Request, out any) error {
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Referer", apiBase+"/")
	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	var status apiError
	if err := json.Unmarshal(body, &status); err != nil {
		return fmt.Errorf("netease: %s: %w", req.URL.Path, err)
	}
	if status.Code != 200 {
		msg := status.Message
		if msg == "" {
			msg = status.Msg
		}
		return fmt.Errorf("netease: %s: code %d %s", req.URL.Path, status.Code, msg)
	}
	return json.Unmarshal(body, out)
}

// FeaturedPlaylists returns the playlists the home page recommends.
func FeaturedPlaylists(ctx context.Context, limit int) ([]Playlist, error) {
	var res struct {
		Result []Playlist `json:"result"`
	}
	err := get(ctx, "/api/personalized/playlist", url.Values{"limit": {strconv.Itoa(limit)}}, &res)
	return res.Result, err
}

// DailySongs returns the day's recommended songs: chosen for the listener
// once signed in, and what many listen to before.
func DailySongs(ctx context.Context) ([]Track, error) {
	var res struct {
		Data struct {
			DailySongs []Track `json:"dailySongs"`
		} `json:"data"`
	}
	if err := get(ctx, "/api/v3/discovery/recommend/songs", nil, &res); err != nil {
		return nil, err
	}
	// They come without privileges: the songs' details have them.
	songs := res.Data.DailySongs
	detailPrivileges(ctx, songs)
	return songs, nil
}

// detailPrivileges gives songs the privileges their details carry for the
// listener, which are right where other lists have none, or wrong ones.
func detailPrivileges(ctx context.Context, songs []Track) {
	ids := make([]int64, len(songs))
	for i := range songs {
		ids[i] = songs[i].ID
	}
	details, err := SongDetails(ctx, ids)
	if err != nil {
		return
	}
	var privileges []Privilege
	for _, d := range details {
		if d.Privilege != nil {
			privileges = append(privileges, *d.Privilege)
		}
	}
	withPrivileges(songs, privileges)
}

// Charts returns the charts, each with its top three songs.
func Charts(ctx context.Context) ([]Playlist, error) {
	var res struct {
		List []struct {
			Playlist
			Tracks []ChartLine `json:"tracks"`
		} `json:"list"`
	}
	if err := get(ctx, "/api/toplist/detail", nil, &res); err != nil {
		return nil, err
	}
	out := make([]Playlist, len(res.List))
	for i, l := range res.List {
		out[i] = l.Playlist
		out[i].Tracks = nil
		out[i].Preview = l.Tracks
	}
	return out, nil
}

// PlaylistDetail returns a playlist with up to 1000 of its tracks.
func PlaylistDetail(ctx context.Context, id int64) (*Playlist, error) {
	var res struct {
		Playlist   Playlist    `json:"playlist"`
		Privileges []Privilege `json:"privileges"`
	}
	q := url.Values{"id": {strconv.FormatInt(id, 10)}, "n": {"1000"}}
	if err := get(ctx, "/api/v6/playlist/detail", q, &res); err != nil {
		return nil, err
	}
	p := &res.Playlist
	withPrivileges(p.Tracks, res.Privileges)
	// Its privileges mark VIP songs as taken down that a subscriber may
	// play, as Stan's: the songs' details say right.
	detailPrivileges(ctx, p.Tracks)
	// Long playlists list only their first tracks in full: the rest by id.
	if len(p.Tracks) < len(p.TrackIDs) {
		var ids []int64
		for _, t := range p.TrackIDs[len(p.Tracks):min(len(p.TrackIDs), 1000)] {
			ids = append(ids, t.ID)
		}
		more, err := SongDetails(ctx, ids)
		if err == nil {
			p.Tracks = append(p.Tracks, more...)
		}
	}
	return p, nil
}

// SongDetails returns the tracks of ids, in their order. It goes through
// the eapi: the web API's privileges call some VIP songs taken down for
// subscribers, whom the eapi streams them to.
func SongDetails(ctx context.Context, ids []int64) ([]Track, error) {
	var out []Track
	for len(ids) > 0 {
		batch := ids[:min(len(ids), 400)]
		ids = ids[len(batch):]
		c := make([]map[string]int64, len(batch))
		for i, id := range batch {
			c[i] = map[string]int64{"id": id}
		}
		b, _ := json.Marshal(c)
		var res struct {
			Code       int         `json:"code"`
			Songs      []Track     `json:"songs"`
			Privileges []Privilege `json:"privileges"`
		}
		if err := eapiPost(ctx, "/api/v3/song/detail", map[string]any{"c": string(b)}, &res); err != nil {
			return out, err
		}
		if res.Code != 200 {
			return out, fmt.Errorf("netease: song detail: code %d", res.Code)
		}
		withPrivileges(res.Songs, res.Privileges)
		out = append(out, res.Songs...)
	}
	return out, nil
}

// ArtistProfile is who an artist is, as their page shows them, with the
// songs they are best known for.
type ArtistProfile struct {
	Artist
	Avatar    string   `json:"img1v1Url"` // square
	PicURL    string   `json:"picUrl"`    // wide
	Alias     []string `json:"alias"`
	Trans     string   `json:"trans"`
	BriefDesc string   `json:"briefDesc"`
	MusicSize int      `json:"musicSize"`
	AlbumSize int      `json:"albumSize"`
	Followed  bool     `json:"followed"` // by the listener signed in
	// AccountID is the artist's own account, which posts their events, or 0.
	AccountID int64   `json:"accountId"`
	Songs     []Track `json:"-"`
}

// ArtistDetail returns an artist with their 50 most popular songs.
func ArtistDetail(ctx context.Context, id int64) (*ArtistProfile, error) {
	var res struct {
		Artist   ArtistProfile `json:"artist"`
		HotSongs []Track       `json:"hotSongs"`
	}
	if err := get(ctx, "/api/v1/artist/"+strconv.FormatInt(id, 10), nil, &res); err != nil {
		return nil, err
	}
	ar := &res.Artist
	ar.Songs = res.HotSongs
	detailPrivileges(ctx, ar.Songs)
	return ar, nil
}

// ArtistLatestSongs returns up to limit of an artist's songs, the newest
// first.
func ArtistLatestSongs(ctx context.Context, id int64, limit int) ([]Track, error) {
	var res struct {
		Songs []struct {
			ID int64 `json:"id"`
		} `json:"songs"`
	}
	form := url.Values{"id": {strconv.FormatInt(id, 10)}, "order": {"time"}, "limit": {strconv.Itoa(limit)}, "offset": {"0"}}
	if err := post(ctx, "/api/v1/artist/songs", form, &res); err != nil {
		return nil, err
	}
	// Some artists' songs come in the old shape (artists, album, duration)
	// that Track doesn't read: their details have them in the new one, as
	// the listener's privileges.
	ids := make([]int64, len(res.Songs))
	for i, s := range res.Songs {
		ids[i] = s.ID
	}
	return SongDetails(ctx, ids)
}

var ErrUnavailable = pluginapi.ErrUnavailable

// Levels are the stream levels, from the lowest.
var Levels = pluginapi.Levels

// Lossless reports whether level streams FLAC.
func Lossless(level string) bool { return level == "lossless" || level == "hires" }

// SongURL returns where to stream a track, MP3 or FLAC, at level (one of
// Levels) or the best below it the listener may play. It goes through
// the eapi, which carries the session, so VIP songs play for subscribers.
func SongURL(ctx context.Context, id int64, level string) (*Stream, error) {
	var res struct {
		Code int `json:"code"`
		Data []struct {
			Stream
			URL           string          `json:"url"`
			MD5           string          `json:"md5"`
			Peak          *float64        `json:"peak"`
			FreeTrialInfo json.RawMessage `json:"freeTrialInfo"`
		} `json:"data"`
	}
	encode := "mp3"
	if Lossless(level) {
		encode = "flac"
	}
	data := map[string]any{"ids": fmt.Sprintf("[%d]", id), "level": level, "encodeType": encode}
	if err := eapiPost(ctx, "/api/song/enhance/player/url/v1", data, &res); err != nil {
		return nil, err
	}
	if len(res.Data) == 0 || res.Data[0].URL == "" {
		return nil, ErrUnavailable
	}
	d := res.Data[0]
	// A preview of a VIP song is no song.
	if len(d.FreeTrialInfo) > 0 && string(d.FreeTrialInfo) != "null" {
		return nil, ErrUnavailable
	}
	s := d.Stream
	s.Type = strings.ToLower(s.Type)
	if s.Type != "" && s.Type != "mp3" && s.Type != "flac" {
		return nil, fmt.Errorf("unsupported format %s", s.Type)
	}
	if d.Peak != nil {
		s.Peak = *d.Peak
	}
	// The level names the song's file in the cache: one not of Levels is
	// taken for the one asked for.
	if !slices.Contains(Levels, s.Level) {
		s.Level = level
	}
	s.URL = strings.Replace(d.URL, "http://", "https://", 1)
	if digest, err := hex.DecodeString(d.MD5); err == nil && len(digest) == md5.Size {
		s.VerifyCachedContent = func(r io.Reader) bool {
			h := md5.New()
			_, err := io.Copy(h, r)
			return err == nil && bytes.Equal(h.Sum(nil), digest)
		}
	}
	return &s, nil
}

// Lyrics returns a track's timed lyrics, their translation if any, and
// its word-timed lyrics (YRC) where NetEase has them, which only the
// desktop client's API serves.
func Lyrics(ctx context.Context, id int64) (lrc, translation, yrc string, err error) {
	var res struct {
		Lrc struct {
			Lyric string `json:"lyric"`
		} `json:"lrc"`
		Tlyric struct {
			Lyric string `json:"lyric"`
		} `json:"tlyric"`
	}
	q := url.Values{"id": {strconv.FormatInt(id, 10)}, "lv": {"1"}, "tv": {"-1"}}
	if err = get(ctx, "/api/song/lyric", q, &res); err != nil {
		return "", "", "", err
	}
	var words struct {
		Yrc struct {
			Lyric string `json:"lyric"`
		} `json:"yrc"`
	}
	data := map[string]any{"id": strconv.FormatInt(id, 10), "cp": false, "tv": 0, "lv": 0, "rv": 0, "kv": 0, "yv": 0, "ytv": 0, "yrv": 0}
	if eapiPost(ctx, "/api/song/lyric/v1", data, &words) == nil {
		yrc = words.Yrc.Lyric
	}
	return res.Lrc.Lyric, res.Tlyric.Lyric, yrc, nil
}

// Sized asks the image CDN for a picture of n×n pixels.
func Sized(pic string, n int) string {
	return network.SizedArtworkURL(pic, n)
}

// Fetch returns what is at url, as a cover the image CDN serves.
func Fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New(res.Status)
	}
	return io.ReadAll(res.Body)
}

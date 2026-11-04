package songloft

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotConnected = errors.New("songloft: not connected")
	ErrAuthFailed   = errors.New("songloft: authentication failed")
)

type SongFilter struct {
	Keyword string
	Artist  string
	Album   string
	Genre   string
	Sort    string
	Order   string
	Limit   int
	Offset  int
}

type Client struct {
	mu           sync.RWMutex
	httpClient   *http.Client
	baseURL      string
	accessToken  string
	refreshToken string
	tokenExpiry  time.Time
}

func NewClient(rawURL string, hc *http.Client) (*Client, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	u := strings.TrimSpace(rawURL)
	if u == "" {
		return nil, errors.New("songloft: empty server url")
	}
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		u = "http://" + u
	}
	u = strings.TrimRight(u, "/")
	parsed, err := url.Parse(u)
	if err != nil {
		return nil, fmt.Errorf("songloft: invalid url %q: %w", rawURL, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("songloft: incomplete server url %q", rawURL)
	}

	basePath := strings.TrimRight(parsed.Path, "/")
	return &Client{
		httpClient: hc,
		baseURL:    fmt.Sprintf("%s://%s%s", parsed.Scheme, parsed.Host, basePath),
	}, nil
}

func (c *Client) BaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseURL
}

func (c *Client) AccessToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.accessToken
}

func (c *Client) SetTokens(access, refresh string, expiresIn int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accessToken = access
	c.refreshToken = refresh
	if expiresIn > 0 {
		c.tokenExpiry = time.Now().Add(time.Duration(expiresIn) * time.Second)
	}
}

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

func (c *Client) Login(ctx context.Context, username, password string) error {
	reqBody, err := json.Marshal(loginReq{Username: username, Password: password})
	if err != nil {
		return err
	}

	c.mu.RLock()
	endpoint := c.baseURL + "/api/v1/auth/login"
	c.mu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var errResp struct {
			Error  string `json:"error"`
			Detail string `json:"detail"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			if errResp.Detail != "" {
				return fmt.Errorf("%s: %s", errResp.Error, errResp.Detail)
			}
			return errors.New(errResp.Error)
		}
		return fmt.Errorf("%w: status %d: %s", ErrAuthFailed, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var lr loginResp
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return fmt.Errorf("songloft: decode login response: %w", err)
	}

	c.SetTokens(lr.AccessToken, lr.RefreshToken, lr.ExpiresIn)
	return nil
}

func (c *Client) RefreshToken(ctx context.Context) error {
	c.mu.RLock()
	rf := c.refreshToken
	endpoint := c.baseURL + "/api/v1/auth/refresh"
	c.mu.RUnlock()

	if rf == "" {
		return ErrAuthFailed
	}

	reqBody, _ := json.Marshal(map[string]string{"refresh_token": rf})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ErrAuthFailed
	}

	var lr loginResp
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return err
	}

	c.SetTokens(lr.AccessToken, lr.RefreshToken, lr.ExpiresIn)
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any, target any) error {
	c.mu.RLock()
	token := c.accessToken
	base := c.baseURL
	c.mu.RUnlock()

	fullURL := base + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Handle 401 by attempting token refresh once
	if resp.StatusCode == http.StatusUnauthorized {
		if rerr := c.RefreshToken(ctx); rerr == nil {
			c.mu.RLock()
			newToken := c.accessToken
			c.mu.RUnlock()

			if body != nil {
				b, _ := json.Marshal(body)
				reader = bytes.NewReader(b)
			}
			retryReq, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
			if err != nil {
				return err
			}
			if body != nil {
				retryReq.Header.Set("Content-Type", "application/json")
			}
			retryReq.Header.Set("Authorization", "Bearer "+newToken)
			resp2, err2 := c.httpClient.Do(retryReq)
			if err2 != nil {
				return err2
			}
			defer resp2.Body.Close()
			resp = resp2
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("songloft error %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	if target != nil {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			return fmt.Errorf("songloft decode error: %w", err)
		}
	}
	return nil
}

func (c *Client) ListSongs(ctx context.Context, f SongFilter) (SongListResponse, error) {
	q := url.Values{}
	if f.Keyword != "" {
		q.Set("keyword", f.Keyword)
	}
	if f.Artist != "" {
		q.Set("artist", f.Artist)
	}
	if f.Album != "" {
		q.Set("album", f.Album)
	}
	if f.Genre != "" {
		q.Set("genre", f.Genre)
	}
	if f.Sort != "" {
		q.Set("sort", f.Sort)
	}
	if f.Order != "" {
		q.Set("order", f.Order)
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	if f.Offset > 0 {
		q.Set("offset", strconv.Itoa(f.Offset))
	}

	var res SongListResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/songs", q, nil, &res)
	return res, err
}

func (c *Client) GetSong(ctx context.Context, id int64) (Song, error) {
	var s Song
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/songs/%d", id), nil, nil, &s)
	return s, err
}

func (c *Client) ListFacets(ctx context.Context, field, keyword, sort, order string, limit, offset int) (FacetListResponse, error) {
	q := url.Values{}
	q.Set("field", field)
	if keyword != "" {
		q.Set("keyword", keyword)
	}
	if sort != "" {
		q.Set("sort", sort)
	}
	if order != "" {
		q.Set("order", order)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}

	var res FacetListResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/songs/facets", q, nil, &res)
	return res, err
}

func (c *Client) ListPlaylists(ctx context.Context, limit, offset int) (PlaylistListResponse, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}

	var res PlaylistListResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/playlists", q, nil, &res)
	return res, err
}

func (c *Client) GetPlaylist(ctx context.Context, id int64) (Playlist, error) {
	var p Playlist
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/playlists/%d", id), nil, nil, &p)
	return p, err
}

func (c *Client) GetPlaylistSongs(ctx context.Context, id int64, sort, order string, limit, offset int) (SongListResponse, error) {
	q := url.Values{}
	if sort != "" {
		q.Set("sort", sort)
	}
	if order != "" {
		q.Set("order", order)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}

	var res SongListResponse
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/playlists/%d/songs", id), q, nil, &res)
	return res, err
}

// ReportPlay notifies Songloft that a song was started.
func (c *Client) ReportPlay(ctx context.Context, songID int64) error {
	q := url.Values{}
	q.Set("type", "play")
	q.Set("source", "mygo-cloudmusic")
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/songs/%d/played", songID), q, nil, nil)
}

type playlistCreateReq struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
}

func (c *Client) CreatePlaylist(ctx context.Context, name, desc string) (Playlist, error) {
	var res Playlist
	err := c.do(ctx, http.MethodPost, "/api/v1/playlists", nil, playlistCreateReq{Name: name, Description: desc, Type: "normal"}, &res)
	return res, err
}

func (c *Client) UpdatePlaylist(ctx context.Context, id int64, name, desc string) error {
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/api/v1/playlists/%d", id), nil, playlistCreateReq{Name: name, Description: desc}, nil)
}

func (c *Client) DeletePlaylist(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/playlists/%d", id), nil, nil, nil)
}

type playlistAddSongsReq struct {
	SongIDs []int64 `json:"song_ids"`
}

func (c *Client) AddSongsToPlaylist(ctx context.Context, playlistID int64, songIDs []int64) error {
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/playlists/%d/songs", playlistID), nil, playlistAddSongsReq{SongIDs: songIDs}, nil)
}

func (c *Client) RemoveSongFromPlaylist(ctx context.Context, playlistID int64, songID int64) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/playlists/%d/songs/%d", playlistID, songID), nil, nil, nil)
}

func (c *Client) GetLyrics(ctx context.Context, songID int64) (LyricResponse, error) {
	var res LyricResponse
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/songs/%d/lyric", songID), nil, nil, &res)
	if err != nil && strings.Contains(err.Error(), "404") {
		return LyricResponse{}, nil
	}
	return res, err
}

func (c *Client) ListFolders(ctx context.Context, path, keyword string) (FolderResponse, error) {
	q := url.Values{}
	if path != "" {
		q.Set("path", path)
	}
	if keyword != "" {
		q.Set("keyword", keyword)
	}
	var res FolderResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/songs/folders", q, nil, &res)
	return res, err
}

func (c *Client) StreamURL(songID int64) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return fmt.Sprintf("%s/api/v1/songs/%d/play?access_token=%s", c.baseURL, songID, url.QueryEscape(c.accessToken))
}

func (c *Client) FetchCover(ctx context.Context, raw string) ([]byte, error) {
	c.mu.RLock()
	base := c.baseURL
	token := c.accessToken
	c.mu.RUnlock()

	var targetURL string
	if rest, ok := strings.CutPrefix(raw, CoverScheme); ok {
		songID, _, _ := strings.Cut(rest, "?")
		targetURL = fmt.Sprintf("%s/api/v1/songs/%s/cover", base, songID)
	} else if rest, ok := strings.CutPrefix(raw, PlaylistCoverScheme); ok {
		plID, _, _ := strings.Cut(rest, "?")
		targetURL = fmt.Sprintf("%s/api/v1/playlists/%s/cover", base, plID)
	} else if rest, ok := strings.CutPrefix(raw, FacetCoverScheme); ok {
		facetURL, err := url.PathUnescape(rest)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(facetURL, "http://") || strings.HasPrefix(facetURL, "https://") {
			targetURL = facetURL
		} else {
			targetURL = base + facetURL
		}
	} else if strings.HasPrefix(raw, "/api/v1/") {
		targetURL = base + raw
	} else if strings.HasPrefix(raw, base) {
		targetURL = raw
	} else {
		return nil, errors.New("songloft: unsupported cover scheme")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("songloft cover fetch status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

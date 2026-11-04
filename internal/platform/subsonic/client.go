package subsonic

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	gosubsonic "github.com/supersonic-app/go-subsonic/subsonic"
	"netease-music-mygo/internal/platform"
)

// DefaultClientName names this application to Subsonic servers.
const DefaultClientName = "MyGoCloudMusic"

// ErrSignedOut is returned when an operation requires an authenticated client.
var ErrSignedOut = errors.New("subsonic: not connected")

// Config holds the connection parameters for a Subsonic server.
type Config struct {
	Address      string `json:"address"`
	Username     string `json:"username"`
	Password     string `json:"-"`
	ClientName   string `json:"clientName,omitempty"`
	PasswordAuth bool   `json:"passwordAuth,omitempty"`
	Insecure     bool   `json:"insecure,omitempty"`
}

// Valid reports whether the config has required fields to connect.
func (c Config) Valid() bool {
	return strings.TrimSpace(c.Address) != "" && strings.TrimSpace(c.Username) != "" && c.Password != ""
}

// NormalizeAddress ensures the server address has a valid HTTP/HTTPS scheme
// and strips trailing slashes.
func NormalizeAddress(raw string, defaultHTTPS bool) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = strings.TrimRight(s, "/")
	if !strings.HasPrefix(strings.ToLower(s), "http://") && !strings.HasPrefix(strings.ToLower(s), "https://") {
		if defaultHTTPS {
			s = "https://" + s
		} else {
			s = "http://" + s
		}
	}
	// Parse and re-serialize to clean up paths
	if u, err := url.Parse(s); err == nil {
		u.Path = strings.TrimRight(u.Path, "/")
		return u.String()
	}
	return s
}

// Client wraps a go-subsonic client with thread-safety and connection state.
type Client struct {
	cfg        Config
	platformID platform.ID

	mu            sync.RWMutex
	subsonic      *gosubsonic.Client
	httpClient    *http.Client
	pingInfo      *gosubsonic.PingResponse
	connected     bool
	songCount     int
	albumCount    int
	sortCounts    map[string]int
	albumStarred  map[string]bool
	transcodeMode string
	maxBitRate    int
}

// NewClient creates a new Subsonic client wrapper with the given config.
func NewClient(cfg Config, platformIDs ...platform.ID) *Client {
	pid := platform.SubsonicID
	if len(platformIDs) > 0 && platformIDs[0] != "" {
		pid = platformIDs[0]
	}
	return &Client{
		cfg:          cfg,
		platformID:   pid,
		sortCounts:   make(map[string]int),
		albumStarred: make(map[string]bool),
	}
}

// PlatformID returns the associated platform ID.
func (c *Client) PlatformID() platform.ID {
	if c == nil || c.platformID == "" {
		return platform.SubsonicID
	}
	return c.platformID
}

// Config returns a copy of the current configuration.
func (c *Client) Config() Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cfg
}

// Connected reports whether the client has successfully authenticated.
func (c *Client) Connected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected && c.subsonic != nil
}

// SetConnectedForTest sets the connected flag and a dummy client for testing.
func (c *Client) SetConnectedForTest(connected bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = connected
	if connected && c.subsonic == nil {
		c.subsonic = &gosubsonic.Client{}
	}
}

// ServerInfo returns the PingResponse obtained during authentication or ping.
func (c *Client) ServerInfo() *gosubsonic.PingResponse {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.pingInfo
}

// Subsonic returns the underlying go-subsonic Client.
func (c *Client) Subsonic() *gosubsonic.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.subsonic
}

// Connect validates credentials, establishes connection and authenticates.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cfg := c.cfg
	if !cfg.Valid() {
		return errors.New("subsonic: incomplete connection configuration")
	}

	address := NormalizeAddress(cfg.Address, false)
	clientName := cfg.ClientName
	if clientName == "" {
		clientName = DefaultClientName
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 128
	transport.MaxIdleConnsPerHost = 64
	if cfg.Insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	sc := &gosubsonic.Client{
		Client:       httpClient,
		BaseUrl:      address,
		User:         cfg.Username,
		ClientName:   clientName,
		PasswordAuth: cfg.PasswordAuth,
	}

	// Authenticate tests ping under the hood
	err := sc.Authenticate(cfg.Password)
	if err != nil {
		c.connected = false
		c.subsonic = nil
		c.pingInfo = nil
		return err
	}

	ping, err := sc.Ping()
	if err != nil {
		c.connected = false
		c.subsonic = nil
		c.pingInfo = nil
		return err
	}

	c.subsonic = sc
	c.httpClient = httpClient
	c.pingInfo = ping
	c.connected = true

	// Query scan status for song count
	if status, err := sc.GetScanStatus(); err == nil && status != nil && status.Count > 0 {
		c.songCount = int(status.Count)
	}

	// Probe album count in background
	go c.probeAlbumCount(sc)

	return nil
}

// probeAlbumCount counts total albums in 500-sized steps.
func (c *Client) probeAlbumCount(sc *gosubsonic.Client) {
	if sc == nil {
		return
	}
	c.mu.RLock()
	if c.albumCount > 0 {
		c.mu.RUnlock()
		return
	}
	c.mu.RUnlock()

	c.probeSortAlbumCount(sc, "newest", 0)
}

// SortAlbumCount returns the cached album count for a specific sort type if known.
func (c *Client) SortAlbumCount(sortType string) (int, bool) {
	if c == nil {
		return 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.sortCounts == nil {
		return 0, false
	}
	cnt, ok := c.sortCounts[sortType]
	return cnt, ok
}

// SetSortAlbumCount records the total album count for a specific sort type.
func (c *Client) SetSortAlbumCount(sortType string, count int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sortCounts == nil {
		c.sortCounts = make(map[string]int)
	}
	c.sortCounts[sortType] = count
}

// InvalidateSortCount clears the cached count for a given sort type.
func (c *Client) InvalidateSortCount(sortType string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sortCounts != nil {
		delete(c.sortCounts, sortType)
	}
}

// SetAlbumStarred records whether an album is starred.
func (c *Client) SetAlbumStarred(ref string, starred bool) {
	if c == nil || ref == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.albumStarred == nil {
		c.albumStarred = make(map[string]bool)
	}
	c.albumStarred[ref] = starred
}

// IsAlbumStarred reports whether an album is known to be starred.
func (c *Client) IsAlbumStarred(ref string) bool {
	if c == nil || ref == "" {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.albumStarred != nil && c.albumStarred[ref]
}

// AlbumStarredKnown reports whether the starred status for an album is known.
func (c *Client) AlbumStarredKnown(ref string) (starred bool, known bool) {
	if c == nil || ref == "" {
		return false, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.albumStarred == nil {
		return false, false
	}
	starred, known = c.albumStarred[ref]
	return starred, known
}

// AlbumCountForSort returns the stable album count for the given sort policy.
// It queries the exact count once if not yet known, preventing pagination flickering.
func (c *Client) AlbumCountForSort(sc *gosubsonic.Client, sortType string) int {
	if c == nil {
		return 0
	}
	if sortType == "" {
		sortType = "newest"
	}
	if cnt, ok := c.SortAlbumCount(sortType); ok {
		return cnt
	}
	switch sortType {
	case "starred":
		if sc != nil {
			if st, err := sc.GetStarred2(nil); err == nil && st != nil {
				c.SetSortAlbumCount(sortType, len(st.Album))
				return len(st.Album)
			}
		}
		c.SetSortAlbumCount(sortType, 0)
		return 0
	case "newest", "alphabeticalByName", "alphabeticalByArtist", "random":
		if cnt := c.AlbumCount(); cnt > 0 {
			c.SetSortAlbumCount(sortType, cnt)
			return cnt
		}
	}

	// For frequent, recent, byYear or unprobed full-library sorts, probe size=500
	if sc != nil {
		params := map[string]string{"size": "500", "offset": "0"}
		if sortType == "byYear" {
			params["fromYear"] = "1900"
			params["toYear"] = "3000"
		}
		albs, err := sc.GetAlbumList2(sortType, params)
		if err == nil {
			if len(albs) < 500 {
				c.SetSortAlbumCount(sortType, len(albs))
				return len(albs)
			}
			c.SetSortAlbumCount(sortType, len(albs))
			go c.probeSortAlbumCount(sc, sortType, 500)
			return len(albs)
		}
	}
	return c.AlbumCount()
}

func (c *Client) probeSortAlbumCount(sc *gosubsonic.Client, sortType string, startOffset int) {
	curr := startOffset
	for {
		params := map[string]string{
			"size":   "500",
			"offset": strconv.Itoa(curr),
		}
		if sortType == "byYear" {
			params["fromYear"] = "1900"
			params["toYear"] = "3000"
		}
		albs, err := sc.GetAlbumList2(sortType, params)
		if err != nil {
			return
		}
		curr += len(albs)
		if len(albs) < 500 {
			c.SetSortAlbumCount(sortType, curr)
			if sortType == "newest" {
				c.SetCounts(0, curr)
			}
			return
		}
		c.SetSortAlbumCount(sortType, curr)
	}
}

// SongCount returns the total number of songs in the library if known.
func (c *Client) SongCount() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	cnt := c.songCount
	c.mu.RUnlock()
	if cnt > 0 {
		return cnt
	}
	sc := c.Subsonic()
	if sc == nil {
		return 0
	}
	if status, err := sc.GetScanStatus(); err == nil && status != nil && status.Count > 0 {
		c.SetCounts(int(status.Count), 0)
		return int(status.Count)
	}
	return 0
}

// AlbumCount returns the total number of albums in the library if known.
func (c *Client) AlbumCount() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.albumCount
}

// SetTranscode updates the transcode mode and bitrate limit.
func (c *Client) SetTranscode(mode string, bitrate int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.transcodeMode = mode
	c.maxBitRate = bitrate
}

// TranscodeMode returns the current transcode policy ("auto", "native", "always").
func (c *Client) TranscodeMode() string {
	if c == nil {
		return "auto"
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.transcodeMode == "" {
		return "auto"
	}
	return c.transcodeMode
}

// MaxBitRate returns the configured maximum bitrate limit in kbps (0 for unlimited).
func (c *Client) MaxBitRate() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.maxBitRate
}

// SetCounts updates the cached song and album counts.
func (c *Client) SetCounts(songs, albums int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if songs > 0 {
		c.songCount = songs
	}
	if albums > 0 {
		c.albumCount = albums
	}
	c.mu.Unlock()
}

// Close disconnects and cleans up resources.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = false
	c.subsonic = nil
	c.pingInfo = nil
	return nil
}

// Ping checks if the connection is still alive.
func (c *Client) Ping() (*gosubsonic.PingResponse, error) {
	c.mu.RLock()
	sc := c.subsonic
	c.mu.RUnlock()

	if sc == nil {
		return nil, ErrSignedOut
	}
	ping, err := sc.Ping()
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.pingInfo = ping
	c.mu.Unlock()
	return ping, nil
}

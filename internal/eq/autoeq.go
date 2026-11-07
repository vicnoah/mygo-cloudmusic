package eq

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"netease-music-mygo/internal/network"
)

const (
	autoEQIndexURL = "https://raw.githubusercontent.com/jaakkopasanen/AutoEq/master/results/INDEX.md"
	autoEQBaseURL  = "https://raw.githubusercontent.com/jaakkopasanen/AutoEq/master/results/"

	indexTTL   = 7 * 24 * time.Hour
	profileTTL = 30 * 24 * time.Hour
)

// Meta is a headphone profile in AutoEQ's index.
type Meta struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Source string `json:"source"`
	Type   string `json:"type"`
}

// Profile is an AutoEQ FixedBandEQ curve: 10 bands and its preamp.
type Profile struct {
	Meta
	Preamp float64   `json:"preamp"`
	Bands  []float64 `json:"bands"`
}

// AutoEQ fetches and caches AutoEQ's headphone index and profiles.
type AutoEQ struct {
	dir    string
	client *http.Client

	mu      sync.Mutex
	index   []Meta
	indexAt time.Time
}

func NewAutoEQ(dir string, timeout time.Duration) *AutoEQ {
	return &AutoEQ{
		dir: dir,
		client: &http.Client{
			Timeout:   timeout,
			Transport: network.Transport(),
		},
	}
}

// Index returns AutoEQ's headphone profiles, from memory, disk, or the
// network, in that order.
func (a *AutoEQ) Index(ctx context.Context) ([]Meta, error) {
	a.mu.Lock()
	if len(a.index) > 0 && time.Since(a.indexAt) < indexTTL {
		out := append([]Meta(nil), a.index...)
		a.mu.Unlock()
		return out, nil
	}
	a.mu.Unlock()

	path := filepath.Join(a.dir, "index.json")
	if metas, err := loadJSON[[]Meta](path, indexTTL); err == nil {
		a.setIndex(metas)
		return metas, nil
	}
	b, err := a.get(ctx, autoEQIndexURL)
	if err != nil {
		return nil, err
	}
	metas, err := parseIndex(b)
	if err != nil {
		return nil, err
	}
	saveJSON(path, metas)
	a.setIndex(metas)
	return metas, nil
}

func (a *AutoEQ) setIndex(metas []Meta) {
	a.mu.Lock()
	a.index, a.indexAt = metas, time.Now()
	a.mu.Unlock()
}

// Profile fetches a profile from memory, disk, or the network.
func (a *AutoEQ) Profile(ctx context.Context, m Meta) (*Profile, error) {
	key := fmt.Sprintf("%x", md5.Sum([]byte(m.Path)))
	path := filepath.Join(a.dir, key+".json")
	if p, err := loadJSON[Profile](path, profileTTL); err == nil {
		return &p, nil
	}
	u, err := profileURL(m)
	if err != nil {
		return nil, err
	}
	b, err := a.get(ctx, u)
	if err != nil {
		return nil, err
	}
	p, err := parseProfile(m, b)
	if err != nil {
		return nil, err
	}
	saveJSON(path, p)
	return p, nil
}

func (a *AutoEQ) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "WagyuMusic")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("eq: AutoEQ profile not found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("eq: AutoEQ: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

func profileURL(m Meta) (string, error) {
	decoded, err := url.QueryUnescape(m.Path)
	if err != nil {
		decoded = m.Path
	}
	parts := strings.Split(decoded, "/")
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return "", errors.New("eq: bad AutoEQ path")
	}
	name := parts[len(parts)-1]
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return autoEQBaseURL + strings.Join(parts, "/") + "/" + url.PathEscape(name+" FixedBandEQ.txt"), nil
}

var (
	indexLink = regexp.MustCompile(`-\s*\[([^\]]+)\]\(\./([^)]+)\)`)
	preampRE  = regexp.MustCompile(`Preamp:\s*([-+]?\d+\.?\d*)\s*dB`)
	filterRE  = regexp.MustCompile(`Filter\s+\d+:.*?Fc\s+(\d+)\s+Hz.*?Gain\s+([-+]?\d+\.?\d*)\s*dB`)
)

func parseIndex(b []byte) ([]Meta, error) {
	var out []Meta
	scanner := bufio.NewScanner(strings.NewReader(string(b)))
	for scanner.Scan() {
		m := indexLink.FindStringSubmatch(scanner.Text())
		if len(m) != 3 {
			continue
		}
		name, path := m[1], m[2]
		if s, err := url.QueryUnescape(name); err == nil {
			name = s
		}
		parts := strings.Split(path, "/")
		var source, typ string
		if len(parts) >= 2 {
			source, typ = parts[0], parts[1]
			if s, err := url.QueryUnescape(source); err == nil {
				source = s
			}
			if s, err := url.QueryUnescape(typ); err == nil {
				typ = s
			}
		}
		out = append(out, Meta{Name: name, Path: path, Source: source, Type: typ})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("eq: AutoEQ index is empty")
	}
	return out, nil
}

func parseProfile(m Meta, b []byte) (*Profile, error) {
	content := string(b)
	pm := preampRE.FindStringSubmatch(content)
	if len(pm) < 2 {
		return nil, errors.New("eq: AutoEQ profile has no preamp")
	}
	preamp, err := strconv.ParseFloat(pm[1], 64)
	if err != nil {
		return nil, errors.New("eq: AutoEQ profile has a bad preamp")
	}
	matches := filterRE.FindAllStringSubmatch(content, -1)
	if len(matches) != 10 {
		return nil, fmt.Errorf("eq: AutoEQ profile has %d bands, want 10", len(matches))
	}
	bands := make([]float64, 10)
	for i, match := range matches {
		gain, err := strconv.ParseFloat(match[2], 64)
		if err != nil {
			return nil, fmt.Errorf("eq: AutoEQ profile band %d is invalid", i+1)
		}
		bands[i] = gain
	}
	return &Profile{Meta: m, Preamp: preamp, Bands: bands}, nil
}

func loadJSON[T any](path string, ttl time.Duration) (T, error) {
	var zero T
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) > ttl {
		return zero, errors.New("eq: cache missing or stale")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return zero, err
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return zero, err
	}
	return v, nil
}

func saveJSON(path string, v any) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, b, 0o644)
}

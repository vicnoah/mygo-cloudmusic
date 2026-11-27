package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"sync"

	"golang.org/x/mod/semver"
	"waygu-music/internal/githubdownload"
)

const updateRepo = "https://github.com/vicnoah/mygo-cloudmusic/releases/"
const updateKey = "j48t3n7ieAE6ZSiAqdQVpQWSdlsg1BPoxwym3bXqXJ8="

type updateFile struct {
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	Signature string `json:"signature"`
}
type updateManifest struct {
	updateFile
	Version string       `json:"version"`
	Deltas  []updateFile `json:"deltas"`
}
type githubUpdateSource struct {
	client *http.Client
	key    string
	mu     sync.Mutex
	files  map[string]updateFile
}

func (s *githubUpdateSource) read(ctx context.Context, raw string, limit int64, validate func([]byte) error) ([]byte, error) {
	var data []byte
	err := githubdownload.Try(ctx, s.client, raw, func(resp *http.Response) error {
		b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
		if err != nil {
			return err
		}
		if int64(len(b)) > limit {
			return errors.New("update metadata too large")
		}
		if err = validate(b); err != nil {
			return err
		}
		data = b
		return nil
	})
	return data, err
}
func (s *githubUpdateSource) Manifest(ctx context.Context, pre bool) (io.ReadCloser, error) {
	base := updateRepo + "latest/download/"
	if pre {
		best := ""
		for page := 1; page <= 10; page++ {
			var releases []struct {
				Tag        string `json:"tag_name"`
				Draft      bool   `json:"draft"`
				Prerelease bool   `json:"prerelease"`
			}
			_, err := s.read(ctx, fmt.Sprintf("https://api.github.com/repos/vicnoah/mygo-cloudmusic/releases?per_page=100&page=%d", page), 64<<20, func(b []byte) error { return json.Unmarshal(b, &releases) })
			if err != nil {
				return nil, err
			}
			stable := false
			for _, r := range releases {
				if r.Draft || !semver.IsValid(r.Tag) {
					continue
				}
				if best == "" || semver.Compare(r.Tag, best) > 0 {
					best = r.Tag
				}
				if !r.Prerelease {
					stable = true
				}
			}
			if stable || len(releases) < 100 {
				break
			}
		}
		if best == "" {
			return nil, errors.New("no published update")
		}
		base = updateRepo + "download/" + best + "/"
	}
	var m updateManifest
	data, err := s.read(ctx, base+"update-"+runtime.GOOS+"-"+runtime.GOARCH+".json", 1<<20, func(b []byte) error {
		m = updateManifest{}
		if err := json.Unmarshal(b, &m); err != nil {
			return err
		}
		if m.Version == "" || m.URL == "" || m.Signature == "" {
			return errors.New("incomplete update manifest")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	files := map[string]updateFile{m.URL: m.updateFile}
	for _, d := range m.Deltas {
		files[d.URL] = d
	}
	s.mu.Lock()
	if s.files == nil {
		s.files = make(map[string]updateFile)
	}
	for url, file := range files {
		s.files[url] = file
	}
	s.mu.Unlock()
	return io.NopCloser(bytes.NewReader(data)), nil
}

type temporaryUpdate struct {
	*os.File
	path string
}

func (f *temporaryUpdate) Close() error {
	err := f.File.Close()
	return errors.Join(err, os.Remove(f.path))
}
func (s *githubUpdateSource) Open(ctx context.Context, raw string) (io.ReadCloser, error) {
	s.mu.Lock()
	meta, ok := s.files[raw]
	s.mu.Unlock()
	if !ok || meta.Size <= 0 || meta.Size > 1<<30 {
		return nil, errors.New("invalid update file")
	}
	sig, err := base64.StdEncoding.DecodeString(meta.Signature)
	if err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(s.key)
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp("", "wagyu-update-*")
	if err != nil {
		return nil, err
	}
	result := &temporaryUpdate{f, f.Name()}
	err = githubdownload.Try(ctx, s.client, raw, func(resp *http.Response) error {
		if err := f.Truncate(0); err != nil {
			return err
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		hash := sha256.New()
		n, err := io.Copy(io.MultiWriter(f, hash), io.LimitReader(resp.Body, meta.Size+1))
		if err != nil {
			return err
		}
		if n != meta.Size || !ed25519.Verify(key, hash.Sum(nil), sig) {
			return errors.New("update size or signature mismatch")
		}
		return nil
	})
	if err != nil {
		result.Close()
		return nil, err
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		result.Close()
		return nil, err
	}
	return result, nil
}

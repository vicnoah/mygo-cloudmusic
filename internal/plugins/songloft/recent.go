package songloft

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/storage"
)

const (
	recentFileName = "songloft_recent.json"
	maxRecentSongs = 200
)

type RecentStore struct {
	mu     sync.RWMutex
	path   string
	tracks []pluginapi.Track
}

func recentFilePath(name string) string {
	if path, err := storage.LegacyDataPath(name); err == nil {
		return path
	}
	return filepath.Join(os.TempDir(), name)
}

func NewRecentStore() *RecentStore {
	s := &RecentStore{
		path: recentFilePath(recentFileName),
	}
	s.load()
	return s
}

func (s *RecentStore) load() {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path)
	if err != nil {
		s.tracks = nil
		return
	}
	var data struct {
		Tracks []pluginapi.Track `json:"tracks"`
	}
	if err := json.Unmarshal(b, &data); err == nil {
		s.tracks = data.Tracks
		if len(s.tracks) > maxRecentSongs {
			s.tracks = s.tracks[:maxRecentSongs]
		}
	}
}

func (s *RecentStore) saveLocked() {
	if s.path == "" {
		return
	}
	data := struct {
		Tracks []pluginapi.Track `json:"tracks"`
	}{
		Tracks: s.tracks,
	}
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err == nil {
		_ = os.Rename(tmp, s.path)
	}
}

func (s *RecentStore) Add(tr pluginapi.Track) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Remove if already exists (deduplicate)
	filtered := make([]pluginapi.Track, 0, len(s.tracks)+1)
	for _, item := range s.tracks {
		if item.ID != tr.ID {
			filtered = append(filtered, item)
		}
	}

	// 2. Prepend at index 0 (newest first)
	s.tracks = append([]pluginapi.Track{tr}, filtered...)

	// 3. Limit to maxRecentSongs
	if len(s.tracks) > maxRecentSongs {
		s.tracks = s.tracks[:maxRecentSongs]
	}

	s.saveLocked()
}

func (s *RecentStore) Remove(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := make([]pluginapi.Track, 0, len(s.tracks))
	for _, item := range s.tracks {
		if item.ID != id {
			filtered = append(filtered, item)
		}
	}
	if len(filtered) != len(s.tracks) {
		s.tracks = filtered
		s.saveLocked()
	}
}

func (s *RecentStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tracks = nil
	s.saveLocked()
}

func (s *RecentStore) Slice(offset, limit int) ([]pluginapi.Track, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := len(s.tracks)
	if offset < 0 || offset >= total || limit <= 0 {
		return nil, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	out := make([]pluginapi.Track, end-offset)
	copy(out, s.tracks[offset:end])
	return out, total
}

func (s *RecentStore) Total() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tracks)
}

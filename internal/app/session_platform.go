package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/waygu-music-plugin/sdk/pluginapi"

	"waygu-music/internal/player"
)

// Platform session extension seam.
//
// Keep only session behavior needed when the active provider changes here.
// The upstream session format and normal restore flow remain in session.go.

// clearSessionState drops the timers and bookkeeping of the old activation.
// Saved queues stay in their platform's own file.
func (a *app) clearSessionState() {
	a.savedSession = ""
	a.sessionDue = false
	a.restored = false
}

func platformSessionPath(id pluginapi.ID) (string, error) {
	path, err := sessionPath()
	if err != nil {
		return "", err
	}
	// Encode IDs so instance names cannot escape the directory or alias a
	// different instance on case-insensitive filesystems.
	return filepath.Join(filepath.Dir(path), "sessions", hex.EncodeToString([]byte(id))+".json"), nil
}

type legacySubsonicSession struct {
	SHA256  string       `json:"sha256"`
	Profile pluginapi.ID `json:"profile,omitempty"`
}

// rememberLegacySubsonicSession binds the shared old source to the profile
// selected before any switch or fallback can change it. The original file
// remains available to older releases, including when its owner is unknown.
func (s *platformSettings) rememberLegacySubsonicSession() {
	if s.LegacySubsonicSession != nil {
		return
	}
	path, err := sessionPath()
	if err != nil {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var session player.Session
	if json.Unmarshal(data, &session) != nil {
		return
	}
	for _, track := range session.Queue {
		if track.Source == "subsonic" {
			digest := sha256.Sum256(data)
			record := legacySubsonicSession{SHA256: hex.EncodeToString(digest[:])}
			switch s.Platform {
			case "navidrome", "jellyfin", "gonic", "subsonic":
				record.Profile = pluginapi.ID(s.Platform)
			}
			s.LegacySubsonicSession = &record
			return
		}
	}
}

func (s *platformSettings) assignLegacySubsonicSource(data []byte, queue []pluginapi.Track) bool {
	record := s.LegacySubsonicSession
	var profile pluginapi.ID
	if record != nil && record.Profile != "" {
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) == record.SHA256 {
			profile = record.Profile
		}
	}
	for i := range queue {
		if queue[i].Source == "subsonic" {
			if profile == "" {
				return false
			}
			queue[i].Source = string(profile)
		}
	}
	return true
}

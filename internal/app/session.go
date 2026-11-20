package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"

	"waygu-music/internal/player"
)

// The original session.json remains available to its owner and to older
// releases. New sessions are saved per platform.

func sessionPath() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "session.json"), nil
}

// saveSession keeps where playback is, if it moved on since last kept.
func (a *app) saveSession() {
	s, ok := a.pl.Session()
	if !ok {
		return
	}
	b, err := json.Marshal(s)
	if err != nil || string(b) == a.savedSession {
		return
	}
	path, err := platformSessionPath(a.platformID)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	if os.WriteFile(path+".part", b, 0o600) == nil && os.Rename(path+".part", path) == nil {
		a.savedSession = string(b)
	}
}

// sessionEvery is how long after playback changed, and how often as it
// plays, the session is kept.
const sessionEvery = 30 * time.Second

// keepSession saves the session a while after playback changed, and every
// while again as it plays. Paused, nothing is due, and the app sleeps.
func (a *app) keepSession() {
	if a.sessionDue {
		return
	}
	a.sessionDue = true
	generation := a.platformGeneration
	sessionAfter(sessionEvery, func() {
		if a.platformGeneration != generation {
			return
		}
		a.sessionDue = false
		a.saveSession()
		if a.pl.Playing() {
			a.keepSession()
		}
	})
}

// sessionAfter runs fn on the main thread after d, without redrawing a
// window as Update would.
var sessionAfter = func(d time.Duration, fn func()) {
	time.AfterFunc(d, func() { mygo.RunOnMain(fn) })
}

// LoadSession offers the saved queue once, after the active plugin is ready.
func (p playbackContinuity) LoadSession() (player.Session, bool) {
	if !p.active() || p.a.restored {
		return player.Session{}, false
	}
	a := p.a
	a.restored = true
	path, err := platformSessionPath(a.platformID)
	if err != nil {
		return player.Session{}, false
	}
	b, err := os.ReadFile(path)
	legacy := os.IsNotExist(err)
	if legacy {
		if legacyPath, pathErr := sessionPath(); pathErr == nil {
			b, err = os.ReadFile(legacyPath)
		}
	}
	if err != nil {
		return player.Session{}, false
	}
	var s player.Session
	if json.Unmarshal(b, &s) != nil || (legacy && !a.settings.assignLegacySubsonicSource(b, s.Queue)) ||
		!a.ownsTracksFor(a.platformID, s.Queue) {
		return player.Session{}, false
	}
	a.savedSession = string(b)
	return s, true
}

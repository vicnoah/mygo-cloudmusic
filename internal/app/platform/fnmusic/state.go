// Package fnmusic contains the application-facing state and views of the
// fnOS Music platform. Its backend implementation lives in
// internal/platform/fnmusic; this package owns the platform session without
// importing the root app package.
package fnmusic

import (
	"context"
	"errors"
	"sync"

	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/fnmusic"
)

// State is the part of a Feiniu session that belongs to the platform. It is
// deliberately independent of windows, routes, MyGo UI elements and the
// shared player; the root app supplies those through a Host when rendering.
type State struct {
	mu sync.RWMutex

	client    *backend.Client
	provider  *backend.Provider
	err       error
	playlists []netease.Playlist
	liked     map[int64]bool
	likeBusy  map[int64]bool
	roamID    string
	catalog   *Catalog
}

// NewState returns an empty disconnected platform session.
func NewState() *State {
	s := &State{liked: make(map[int64]bool), likeBusy: make(map[int64]bool)}
	s.catalog = NewCatalog(s, nil)
	return s
}

// Catalog returns the platform-owned lazy catalog state.
func (s *State) Catalog() *Catalog {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.catalog == nil {
		s.catalog = NewCatalog(s, nil)
	}
	return s.catalog
}

// SetPost supplies the owner goroutine used by catalog page completions.
func (s *State) SetPost(post func(func())) {
	if s == nil {
		return
	}
	s.Catalog().Post = post
}

// Client returns the current backend client, if connected.
func (s *State) Client() *backend.Client {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client
}

// Provider returns the shared platform adapter for the current session.
func (s *State) Provider() *backend.Provider {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.provider
}

// Connected reports whether a client is attached.
func (s *State) Connected() bool { return s.Client() != nil }

// Error returns the latest connection or account error.
func (s *State) Error() error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}

// SetError records an account or connection error without coupling the
// platform package to the root app's toast and error presentation.
func (s *State) SetError(err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
}

// Attach makes client the active session. It does not connect or log in; the
// caller controls that lifecycle and can publish the result through Host.
func (s *State) Attach(client *backend.Client) error {
	if s == nil {
		return errors.New("fnmusic: nil state")
	}
	if client == nil {
		return errors.New("fnmusic: nil client")
	}
	s.mu.Lock()
	s.client = client
	s.provider = backend.Wrap(client)
	s.err = nil
	s.playlists = nil
	s.roamID = ""
	s.catalog = NewCatalog(s, nil)
	if s.liked == nil {
		s.liked = make(map[int64]bool)
	}
	if s.likeBusy == nil {
		s.likeBusy = make(map[int64]bool)
	}
	s.mu.Unlock()
	return nil
}

// Detach clears the active session and returns its client for logout. The
// platform state is reset together, so a later login cannot see stale lists.
func (s *State) Detach() *backend.Client {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	client := s.client
	s.client, s.provider, s.err = nil, nil, nil
	s.playlists = nil
	s.roamID = ""
	s.catalog = NewCatalog(s, nil)
	s.liked = make(map[int64]bool)
	s.likeBusy = make(map[int64]bool)
	s.mu.Unlock()
	return client
}

// Playlists returns a copy of the account playlists held by this session.
func (s *State) Playlists() []netease.Playlist {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]netease.Playlist(nil), s.playlists...)
}

// SetPlaylists replaces the account playlists with a copy owned by State.
func (s *State) SetPlaylists(lists []netease.Playlist) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.playlists = append([]netease.Playlist(nil), lists...)
	s.mu.Unlock()
}

// Liked reports the optimistic liked state of a track.
func (s *State) Liked(id int64) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.liked[id]
}

// SetLiked updates the optimistic liked state.
func (s *State) SetLiked(id int64, liked bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if liked {
		s.liked[id] = true
	} else {
		delete(s.liked, id)
	}
	s.mu.Unlock()
}

// LikeBusy reports whether a like operation is in flight for id.
func (s *State) LikeBusy(id int64) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.likeBusy[id]
}

// SetLikeBusy marks a like operation as in flight or complete.
func (s *State) SetLikeBusy(id int64, busy bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if busy {
		s.likeBusy[id] = true
	} else {
		delete(s.likeBusy, id)
	}
	s.mu.Unlock()
}

// SetRoamID and RoamID keep the service's roaming cursor in platform state.
func (s *State) SetRoamID(id string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.roamID = id
	s.mu.Unlock()
}

func (s *State) RoamID() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.roamID
}

// CheckContext lets asynchronous platform jobs reject stale or canceled work
// before invoking a backend operation.
func (s *State) CheckContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.Connected() {
		return backend.ErrSignedOut
	}
	return nil
}

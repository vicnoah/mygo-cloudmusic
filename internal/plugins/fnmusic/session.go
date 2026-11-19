package fnmusic

import (
	"context"
	"errors"

	"waygu-music/internal/pluginapi"
	backend "waygu-music/internal/plugins/fnmusic/internal/client"
)

// connect signs in and publishes the client for the current attempt.
func (s *State) connect(ctx context.Context, cfg backend.Config, gen uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c, err := backend.New(cfg)
	if err != nil {
		if s.connectCurrent(gen) {
			s.SetError(err)
		}
		return err
	}
	if err := c.WithContext(ctx).Connect(); err != nil {
		if s.connectCurrent(gen) {
			s.SetError(err)
		}
		return err
	}
	if !s.connectCurrent(gen) {
		c.Logout()
		return context.Canceled
	}
	if err := ctx.Err(); err != nil {
		c.Logout()
		return err
	}
	if err := c.WithContext(ctx).Login(); err != nil {
		if s.connectCurrent(gen) {
			s.SetError(err)
		}
		return err
	}
	if err := ctx.Err(); err != nil {
		c.Logout()
		return err
	}
	if !s.connectCurrent(gen) {
		c.Logout()
		return context.Canceled
	}
	// Capability discovery is optional for older servers. Finish it before
	// publishing so a canceled probe cannot leave a partially loaded session.
	caps, probeErr := c.WithContext(ctx).Capabilities()
	if err := ctx.Err(); err != nil {
		c.Logout()
		return err
	}
	if !s.attachCurrent(c, gen, caps, probeErr == nil) {
		c.Logout()
		return context.Canceled
	}
	return nil
}

// LoadPlaylists reads the account playlists through the platform provider and
// stores a private copy in the session.
func (s *State) LoadPlaylists(ctx context.Context) ([]pluginapi.Playlist, error) {
	c, gen, err := s.activeClient(ctx)
	if err != nil {
		return nil, err
	}
	lists, err := c.WithContext(ctx).Playlists()
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil || s.client != c || s.connectGen != gen {
		return nil, context.Canceled
	}
	if err != nil {
		s.err = err
		return nil, err
	}
	s.playlists = make([]pluginapi.Playlist, len(lists))
	copy(s.playlists, lists)
	s.err = nil
	return lists, nil
}

// SetFavorite updates the optimistic state and persists the change through
// the platform backend. A failed request rolls the optimistic state back.
func (s *State) SetFavorite(ctx context.Context, tr *pluginapi.Track, liked bool) error {
	if s == nil || tr == nil {
		return errors.New("fnmusic: nil track")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	c, gen := s.client, s.connectGen
	if c == nil {
		s.mu.Unlock()
		return backend.ErrSignedOut
	}
	if s.likeBusy[tr.ID] {
		s.mu.Unlock()
		return nil
	}
	old := s.liked[tr.ID]
	s.setLikedLocked(tr.ID, liked)
	s.likeBusy[tr.ID] = true
	s.mu.Unlock()

	err := c.WithContext(ctx).SetFavorite(tr.Ref, liked)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != c {
		return context.Canceled
	}
	// Switching platforms keeps the client, including optimistic favorites.
	// Resolve the pending state on that client even if the view has left.
	delete(s.likeBusy, tr.ID)
	if err != nil {
		s.setLikedLocked(tr.ID, old)
	}
	if gen != s.connectGen {
		return context.Canceled
	}
	return err
}

// ReportPlay records a started track in the service's play history.
func (s *State) ReportPlay(ctx context.Context, tr *pluginapi.Track) error {
	if s == nil || tr == nil || tr.Ref == "" {
		return nil
	}
	c, gen, err := s.activeClient(ctx)
	if err != nil {
		return err
	}
	if err = c.WithContext(ctx).ReportPlay(tr.Ref); err != nil {
		return err
	}
	if !s.clientCurrent(c, gen) {
		return context.Canceled
	}
	return nil
}

// RemoveFromHistory removes one entry from the server play history and keeps
// the operation tied to the session that issued it.
func (s *State) RemoveFromHistory(ctx context.Context, tr pluginapi.Track) error {
	if s == nil {
		return errors.New("fnmusic: nil state")
	}
	c, gen, err := s.activeClient(ctx)
	if err != nil {
		return err
	}
	if err = c.WithContext(ctx).RemoveFromHistory([]string{tr.Ref}); err != nil {
		return err
	}
	if !s.clientCurrent(c, gen) {
		return context.Canceled
	}
	return nil
}

// StartRoam asks the service for the first roaming track and records its
// cursor in the platform session.
func (s *State) StartRoam(ctx context.Context) (pluginapi.Track, error) {
	c, gen, err := s.activeClient(ctx)
	if err != nil {
		return pluginapi.Track{}, err
	}
	tr, id, err := c.WithContext(ctx).RoamStart()
	if err != nil {
		if s.clientCurrent(c, gen) {
			s.SetError(err)
		}
		return pluginapi.Track{}, err
	}
	if !s.clientCurrent(c, gen) {
		return pluginapi.Track{}, context.Canceled
	}
	s.SetRoamID(id)
	s.SetError(nil)
	return tr, nil
}

// NextRoam advances the service roaming cursor.
func (s *State) NextRoam(ctx context.Context) (pluginapi.Track, error) {
	c, gen, err := s.activeClient(ctx)
	if err != nil {
		return pluginapi.Track{}, err
	}
	tr, id, err := c.WithContext(ctx).RoamNext(s.RoamID())
	if err != nil {
		if s.clientCurrent(c, gen) {
			s.SetError(err)
		}
		return pluginapi.Track{}, err
	}
	if !s.clientCurrent(c, gen) {
		return pluginapi.Track{}, context.Canceled
	}
	s.SetRoamID(id)
	s.SetError(nil)
	return tr, nil
}

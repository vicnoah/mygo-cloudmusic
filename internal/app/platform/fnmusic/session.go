package fnmusic

import (
	"context"
	"errors"

	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/fnmusic"
)

// Connect creates and signs in a new Feiniu client, then publishes it as the
// active session. The caller owns configuration persistence and presentation
// of errors; this package only owns platform lifecycle.
func (s *State) Connect(ctx context.Context, cfg backend.Config) error {
	if s == nil {
		return errors.New("fnmusic: nil state")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c, err := backend.New(cfg)
	if err != nil {
		s.SetError(err)
		return err
	}
	if err := c.Connect(); err != nil {
		s.SetError(err)
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.Login(); err != nil {
		s.SetError(err)
		return err
	}
	if err := s.Attach(c); err != nil {
		s.SetError(err)
		return err
	}
	return nil
}

// Close logs out and detaches the current session. It is safe to call when
// the platform is already disconnected.
func (s *State) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c := s.Detach()
	if c == nil {
		return nil
	}
	c.Logout()
	return nil
}

// LoadPlaylists reads the account playlists through the platform provider and
// stores a private copy in the session.
func (s *State) LoadPlaylists(ctx context.Context) ([]netease.Playlist, error) {
	if err := s.CheckContext(ctx); err != nil {
		return nil, err
	}
	lists, err := s.Client().Playlists()
	if err != nil {
		s.SetError(err)
		return nil, err
	}
	s.SetPlaylists(lists)
	s.SetError(nil)
	return s.Playlists(), nil
}

// SetFavorite updates the optimistic state and persists the change through
// the platform backend. A failed request rolls the optimistic state back.
func (s *State) SetFavorite(ctx context.Context, tr *netease.Track, liked bool) error {
	if tr == nil {
		return errors.New("fnmusic: nil track")
	}
	if err := s.CheckContext(ctx); err != nil {
		return err
	}
	old := s.Liked(tr.ID)
	s.SetLiked(tr.ID, liked)
	s.SetLikeBusy(tr.ID, true)
	err := s.Client().SetFavorite(tr.Ref, liked)
	s.SetLikeBusy(tr.ID, false)
	if err != nil {
		s.SetLiked(tr.ID, old)
	}
	return err
}

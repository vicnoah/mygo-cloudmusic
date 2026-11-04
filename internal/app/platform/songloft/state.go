package songloft

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/songloft"
)

type State struct {
	mu         sync.RWMutex
	client     *backend.Client
	provider   *backend.Provider
	config     backend.Config
	connectGen uint64
	connecting bool
	err        error
	post       func(func())

	playlists []netease.Playlist
	liked     map[int64]bool
	likeBusy  map[int64]bool

	catalog *Catalog
}

func NewState() *State {
	s := &State{
		liked:    make(map[int64]bool),
		likeBusy: make(map[int64]bool),
	}
	s.catalog = NewCatalog(s, nil)
	return s
}

func (s *State) Catalog() *Catalog {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.catalog == nil {
		s.catalog = NewCatalog(s, s.post)
	}
	return s.catalog
}

func (s *State) SetPost(post func(func())) {
	if s == nil {
		return
	}
	if post == nil {
		post = func(fn func()) { fn() }
	}
	s.mu.Lock()
	s.post = post
	if s.catalog != nil {
		s.catalog.Post = post
	}
	s.mu.Unlock()
}

func (s *State) Generation() uint64 {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connectGen
}

func (s *State) Config() backend.Config {
	if s == nil {
		return backend.Config{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

func (s *State) SetConfig(cfg backend.Config) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.config = cfg
	s.mu.Unlock()
}

func (s *State) Connected() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.provider != nil && s.provider.Connected()
}

func (s *State) Connecting() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connecting
}

func (s *State) Error() error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}

func (s *State) SetError(err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

func (s *State) Provider() *backend.Provider {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.provider
}

func (s *State) Client() *backend.Client {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.provider != nil {
		return s.provider.Client()
	}
	return nil
}

func (s *State) setConnecting(gen uint64, connecting bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if gen != s.connectGen {
		return
	}
	s.connecting = connecting
}

func (s *State) Connect(ctx context.Context, cfg backend.Config) error {
	if s == nil {
		return backend.ErrNotConnected
	}
	s.mu.Lock()
	s.connectGen++
	gen := s.connectGen
	s.connecting = true
	s.err = nil
	s.config = cfg
	s.mu.Unlock()

	p := backend.NewProvider(cfg)
	err := p.Connect(ctx)

	s.mu.Lock()
	if gen != s.connectGen {
		s.mu.Unlock()
		return context.Canceled
	}
	s.connecting = false
	if err != nil {
		s.err = err
		s.provider = nil
		s.client = nil
		s.mu.Unlock()
		return err
	}
	s.provider = p
	s.client = p.Client()
	s.catalog.Reset()
	s.mu.Unlock()

	_ = s.ReloadPlaylists(ctx)
	return nil
}

func (s *State) Disconnect() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	s.connectGen++
	s.connecting = false
	s.err = nil
	if s.provider != nil {
		_ = s.provider.Close()
		s.provider = nil
	}
	s.client = nil
	s.playlists = nil
	s.liked = make(map[int64]bool)
	s.likeBusy = make(map[int64]bool)
	if s.catalog != nil {
		s.catalog.Reset()
	}
	s.mu.Unlock()
	return nil
}

func (s *State) ReloadPlaylists(ctx context.Context) error {
	cli := s.Client()
	if cli == nil {
		return backend.ErrNotConnected
	}
	res, err := cli.ListPlaylists(ctx, 100, 0)
	if err != nil {
		return err
	}

	pls := make([]netease.Playlist, 0, len(res.Playlists))
	for _, p := range res.Playlists {
		if p.ID == 1 || p.ID == 2 {
			continue // Hide built-in favorites and radio from custom playlist section
		}
		pls = append(pls, backend.ToPlaylist(&p))
	}

	s.mu.Lock()
	s.playlists = pls
	s.mu.Unlock()
	return nil
}

func (s *State) Playlists() []netease.Playlist {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]netease.Playlist, len(s.playlists))
	copy(out, s.playlists)
	return out
}

func (s *State) FindPlaylistByRef(ref string) *netease.Playlist {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.playlists {
		if s.playlists[i].Ref == ref {
			pl := s.playlists[i]
			return &pl
		}
	}
	return nil
}

func (s *State) Liked(id int64) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	fav, ok := s.liked[id]
	provider := s.provider
	s.mu.RUnlock()
	if ok {
		return fav
	}
	if provider != nil && provider.IsFav(id) {
		return true
	}
	return false
}

func (s *State) LikeBusy(id int64) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.likeBusy[id]
}

func (s *State) SetLiked(id int64, liked bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.liked == nil {
		s.liked = make(map[int64]bool)
	}
	s.liked[id] = liked
	s.mu.Unlock()
}

func (s *State) LikedTrack(tr *netease.Track) bool {
	if s == nil || tr == nil {
		return false
	}
	s.mu.RLock()
	fav, ok := s.liked[tr.ID]
	provider := s.provider
	s.mu.RUnlock()
	if ok {
		return fav
	}
	if provider != nil {
		if sid, err := strconv.ParseInt(tr.Ref, 10, 64); err == nil && provider.IsFav(sid) {
			return true
		}
	}
	return tr.Fav
}

func (s *State) markLiked(tracks []netease.Track) {
	if s == nil || len(tracks) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range tracks {
		if fav, ok := s.liked[tracks[i].ID]; ok {
			tracks[i].Fav = fav
		} else if s.provider != nil {
			if sid, err := strconv.ParseInt(tracks[i].Ref, 10, 64); err == nil && s.provider.IsFav(sid) {
				tracks[i].Fav = true
				s.liked[tracks[i].ID] = true
			} else {
				s.liked[tracks[i].ID] = tracks[i].Fav
			}
		} else {
			s.liked[tracks[i].ID] = tracks[i].Fav
		}
	}
}

func (s *State) SetFavorite(ctx context.Context, tr *netease.Track, liked bool) error {
	if s == nil || tr == nil {
		return errors.New("songloft: nil track")
	}
	s.mu.Lock()
	if s.likeBusy[tr.ID] {
		s.mu.Unlock()
		return nil
	}
	s.likeBusy[tr.ID] = true
	provider := s.provider
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.likeBusy, tr.ID)
		s.mu.Unlock()
	}()

	if provider == nil {
		return backend.ErrNotConnected
	}

	if err := provider.SetLiked(ctx, tr, liked); err != nil {
		return err
	}

	s.mu.Lock()
	if s.liked == nil {
		s.liked = make(map[int64]bool)
	}
	s.liked[tr.ID] = liked
	tr.Fav = liked
	cat := s.catalog
	post := s.post
	s.mu.Unlock()

	if cat != nil {
		cat.FavoriteChanged(tr.ID, liked)
	}
	if post != nil {
		post(func() {})
	}

	return nil
}

func (s *State) CreatePlaylist(ctx context.Context, name string) (netease.Playlist, error) {
	provider := s.Provider()
	if provider == nil {
		return netease.Playlist{}, backend.ErrNotConnected
	}
	pl, err := provider.CreatePlaylist(ctx, name)
	if err != nil {
		return netease.Playlist{}, err
	}
	_ = s.ReloadPlaylists(ctx)
	return pl, nil
}

func (s *State) RenamePlaylist(ctx context.Context, pl netease.Playlist, name string) error {
	provider := s.Provider()
	if provider == nil {
		return backend.ErrNotConnected
	}
	if err := provider.RenamePlaylist(ctx, pl, name); err != nil {
		return err
	}
	_ = s.ReloadPlaylists(ctx)
	return nil
}

func (s *State) DeletePlaylist(ctx context.Context, pl netease.Playlist) error {
	provider := s.Provider()
	if provider == nil {
		return backend.ErrNotConnected
	}
	if err := provider.DeletePlaylist(ctx, pl); err != nil {
		return err
	}
	_ = s.ReloadPlaylists(ctx)
	return nil
}

func (s *State) AddToPlaylist(ctx context.Context, pl netease.Playlist, tracks []netease.Track) error {
	provider := s.Provider()
	if provider == nil {
		return backend.ErrNotConnected
	}
	if err := provider.AddToPlaylist(ctx, pl, tracks); err != nil {
		return err
	}
	_ = s.ReloadPlaylists(ctx)
	return nil
}

func (s *State) RemoveFromPlaylist(ctx context.Context, pl netease.Playlist, tracks []netease.Track) error {
	provider := s.Provider()
	if provider == nil {
		return backend.ErrNotConnected
	}
	if err := provider.RemoveFromPlaylist(ctx, pl, tracks); err != nil {
		return err
	}
	_ = s.ReloadPlaylists(ctx)
	return nil
}

package subsonic

import (
	"context"
	"errors"
	"sync"

	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/subsonic"
)

// State holds the live connection, playlist cache, and favorite statuses.
type State struct {
	mu sync.RWMutex

	profile    backend.Profile
	client     *backend.Client
	provider   *backend.Provider
	config     backend.Config
	connectGen uint64
	connecting bool
	err        error
	post       func(func())

	playlists     []netease.Playlist
	liked         map[int64]bool
	likeBusy      map[int64]bool
	likedAlbums   map[string]bool
	albumLikeBusy map[string]bool
	transcode     appplatform.TranscodeSettings

	catalog *Catalog
}

func NewState(profiles ...backend.Profile) *State {
	prof := backend.ProfileSubsonic
	if len(profiles) > 0 && profiles[0].ID != "" {
		prof = profiles[0]
	}
	s := &State{
		profile:       prof,
		liked:         make(map[int64]bool),
		likeBusy:      make(map[int64]bool),
		likedAlbums:   make(map[string]bool),
		albumLikeBusy: make(map[string]bool),
	}
	s.catalog = NewCatalog(s, nil)
	return s
}

func (s *State) Profile() backend.Profile {
	if s == nil {
		return backend.ProfileSubsonic
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.profile
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

func (s *State) Generation() uint64 {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connectGen
}

func (s *State) Connecting() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connecting
}

func (s *State) setConnecting(gen uint64, connecting bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if gen != s.connectGen {
		return
	}
	s.connecting = connecting
}

func (s *State) Connected() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client != nil && s.client.Connected()
}

func (s *State) Client() *backend.Client {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client
}

func (s *State) SetClient(client *backend.Client) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.client = client
	if client != nil {
		s.provider = backend.Wrap(client, s.profile)
	} else {
		s.provider = nil
	}
}

func (s *State) Provider() *backend.Provider {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.provider
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
	s.err = err
	s.mu.Unlock()
}

func (s *State) Playlists() []netease.Playlist {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.playlists
}

func (s *State) LikedTrack(tr *netease.Track) bool {
	if s == nil || tr == nil {
		return false
	}
	return s.Liked(tr.ID)
}

func (s *State) Liked(id int64) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.liked[id]
}

func (s *State) SetLikedMemory(id int64, liked bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.liked[id] = liked
	s.mu.Unlock()
}

func (s *State) RecordTracks(items []netease.Track) {
	if s == nil || len(items) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, tr := range items {
		if tr.Fav {
			s.liked[tr.ID] = true
		}
	}
}

func (s *State) Transcode() appplatform.TranscodeSettings {
	if s == nil {
		return appplatform.TranscodeSettings{Mode: appplatform.TranscodeAuto}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	mode := s.transcode.Mode
	if mode == "" {
		mode = appplatform.TranscodeAuto
	}
	return appplatform.TranscodeSettings{Mode: mode, Bitrate: s.transcode.Bitrate}
}

func (s *State) SetTranscode(ts appplatform.TranscodeSettings) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.transcode = ts
	client := s.client
	s.mu.Unlock()
	if client != nil {
		client.SetTranscode(string(ts.Mode), ts.Bitrate)
	}
}

func (s *State) LikeBusy(id int64) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.likeBusy[id]
}

func (s *State) SetFavorite(ctx context.Context, tr *netease.Track, liked bool) error {
	if s == nil || tr == nil {
		return errors.New("subsonic: nil track")
	}
	provider := s.Provider()
	if provider == nil {
		return backend.ErrSignedOut
	}

	s.mu.Lock()
	s.likeBusy[tr.ID] = true
	prev := s.liked[tr.ID]
	s.liked[tr.ID] = liked
	s.mu.Unlock()

	err := provider.SetLiked(ctx, tr, liked)
	s.mu.Lock()
	delete(s.likeBusy, tr.ID)
	if err != nil {
		s.liked[tr.ID] = prev
	}
	s.mu.Unlock()

	return err
}

func (s *State) AlbumLiked(ref string) bool {
	if s == nil || ref == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if liked, ok := s.likedAlbums[ref]; ok {
		return liked
	}
	if s.client != nil {
		return s.client.IsAlbumStarred(ref)
	}
	return false
}

func (s *State) SetAlbumLikedMemory(ref string, liked bool) {
	if s == nil || ref == "" {
		return
	}
	s.mu.Lock()
	if s.likedAlbums == nil {
		s.likedAlbums = make(map[string]bool)
	}
	s.likedAlbums[ref] = liked
	client := s.client
	s.mu.Unlock()
	if client != nil {
		client.SetAlbumStarred(ref, liked)
	}
}

func (s *State) AlbumLikeBusy(ref string) bool {
	if s == nil || ref == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.albumLikeBusy[ref]
}

func (s *State) SetAlbumFavorite(ctx context.Context, ref string, liked bool) error {
	if s == nil || ref == "" {
		return errors.New("subsonic: empty album ref")
	}
	provider := s.Provider()
	if provider == nil {
		return backend.ErrSignedOut
	}

	s.mu.Lock()
	if s.albumLikeBusy == nil {
		s.albumLikeBusy = make(map[string]bool)
	}
	if s.likedAlbums == nil {
		s.likedAlbums = make(map[string]bool)
	}
	s.albumLikeBusy[ref] = true
	prev := s.likedAlbums[ref]
	s.likedAlbums[ref] = liked
	s.mu.Unlock()

	err := provider.SetAlbumLiked(ctx, ref, liked)

	s.mu.Lock()
	delete(s.albumLikeBusy, ref)
	if err != nil {
		s.likedAlbums[ref] = prev
	} else if s.client != nil {
		s.client.SetAlbumStarred(ref, liked)
	}
	s.mu.Unlock()

	if err == nil && s.catalog != nil {
		if s.catalog.AlbumSort == "starred" {
			s.catalog.Albums = nil
		}
	}
	return err
}

func (s *State) Connect(ctx context.Context, cfg backend.Config) error {
	if s == nil {
		return errors.New("subsonic: nil state")
	}

	s.mu.Lock()
	s.connectGen++
	gen := s.connectGen
	s.connecting = true
	s.err = nil
	s.config = cfg
	s.mu.Unlock()

	client := backend.NewClient(cfg, s.profile.ID)
	err := client.Connect(ctx)

	s.mu.Lock()
	if gen != s.connectGen {
		s.mu.Unlock()
		_ = client.Close()
		return errors.New("subsonic: connection cancelled")
	}
	s.connecting = false
	if err != nil {
		s.err = err
		s.client = nil
		s.provider = nil
		s.mu.Unlock()
		return err
	}
	s.client = client
	s.provider = backend.Wrap(client, s.profile)
	client.SetTranscode(string(s.transcode.Mode), s.transcode.Bitrate)
	if s.catalog != nil {
		s.catalog.Reset()
	}
	s.mu.Unlock()

	// Load playlists & starred songs in background
	go func() {
		_ = s.RefreshPlaylists(context.Background())
		_ = s.LoadLiked(context.Background())
	}()

	return nil
}

func (s *State) Cancel(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connectGen++
	s.connecting = false
	return nil
}

func (s *State) Disconnect(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	s.connectGen++
	s.connecting = false
	s.err = nil
	client := s.client
	s.client = nil
	s.provider = nil
	s.playlists = nil
	s.liked = make(map[int64]bool)
	if s.catalog != nil {
		s.catalog.Reset()
	}
	s.mu.Unlock()

	if client != nil {
		return client.Close()
	}
	return nil
}

func (s *State) RefreshPlaylists(ctx context.Context) error {
	provider := s.Provider()
	if provider == nil {
		return backend.ErrSignedOut
	}
	sc := provider.Client().Subsonic()
	if sc == nil {
		return backend.ErrSignedOut
	}
	playlists, err := sc.GetPlaylists(nil)
	if err != nil {
		return err
	}
	var out []netease.Playlist
	for _, p := range playlists {
		out = append(out, backend.ToPlaylistWithPlatform(s.profile.ID, p))
	}

	s.mu.Lock()
	s.playlists = out
	s.mu.Unlock()

	if s.post != nil {
		s.post(func() {})
	}
	return nil
}

func (s *State) LoadLiked(ctx context.Context) error {
	provider := s.Provider()
	if provider == nil {
		return backend.ErrSignedOut
	}
	sc := provider.Client().Subsonic()
	if sc == nil {
		return backend.ErrSignedOut
	}
	starred, err := sc.GetStarred2(nil)
	if err != nil {
		return err
	}

	s.mu.Lock()
	if s.liked == nil {
		s.liked = make(map[int64]bool)
	}
	if s.likedAlbums == nil {
		s.likedAlbums = make(map[string]bool)
	}
	for _, c := range starred.Song {
		s.liked[backend.IDWithPlatform(s.profile.ID, c.ID)] = true
	}
	for _, a := range starred.Album {
		s.likedAlbums[a.ID] = true
		if s.client != nil {
			s.client.SetAlbumStarred(a.ID, true)
		}
	}
	s.mu.Unlock()

	if s.post != nil {
		s.post(func() {})
	}
	return nil
}

package songloft

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"waygu-music/internal/pluginapi"
	backend "waygu-music/internal/plugins/songloft/internal/client"
)

type playlistEditHost struct {
	Host
	post func(func())
}

func (h playlistEditHost) Post(fn func())                     { h.post(fn) }
func (playlistEditHost) Presentation() pluginapi.Presentation { return pluginapi.Presentation{} }
func (playlistEditHost) Toast(string)                         {}

// Adding the first song to a cached empty playlist makes its next read include that song.
func TestAddToEmptyPlaylistRefreshesTracks(t *testing.T) {
	var added atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"access_token":"fixture","expires_in":3600}`)
	})
	mux.HandleFunc("GET /api/v1/playlists", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"playlists":[{"id":7,"name":"Test playlist"}],"total":1}`)
	})
	mux.HandleFunc("POST /api/v1/playlists/7/songs", func(http.ResponseWriter, *http.Request) {
		added.Store(true)
	})
	mux.HandleFunc("GET /api/v1/playlists/7/songs", func(w http.ResponseWriter, _ *http.Request) {
		if added.Load() {
			_, _ = io.WriteString(w, `{"songs":[{"id":9,"title":"Test song"}],"total":1}`)
			return
		}
		_, _ = io.WriteString(w, `{"songs":[],"total":0}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	posts := make(chan func(), 4)
	post := func(fn func()) { posts <- fn }
	apply := func() {
		t.Helper()
		select {
		case fn := <-posts:
			fn()
		case <-ctx.Done():
			t.Fatal("the playlist update did not complete")
		}
	}
	s := NewState()
	s.SetPost(post)
	if err := s.Connect(ctx, backend.Config{Address: server.URL, Username: "listener", Password: "fixture"}); err != nil {
		t.Fatal(err)
	}
	h := NewHome(s, playlistEditHost{post: post})
	h.scope = ctx
	playlists := s.Playlists()
	if len(playlists) != 1 {
		t.Fatalf("loaded %d playlists, want the fixture playlist", len(playlists))
	}
	pl := playlists[0]
	playlistTracks := func() *List { return s.Catalog().ListForRoute(pl.ID, "playlist", pl.Ref) }
	playlistTracks().Pages.At(0)
	apply()
	if n := playlistTracks().Pages.Total(); n != 0 {
		t.Fatalf("initial playlist has %d tracks, want empty", n)
	}
	h.RequestAddTracks(pl, []pluginapi.Track{{Ref: "9"}})
	apply()
	tracks := playlistTracks().Pages
	if tracks.Total() == 0 {
		t.Fatal("the playlist still reported the empty result after a successful add")
	}
	tracks.At(0)
	apply()
	if got := tracks.Peek(0); got == nil || got.Ref != "9" {
		t.Fatalf("updated playlist did not expose the added song: %+v", got)
	}
}

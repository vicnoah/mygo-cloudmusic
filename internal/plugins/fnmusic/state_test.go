package fnmusic

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"waygu-music/internal/pluginapi"
	backend "waygu-music/internal/plugins/fnmusic/internal/client"
)

// Attaching and detaching a session clears account state instead of leaking it to the next login.
func TestStateSessionLifecycle(t *testing.T) {
	s := NewState()
	c, err := backend.New(backend.Config{Address: "127.0.0.1:1", Username: "u", PassHash: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Attach(c); err != nil {
		t.Fatal(err)
	}
	s.SetLiked(7, true)
	s.SetPlaylists(nil)
	s.SetRoamID("r1")
	if !s.Connected() || !s.Liked(7) || s.RoamID() != "r1" || s.Provider() == nil {
		t.Fatal("attached state was not published")
	}
	if got := s.Detach(); got != c {
		t.Fatal("Detach returned a different client")
	}
	if s.Connected() || s.Liked(7) || s.RoamID() != "" || s.Provider() != nil {
		t.Fatal("detached state retained session data")
	}
}

// Replacing a session keeps catalog completions on the application owner goroutine.
func TestStateCatalogKeepsPost(t *testing.T) {
	s := NewState()
	post := func(func()) {}
	s.SetPost(post)
	if got := s.Catalog().Post; got == nil {
		t.Fatal("catalog lost owner post callback")
	}
	c, err := backend.New(backend.Config{Address: "127.0.0.1:1", Username: "u", PassHash: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Attach(c); err != nil {
		t.Fatal(err)
	}
	if got := s.Catalog().Post; got == nil {
		t.Fatal("attached catalog lost owner post callback")
	}
	s.Detach()
	if got := s.Catalog().Post; got == nil {
		t.Fatal("detached catalog lost owner post callback")
	}
}

// Cancel invalidates asynchronous work without dropping the connected client.
func TestStateCancelKeepsClient(t *testing.T) {
	s := NewState()
	c, err := backend.New(backend.Config{Address: "127.0.0.1:1", Username: "u", PassHash: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Attach(c); err != nil {
		t.Fatal(err)
	}
	_, gen, err := s.activeClient(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s.Cancel()
	if !s.Connected() || s.clientCurrent(c, gen) {
		t.Fatal("cancel did not invalidate the old generation")
	}
}

// Generation changes when a session is canceled, so late platform UI work can be ignored.
func TestStateGenerationChangesOnCancel(t *testing.T) {
	s := NewState()
	before := s.Generation()
	s.Cancel()
	if after := s.Generation(); after == before {
		t.Fatalf("generation stayed %d after cancel", before)
	}
}

// A favorite request canceled on switch rolls back and remains retryable on the same account.
func TestFavoriteRollsBackOnPlatformSwitch(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	activation, leave := context.WithCancel(ctx)
	started := make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/music/api/v1/user/password-login":
			_, _ = io.WriteString(w, `{"code":0,"data":{"userToken":"session"}}`)
		case "/music/api/v1/favorite-track/create":
			if requests.Add(1) == 1 {
				close(started)
				<-activation.Done()
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = io.WriteString(w, `{"code":0}`)
		default:
			_, _ = io.WriteString(w, `{"code":0,"data":{}}`)
		}
	}))
	defer server.Close()
	defer leave()
	s := NewState()
	gen := s.beginConnect()
	err := s.connect(ctx, backend.Config{Address: server.URL, Username: "listener", PassHash: "fixture"}, gen)
	s.setConnecting(gen, false)
	if err != nil {
		t.Fatal(err)
	}
	track := pluginapi.Track{ID: 7, Ref: "fixture-track"}
	finished := make(chan error, 1)
	go func() { finished <- s.SetFavorite(activation, &track, true) }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("favorite request did not reach the service")
	}
	if !s.Liked(track.ID) {
		t.Fatal("the pending request did not show the optimistic favorite")
	}
	s.Cancel()
	leave()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("leaving the platform returned %v, want cancellation", err)
		}
	case <-ctx.Done():
		t.Fatal("favorite request remained in flight after switching")
	}
	if s.Liked(track.ID) || s.LikeBusy(track.ID) {
		t.Fatal("the canceled favorite remained selected or pending")
	}
	if err := s.SetFavorite(ctx, &track, true); err != nil {
		t.Fatal(err)
	}
	if !s.Liked(track.ID) || requests.Load() != 2 {
		t.Fatal("the same account could not retry the favorite after switching back")
	}
}

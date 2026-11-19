package dlna

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/supersonic-app/go-upnpcast/services/avtransport"
)

// A renderer returning HH:MM:SS drives progress, and pausing on it freezes that progress.
func TestRendererPositionAndRemotePause(t *testing.T) {
	var state, rel = "PLAYING", "00:00:00"
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch action := r.Header.Get("SOAPAction"); {
		case strings.Contains(action, "#GetTransportInfo"):
			fmt.Fprintf(w, "<Envelope><Body><GetTransportInfoResponse><CurrentTransportState>%s</CurrentTransportState></GetTransportInfoResponse></Body></Envelope>", state)
		case strings.Contains(action, "#GetPositionInfo"):
			fmt.Fprintf(w, "<Envelope><Body><GetPositionInfoResponse><RelTime>%s</RelTime><TrackDuration>00:00:14</TrackDuration></GetPositionInfoResponse></Body></Envelope>", rel)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	c, err := NewController(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	d := &Device{ID: "renderer", URL: srv.URL, avCtrl: srv.URL, av: avtransport.NewClient(srv.URL, "")}
	c.AddDevice(d)
	if err := c.SelectDevice(d.ID); err != nil {
		t.Fatal(err)
	}
	if err := c.PlayTrack(t.Context(), TrackMeta{ID: "sample", Format: "mp3", LocalFile: "../testdata/seek.mp3"}, 0); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	s := c.session
	c.cancelPoll()
	c.cancelPoll = nil
	c.mu.Unlock()
	sample := func(next, pos string) CastStatus {
		mu.Lock()
		state, rel = next, pos
		mu.Unlock()
		c.remoteMu.Lock()
		c.sample(t.Context(), s)
		c.remoteMu.Unlock()
		return c.CastStatus()
	}
	if got := sample("PLAYING", "00:00:00"); !got.Loading || got.Playing || got.Position != 0 {
		t.Fatalf("a stalled renderer advanced the player: %+v", got)
	}
	if got := sample("PLAYING", "00:00:02"); !got.Playing || got.Loading || got.Position != 2*time.Second || got.Duration != 14*time.Second {
		t.Fatalf("app did not adopt the reported renderer clock: %+v", got)
	}
	if got := sample("PAUSED_PLAYBACK", "00:00:03"); got.Playing || got.Want || got.Position != 3*time.Second {
		t.Fatalf("renderer pause was not reflected: %+v", got)
	}
	if got := sample("STOPPED", "00:00:00"); got.Ended || got.Position != 3*time.Second {
		t.Fatalf("a manual stop was treated as EOF: %+v", got)
	}
}

// A position reply from a revoked track cannot affect a newer cast.
func TestStaleRendererPosition(t *testing.T) {
	arrived, release := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.Header.Get("SOAPAction")
		switch {
		case strings.Contains(action, "#GetTransportInfo"):
			io.WriteString(w, "<Envelope><Body><GetTransportInfoResponse><CurrentTransportState>PLAYING</CurrentTransportState></GetTransportInfoResponse></Body></Envelope>")
		case strings.Contains(action, "#GetPositionInfo"):
			close(arrived)
			select {
			case <-release:
			case <-ctx.Done():
			}
			io.WriteString(w, "<Envelope><Body><GetPositionInfoResponse><RelTime>00:00:11</RelTime></GetPositionInfoResponse></Body></Envelope>")
		}
	}))
	defer srv.Close()
	c, err := NewController(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	d := &Device{ID: "renderer", URL: srv.URL, avCtrl: srv.URL, av: avtransport.NewClient(srv.URL, "")}
	c.AddDevice(d)
	if err := c.SelectDevice(d.ID); err != nil {
		t.Fatal(err)
	}
	meta := TrackMeta{ID: "old", Format: "mp3", LocalFile: "../testdata/seek.mp3"}
	if err := c.PrepareTrack(ctx, meta, 0); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	s := c.session
	s.loaded = true
	c.mu.Unlock()
	done := make(chan struct{})
	go func() {
		c.sample(ctx, s)
		close(done)
	}()
	select {
	case <-arrived:
	case <-ctx.Done():
		t.Fatal("renderer did not receive the progress query")
	}
	meta.ID = "new"
	if err := c.PrepareTrack(ctx, meta, 2*time.Second); err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("position query did not return")
	}
	if got := c.CastStatus(); got.ID != "new" || got.Position != 2*time.Second || got.Playing {
		t.Fatalf("stale renderer response changed the current track: %+v", got)
	}
}

// An acknowledged seek that the renderer ignores while paused is applied after it resumes.
func TestPausedSeekIsAppliedOnResume(t *testing.T) {
	var mu sync.Mutex
	state, position := "PAUSED_PLAYBACK", "00:00:03"
	seeks := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch a := r.Header.Get("SOAPAction"); {
		case strings.Contains(a, "#GetTransportInfo"):
			fmt.Fprintf(w, "<Envelope><Body><GetTransportInfoResponse><CurrentTransportState>%s</CurrentTransportState></GetTransportInfoResponse></Body></Envelope>", state)
		case strings.Contains(a, "#GetPositionInfo"):
			fmt.Fprintf(w, "<Envelope><Body><GetPositionInfoResponse><RelTime>%s</RelTime><TrackDuration>00:00:14</TrackDuration></GetPositionInfoResponse></Body></Envelope>", position)
		case strings.Contains(a, "#Seek"):
			data, _ := io.ReadAll(r.Body)
			if strings.Contains(string(data), "00:00:08") && state == "PLAYING" {
				position = "00:00:08"
			}
			seeks <- state
		case strings.Contains(a, "#Play"):
			state = "PLAYING"
		}
	}))
	defer srv.Close()
	c, err := NewController(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	d := &Device{ID: "renderer", URL: srv.URL, avCtrl: srv.URL, av: avtransport.NewClient(srv.URL, "")}
	c.AddDevice(d)
	if err := c.SelectDevice(d.ID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()
	if err := c.PrepareTrack(ctx, TrackMeta{ID: "sample", Format: "mp3", LocalFile: "../testdata/seek.mp3"}, 3*time.Second); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	s := c.session
	s.sent, s.loaded, s.confirmed = true, true, true
	c.mu.Unlock()
	if err := c.Seek(ctx, 8*time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case call := <-seeks:
		if call != "PAUSED_PLAYBACK" {
			t.Fatalf("seek ran while renderer was %s", call)
		}
	case <-ctx.Done():
		t.Fatal("paused seek was not sent")
	}
	if got := c.CastStatus(); got.Position != 3*time.Second || got.Playing {
		t.Fatalf("app reported an unconfirmed position: %+v", got)
	}
	playing, rev := c.playIntent(ctx)
	if err := c.start(playing, rev); err != nil {
		t.Fatal(err)
	}
	c.remoteMu.Lock()
	c.sample(ctx, s)
	c.sample(ctx, s)
	c.remoteMu.Unlock()
	select {
	case call := <-seeks:
		if call != "PLAYING" {
			t.Fatalf("deferred seek ran while renderer was %s", call)
		}
	case <-ctx.Done():
		t.Fatal("resume did not seek after playback began")
	}
	mu.Lock()
	position = "00:00:09"
	mu.Unlock()
	c.remoteMu.Lock()
	c.sample(ctx, s)
	c.remoteMu.Unlock()
	if got := c.CastStatus(); !got.Playing || got.Position != 9*time.Second {
		t.Fatalf("resume did not adopt the confirmed seek position: %+v", got)
	}
}

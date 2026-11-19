package dlna

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/supersonic-app/go-upnpcast/services/avtransport"
)

// Stopping a cast revokes its current and staged media so the next cast can use the same renderer cleanly.
func TestStopRevokesCastTracks(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	stopped := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.Header.Get("SOAPAction"), `#Stop"`) {
			stopped <- struct{}{}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	controller, err := NewController(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer controller.Close()
	device := &Device{ID: "fixture", URL: server.URL, av: avtransport.NewClient(server.URL, ""), avCtrl: server.URL}
	controller.AddDevice(device)
	if err := controller.SelectDevice(device.ID); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "audio.mp3")
	if err := os.WriteFile(file, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	current := TrackMeta{ID: "owner-one-17", LocalFile: file, Format: "mp3", CoverData: []byte{0xff, 0xd8, 0xff, 1}}
	next := current
	next.ID = "owner-two-17"
	if err := controller.PlayTrack(ctx, current, 0); err != nil {
		t.Fatal(err)
	}
	if err := controller.SetNextTrack(ctx, next); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	for _, id := range []string{current.ID, next.ID} {
		resp, err := client.Get(controller.Proxy().LocalURL(id))
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || string(body) != "fixture" {
			t.Fatalf("cast URL for %s did not serve the cached file: %q, %v", id, body, err)
		}
	}
	controller.Stop()
	select {
	case <-stopped:
	case <-ctx.Done():
		t.Fatal("renderer did not receive Stop")
	}
	if playing, pos, dur := controller.Status(); playing || pos != 0 || dur != 0 || controller.ActiveDevice() != device {
		t.Fatalf("cast state after stopping = playing %t, pos %v, duration %v, device %v", playing, pos, dur, controller.ActiveDevice())
	}
	for _, id := range []string{current.ID, next.ID} {
		streamURL := controller.Proxy().LocalURL(id)
		coverURL := strings.Replace(streamURL, "/stream/", "/cover/", 1) + ".jpg"
		for _, address := range []string{streamURL, coverURL} {
			resp, err := client.Get(address)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("revoked URL %s returned %d", address, resp.StatusCode)
			}
		}
	}
	if err := controller.SetNextTrack(ctx, next); err == nil {
		t.Fatal("a late prefetch was accepted after the cast stopped")
	}
	current.ID = "new-owner-17"
	if err := controller.PlayTrack(ctx, current, 0); err != nil {
		t.Fatal(err)
	}
}

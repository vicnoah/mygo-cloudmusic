package player

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"waygu-music/internal/player/dlna"
	"waygu-music/internal/pluginapi"
)

// Switching from paused local playback does not resolve the source or set a URI on the renderer.
func TestSwitchOutputPreservesPausedPosition(t *testing.T) {
	actions := make(chan string, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actions <- r.Header.Get("SOAPAction")
	}))
	defer srv.Close()
	p := New(func(func()) {})
	defer p.DLNA().Close()
	p.ResolveStream = func(context.Context, *pluginapi.Track, string) (*pluginapi.Stream, error) {
		actions <- "resolve"
		return nil, errors.New("local fixture does not load audio")
	}
	p.Cue([]pluginapi.Track{{ID: 17, Name: "paused", Duration: 30000}})
	p.Seek(8 * time.Second)
	dev := &dlna.Device{ID: "renderer", URL: srv.URL}
	p.DLNA().AddDevice(dev)
	p.SwitchOutput(dev.ID)
	p.Seek(9*time.Second + 100*time.Millisecond)
	if !p.IsCasting() || p.Playing() || p.Loading() || p.Position() != 9100*time.Millisecond ||
		p.Current().ID != 17 || len(p.Queue()) != 1 {
		t.Fatalf("paused handoff changed playback: playing %t, position %v, track %v", p.Playing(), p.Position(), p.Current())
	}
	p.SwitchOutput("")
	if p.IsCasting() || p.Playing() || p.Position() != 9100*time.Millisecond {
		t.Fatal("switching back did not preserve the paused position")
	}
	select {
	case action := <-actions:
		if action != "resolve" { // The local output may resolve the source on return.
			t.Fatalf("paused handoff touched the renderer: %s", action)
		}
	default:
	}
}

// The selected quality wins if compatible; otherwise the platform supplies MP3 and byte seeking works.
func TestCastUsesPlatformCompatibleSource(t *testing.T) {
	audio, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	for _, flacOK := range []bool{true, false} {
		t.Run(fmt.Sprintf("flacSupported=%t", flacOK), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/":
					io.WriteString(w, "<root><device><serviceList><service><serviceType>urn:schemas-upnp-org:service:ConnectionManager:1</serviceType><controlURL>/cm</controlURL></service></serviceList></device></root>")
				case "/cm":
					sink := "http-get:*:audio/mpeg:DLNA.ORG_PN=MP3"
					if flacOK {
						sink += ",http-get:*:audio/flac:*"
					}
					fmt.Fprintf(w, "<Envelope><Body><GetProtocolInfoResponse><Sink>%s</Sink></GetProtocolInfoResponse></Body></Envelope>", sink)
				case "/mp3":
					// An older platform can serve converted audio without Range. Host staging makes it seekable.
					w.Header().Set("Content-Type", "audio/mpeg")
					w.Write(audio)
				}
			}))
			defer srv.Close()
			p := New(func(func()) {})
			defer p.DLNA().Close()
			p.ResolveStream = func(_ context.Context, _ *pluginapi.Track, level string) (*pluginapi.Stream, error) {
				if level != "lossless" {
					t.Fatalf("resolved with changed quality %q", level)
				}
				return &pluginapi.Stream{Type: "flac", URL: srv.URL + "/flac"}, nil
			}
			var target string
			p.ResolveCastSource = func(_ context.Context, _ *pluginapi.Track, format string) (*pluginapi.Stream, pluginapi.SegmentedSource, error) {
				if format == "" {
					return nil, nil, nil
				}
				target = format
				return &pluginapi.Stream{Type: "mp3", URL: srv.URL + "/mp3"}, nil, nil
			}
			device := &dlna.Device{ID: "renderer", URL: srv.URL}
			media, retry, err := p.prepareCast(t.Context(), pluginapi.Track{}, "lossless", device, false, nil)
			if err != nil || retry {
				t.Fatalf("could not prepare cast: %v (retry=%t)", err, retry)
			}
			if flacOK {
				if target != "" || media.info.Type != "flac" {
					t.Fatal("compatible audio was unnecessarily replaced")
				}
				return
			}
			if target != "mp3" || media.info.Type != "mp3" || media.localFile == "" {
				t.Fatalf("platform conversion was not selected and staged: %+v", media)
			}
			p.DLNA().Proxy().Register("cast", dlna.StreamSource{LocalFile: media.localFile, ContentType: "audio/mpeg"})
			req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, p.DLNA().Proxy().LocalURL("cast"), nil)
			req.Header.Set("Range", "bytes=16-31")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			got, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil || resp.StatusCode != 206 || string(got) != string(audio[16:32]) ||
				!strings.HasPrefix(resp.Header.Get("Content-Range"), "bytes 16-31/") {
				t.Fatalf("staged MP3 failed range request: %d, %v", resp.StatusCode, readErr)
			}
		})
	}
}

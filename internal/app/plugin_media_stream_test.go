package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/waygu-music-plugin/sdk/bridge"
	"github.com/waygu-music-plugin/sdk/platformrpc"
	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/pluginapi/network"
	"github.com/waygu-music-plugin/sdk/protocol"

	"waygu-music/internal/player/dlna"
)

var fixtureCover = bytes.Repeat([]byte{0x12, 0x34, 0x56}, 1<<20)
var fixtureAudio = []byte("ID3TEST0123456789")

type mediaFixture struct {
	link  *platformrpc.ChildLink
	slots bridge.MediaSlots
}

func TestMain(m *testing.M) {
	if platformrpc.IsChild() {
		platformrpc.Serve(func(link *platformrpc.ChildLink) any {
			return &mediaFixture{link: link, slots: bridge.NewMediaSlots()}
		})
		return
	}
	os.Exit(m.Run())
}

func (f *mediaFixture) Hello(req platformrpc.HelloRequest, out *platformrpc.HelloReply) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	offer := platformrpc.Offer{
		Version:  protocol.Version,
		Provides: []string{platformrpc.CapabilityWindow, platformrpc.CapabilityMedia, platformrpc.CapabilityMediaStreams, protocol.CapabilityMediaRenewal},
		Needs:    []string{platformrpc.CapabilityPlayback, platformrpc.CapabilitySettings},
	}
	if err := platformrpc.CheckCompatibility(req.Host, offer); err != nil {
		return err
	}
	if err := f.link.Bind(ctx, req, func() { os.Exit(0) }); err != nil {
		return err
	}
	*out = platformrpc.HelloReply{Identity: req.Identity, Plugin: offer}
	return nil
}

func (f *mediaFixture) Media(request protocol.PluginCall, out *protocol.PluginMedia) error {
	in := bridge.LocalMediaCall(request)
	var reply PluginMedia
	bridge.ServeMedia(context.Background(), f.slots.ForAction(in.Action), f.link.DialStream, in, &reply, func(ctx context.Context) error {
		if in.Action == "cast" && in.Format == "mp3" {
			stream := &pluginapi.Stream{URL: in.Track.Ref + "/expired", Type: in.Format, Size: int64(len(fixtureAudio))}
			stream.Header = func() http.Header { return http.Header{"Authorization": {"Bearer expired"}} }
			stream.Renew = func(_ context.Context, previous network.MediaEndpoint) (network.MediaEndpoint, error) {
				if previous.URL != stream.URL {
					return network.MediaEndpoint{}, errors.New("incorrect renewal source")
				}
				return network.MediaEndpoint{URL: in.Track.Ref + "/renewed", Header: http.Header{"Authorization": {"Bearer renewed"}}}, nil
			}
			var err error
			reply.Stream, err = bridge.DescribeMediaSource(ctx, true, in.PreviousURL, stream)
			return err
		}
		if in.Action == "stream" {
			if in.Track.ID == 17 {
				return protocol.Failure{Code: protocol.FailureMediaMessage, Message: "此音源只能试听"}
			}
			reply.Stream = &PluginSource{URL: in.Track.Ref, Type: "flac"}
			return nil
		}
		if in.Action == "started" {
			reply.Handled = true
			return nil
		}
		if in.Ref == "" {
			reply.Cover, reply.Handled = fixtureCover, true
			return nil
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.Ref, nil)
		if err != nil {
			return err
		}
		response, err := http.DefaultClient.Do(req)
		if err == nil {
			response.Body.Close()
		}
		return err
	})
	*out = reply.Wire()
	return nil
}

func startMediaFixture(t *testing.T, ctx context.Context) *pluginProcess {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := platformrpc.Start(ctx, platformrpc.Program{
		Path: exe, SHA256: sha256.Sum256(binary), StateDir: t.TempDir(),
		Identity: platformrpc.Identity{Package: "fixture", Entry: "fixture"},
	}, hostOffer, func(id string) any { return pluginReverse{connection: id} })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(transport.Close)
	return &pluginProcess{
		transport: transport, media: bridge.NewMediaSlots(),
		scope: platformrpc.Scope{Connection: transport.Connection(), Activation: 1},
	}
}

// Playback displays the plugin's message without exposing its internal failure code.
func TestPluginPlaybackMessageSurvivesTransport(t *testing.T) {
	process := startMediaFixture(t, t.Context())
	adapter := &processAdapter{id: "fixture"}
	adapter.active.Store(process)
	media := adapter.PlaybackMedia()
	track := &pluginapi.Track{ID: 17}
	stream, err := media.Stream(t.Context(), track, "standard")
	if stream != nil || err == nil || media.ErrorMessage(track, err) != "此音源只能试听" {
		t.Fatalf("expected a displayable playback failure, got %+v, %v", stream, err)
	}
}

// A cast range retries with the same format after the child refreshes its URL and header.
func TestPluginCastRenewalPreservesRange(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	var rejected, renewed atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "upstream-session=secret")
		switch {
		case r.URL.Path == "/expired" && r.Header.Get("Authorization") == "Bearer expired":
			rejected.Add(1)
			http.Error(w, "expired credential", http.StatusUnauthorized)
		case r.URL.Path == "/renewed" && r.Header.Get("Authorization") == "Bearer renewed":
			renewed.Add(1)
			w.Header().Set("Content-Type", "audio/mpeg")
			http.ServeContent(w, r, "fixture.mp3", time.Time{}, bytes.NewReader(fixtureAudio))
		default:
			http.Error(w, "missing upstream authorization", http.StatusForbidden)
		}
	}))
	defer server.Close()
	process := startMediaFixture(t, ctx)
	adapter := &processAdapter{id: "fixture"}
	adapter.active.Store(process)
	stream, segments, err := adapter.PlaybackMedia().CastSource(ctx, &pluginapi.Track{
		PlatformTrack: pluginapi.PlatformTrack{Source: "fixture", Ref: server.URL},
	}, "mp3")
	if err != nil || stream == nil || stream.Renew == nil || segments != nil {
		t.Fatalf("could not resolve a renewable compatible source: %v", err)
	}
	proxy, err := dlna.NewProxy()
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()
	proxy.Register("fixture-audio", dlna.StreamSource{
		RemoteURL: stream.URL, Header: stream.Header, Renew: stream.Renew, ContentType: "audio/mpeg",
	})
	local := proxy.LocalURL("fixture-audio")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, local, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=7-9")
	req.Header.Set("Authorization", "Bearer renderer-token")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusPartialContent || !bytes.Equal(got, fixtureAudio[7:10]) ||
		response.Header.Get("Content-Range") != "bytes 7-9/17" || response.Header.Get("Set-Cookie") != "" {
		t.Fatalf("renewed range returned %d, headers %v, %q, %v", response.StatusCode, response.Header, got, err)
	}
	head, err := http.NewRequestWithContext(ctx, http.MethodHead, local, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err = http.DefaultClient.Do(head)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || rejected.Load() != 1 || renewed.Load() != 2 {
		t.Fatalf("HEAD did not reuse the renewed source: status %d, rejected %d, renewed %d", response.StatusCode, rejected.Load(), renewed.Load())
	}
	adapter.active.Store(nil)
	if _, err := stream.Renew(ctx, network.MediaEndpoint{URL: stream.URL}); !errors.Is(err, pluginapi.ErrUnavailable) {
		t.Fatalf("a retired process could renew the source: %v", err)
	}
	proxy.Unregister("fixture-audio")
	response, err = http.DefaultClient.Do(head)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound || renewed.Load() != 2 {
		t.Fatalf("revoked proxy URL reached the provider: %d, requests %d", response.StatusCode, renewed.Load())
	}
}

// Closing a product media request stream cancels the remote provider and leaves the same plugin usable for a large cover.
func TestPluginMediaStreamCancelsRemoteProvider(t *testing.T) {
	requested, stopped := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		close(requested)
		<-req.Context().Done()
		close(stopped)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	p := startMediaFixture(t, ctx)
	requestCtx, stopRequest := context.WithCancel(ctx)
	defer stopRequest()
	result := make(chan error, 1)
	go func() {
		_, err := p.call(requestCtx, PluginCall{Action: "artwork", Ref: server.URL})
		result <- err
	}()
	select {
	case <-requested:
	case <-ctx.Done():
		t.Fatal("remote provider never received request")
	}
	stopRequest()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request returned %v", err)
	}
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("provider kept running after the caller canceled")
	}
	out, err := p.call(ctx, PluginCall{Action: "cover"})
	if err != nil || !out.Handled || out.CoverSize != uint32(len(fixtureCover)) || !bytes.Equal(out.Cover, fixtureCover) {
		t.Fatalf("large cover after cancellation: %d bytes, declared %d, error %v", len(out.Cover), out.CoverSize, err)
	}
}

// A full batch of slow artwork requests used to prevent the host from resolving a song and reporting its start.
func TestPluginPlaybackProgressesWhileArtworkWaits(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	process := startMediaFixture(t, ctx)
	limit := cap(process.media.ForAction("artwork"))
	arrivals := make(chan struct{}, limit)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		arrivals <- struct{}{}
		select {
		case <-release:
			w.WriteHeader(http.StatusNoContent)
		case <-req.Context().Done():
		}
	}))
	defer server.Close()
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()

	artwork := make(chan error, limit)
	for range limit {
		go func() {
			_, err := process.call(ctx, PluginCall{Action: "artwork", Ref: server.URL})
			artwork <- err
		}()
	}
	for range limit {
		select {
		case <-arrivals:
		case <-ctx.Done():
			t.Fatal("artwork did not fill the active request slots")
		}
	}

	track := pluginapi.Track{PlatformTrack: pluginapi.PlatformTrack{Source: "fixture", Ref: server.URL + "/flac"}}
	checkPlaybackProgress(t, ctx, process, track)
	unblock()
	for range limit {
		if err := <-artwork; err != nil {
			t.Fatalf("an artwork request did not finish: %v", err)
		}
	}
}

// A child writing several covers into stalled readers can still resolve and report playback before the readers resume.
func TestPluginPlaybackProgressesWhileCoverReadersWait(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	process := startMediaFixture(t, ctx)
	readers := make([]net.Conn, 0, cap(process.media.ForAction("cover")))
	for range cap(readers) {
		streamID, accept := process.transport.ReserveStream()
		accepted := make(chan net.Conn, 1)
		go func() {
			conn, _ := accept()
			accepted <- conn
		}()
		in := PluginCall{
			Action: "cover", Scope: process.scope, MediaStream: streamID, DeadlineNano: time.Now().Add(10 * time.Second).UnixNano(),
		}
		var reply protocol.PluginMedia
		err := process.transport.Call(ctx, "Media", in.Wire(), &reply)
		if err != nil || reply.Failure.Err() != nil || int(reply.CoverSize) != len(fixtureCover) {
			t.Fatalf("large cover did not start streaming: %v, %+v", err, reply.Failure)
		}
		var conn net.Conn
		select {
		case conn = <-accepted:
		case <-ctx.Done():
			t.Fatal("cover stream did not connect")
		}
		if conn == nil {
			t.Fatal("cover stream accept failed")
		}
		t.Cleanup(func() { _ = conn.Close() })
		deadline, _ := ctx.Deadline()
		if err := conn.SetReadDeadline(deadline); err != nil {
			t.Fatal(err)
		}
		var first [1]byte
		if _, err := io.ReadFull(conn, first[:]); err != nil || first[0] != fixtureCover[0] {
			t.Fatalf("cover writer did not begin sending the payload: %v", err)
		}
		readers = append(readers, conn)
	}

	track := pluginapi.Track{PlatformTrack: pluginapi.PlatformTrack{Source: "fixture", Ref: "http://127.0.0.1/fixture.flac"}}
	checkPlaybackProgress(t, ctx, process, track)
	for _, reader := range readers {
		data := make([]byte, len(fixtureCover)-1)
		if _, err := io.ReadFull(reader, data); err != nil || !bytes.Equal(fixtureCover[1:], data) {
			t.Fatalf("cover bytes after the reader resumed: %v", err)
		}
	}
}

func checkPlaybackProgress(t *testing.T, ctx context.Context, process *pluginProcess, track pluginapi.Track) {
	t.Helper()
	playback, stop := context.WithTimeout(ctx, 2*time.Second)
	defer stop()
	adapter := &processAdapter{id: "fixture"}
	adapter.active.Store(process)
	stream, err := adapter.PlaybackMedia().Stream(playback, &track, "lossless")
	if err != nil || stream == nil || stream.URL != track.Ref {
		t.Fatalf("could not resolve playback while artwork was pending: %v", err)
	}
	started, err := process.call(playback, PluginCall{Action: "started", Track: track})
	if err != nil || !started.Handled {
		t.Fatalf("could not report playback while artwork was pending: %v", err)
	}
}

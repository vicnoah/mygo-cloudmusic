package player

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player/mpv"
)

// Tests switching between built-in, MPV, and DLNA playback routing.
func TestEngineSwitching(t *testing.T) {
	ch := make(chan func(), 16)
	p := New(func(fn func()) { ch <- fn })

	// Initially MPV is not enabled
	if p.UsingMPV() {
		t.Error("expected UsingMPV() to be false by default")
	}

	p.SetUseMPV(true)
	if !p.MPVReady() && p.UsingMPV() {
		t.Error("UsingMPV() should only be true when MPV is actually ready")
	}

	// Test DLNA routing
	if p.IsCasting() {
		t.Error("expected IsCasting() to be false initially")
	}
	dlnaCtrl := p.DLNA()
	if dlnaCtrl == nil {
		t.Fatal("expected non-nil DLNA controller")
	}

	// Cue a track
	p.Cue([]netease.Track{{ID: 101, Name: "Test Track", Duration: 180000}})

	// When not casting, duration comes from track
	if p.Duration() != 180*time.Second {
		t.Errorf("got duration %v, want 180s", p.Duration())
	}

	// Select a device in DLNA
	_ = dlnaCtrl.SelectDevice("")
	if p.IsCasting() {
		t.Error("expected IsCasting() to be false after empty device")
	}

	// Test volume delegation
	p.SetVolume(0.7)
	if p.volume != 0.7 {
		t.Errorf("got volume %v, want 0.7", p.volume)
	}

	// TogglePlay with empty queue or initial track
	p.TogglePlay()
	if !p.playing {
		t.Error("expected player to be playing after TogglePlay")
	}

	p.TogglePlay()
	if p.playing {
		t.Error("expected playing to be false after second TogglePlay")
	}
	_ = context.Background()
}

// Tests switching output targets via SwitchOutput between local speaker and DLNA.
func TestSwitchOutput(t *testing.T) {
	ch := make(chan func(), 16)
	p := New(func(fn func()) { ch <- fn })

	// Cue a track
	p.Cue([]netease.Track{{ID: 202, Name: "Switch Track", Duration: 200000}})

	// Switch output to "" (local)
	p.SwitchOutput("")
	if p.IsCasting() {
		t.Error("expected IsCasting() to be false for local output")
	}

	// Switch output to an unknown device ID: not discovered, so remains local
	p.SwitchOutput("non-existent-device")
	if p.IsCasting() {
		t.Error("expected IsCasting() to be false for unresolvable device ID")
	}
}

// Tests that MPV plays streams requiring custom headers via the local streaming proxy.
func TestPlayerMPVWithProxyHeaders(t *testing.T) {
	if !mpv.Ready() {
		t.Skip("mpv not available")
	}

	headerReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authx") == "test-auth-token" {
			headerReceived = true
		}
		// Return minimal dummy bytes
		w.Header().Set("Content-Type", "text/html; charset=utf-8") // simulate Feiniu's bad content-type
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("mock audio data"))
	}))
	defer server.Close()

	ch := make(chan func(), 32)
	p := New(func(fn func()) { ch <- fn })
	p.SetUseMPV(true)

	p.ResolveStream = func(tr *netease.Track, level string) (*netease.Stream, error) {
		return &netease.Stream{
			URL:  server.URL,
			Type: "mp3",
			Header: func() http.Header {
				h := http.Header{}
				h.Set("Authx", "test-auth-token")
				return h
			},
		}, nil
	}

	p.PlayQueue([]netease.Track{{ID: 303, Name: "Proxy Track", Duration: 3000}}, 0)

	// Pump event loop
	timeout := time.After(2 * time.Second)
	for p.Loading() {
		select {
		case fn := <-ch:
			fn()
		case <-timeout:
			t.Fatal("timed out waiting for player to load")
		}
	}

	start := time.Now()
	for !headerReceived && time.Since(start) < 2*time.Second {
		select {
		case fn := <-ch:
			fn()
		case <-time.After(50 * time.Millisecond):
		}
	}

	if !headerReceived {
		t.Error("expected custom Authx header to be received by the server via proxy")
	}
}

// TestMPVProgressAndReady verifies that when playing via MPV, p.Ready() reports true
// and p.Duration() accurately reports the track duration instead of fluctuating buffer estimates.
func TestMPVProgressAndReady(t *testing.T) {
	if !mpv.Ready() {
		t.Skip("mpv not available")
	}

	ch := make(chan func(), 32)
	p := New(func(fn func()) { ch <- fn })
	p.SetUseMPV(true)
	p.SetMPVConfig(mpv.Config{NullAO: true})

	p.ResolveStream = func(tr *netease.Track, level string) (*netease.Stream, error) {
		return &netease.Stream{
			URL:  "av://lavfi:sine=frequency=1000:duration=60",
			Type: "mp3",
		}, nil
	}

	p.PlayQueue([]netease.Track{{ID: 404, Name: "Test Progress Track", Duration: 60000}}, 0)

	timeout := time.After(3 * time.Second)
	for p.Loading() {
		select {
		case fn := <-ch:
			fn()
		case <-timeout:
			t.Fatal("timed out waiting for player to load")
		}
	}

	if !p.Ready() {
		t.Error("expected p.Ready() to be true when MPV is playing")
	}

	if p.Duration() != 60*time.Second {
		t.Errorf("got duration %v, want 60s", p.Duration())
	}

	// Wait for a few positions to advance
	time.Sleep(300 * time.Millisecond)
	for i := 0; i < 10; i++ {
		select {
		case fn := <-ch:
			fn()
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Logf("p.playing=%v, mpv.Playing=%v, pause=%s, time-pos=%s",
		p.playing, p.mpvOut.Playing(), p.mpvOut.GetProperty("pause"), p.mpvOut.GetProperty("time-pos"))

	pos := p.Position()
	if pos <= 0 {
		t.Errorf("expected position > 0, got %v", pos)
	}

	// Test Seeking in MPV
	p.Seek(15 * time.Second)
	time.Sleep(200 * time.Millisecond)
	seekPos := p.Position()
	if seekPos < 14*time.Second || seekPos > 18*time.Second {
		t.Errorf("expected seek position near 15s, got %v", seekPos)
	}

	// Test Decoupled Volume Mapping in MPV
	p.SetVolume(0.8)
	if v := p.mpvOut.Volume(); v != 0.8 {
		t.Errorf("expected MPV volume 0.8, got %v", v)
	}
	p.SetVolume(0.5)
	if v := p.mpvOut.Volume(); v != 0.5 {
		t.Errorf("expected MPV volume 0.5, got %v", v)
	}
	p.SetVolume(1.0)
	if v := p.mpvOut.Volume(); v != 1.0 {
		t.Errorf("expected MPV volume 1.0, got %v", v)
	}
}

type testFadeOutput struct {
	vol   float64
	level float64
}

func (t *testFadeOutput) setVolume(v float64) {
	t.vol = v
}

func (t *testFadeOutput) setFadeLevel(l float64) {
	t.level = l
}

// Tests that fader routes through fadeLevelOutput when supported.
func TestFaderFadeLevelOutput(t *testing.T) {
	f := &fader{}
	out := &testFadeOutput{}
	f.attach(out, 1.0)
	f.setGain(0.64)

	if out.level != 1.0 {
		t.Errorf("got level %v, want 1.0", out.level)
	}
	// Normal volumeOutput would have set out.vol = 0.64 * 1.0 = 0.64,
	// but fadeLevelOutput must receive the unscaled fade level 1.0.
	if out.vol != 0 {
		t.Errorf("expected setVolume not to be called, got %v", out.vol)
	}
}

package mpv

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/waygu-music-plugin/sdk/pluginapi/eq"
)

// A nonzero start used to be sent as a seek before the newly opened file could accept it.
func TestMPVLoadsAtRequestedPosition(t *testing.T) {
	if !Ready() {
		t.Skip("mpv not available")
	}
	engine, err := NewEngine(Config{NullAO: true})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	restarted := make(chan struct{}, 1)
	engine.OnPlaybackRestart(func() {
		select {
		case restarted <- struct{}{}:
		default:
		}
	})
	path, err := filepath.Abs("../testdata/seek.flac")
	if err != nil {
		t.Fatal(err)
	}
	const start = 2 * time.Second
	if err := engine.Play(path, nil, start); err != nil {
		t.Fatal(err)
	}
	engine.Pause()
	select {
	case <-restarted:
	case <-time.After(3 * time.Second):
		t.Fatal("mpv did not finish opening the paused track")
	}
	if pos := engine.Position(); (pos - start).Abs() > 250*time.Millisecond {
		t.Fatalf("mpv started at %s, want %s", pos, start)
	}
}

// Tests MPV engine initialization and basic control commands if libmpv is available.
func TestMPVEngine(t *testing.T) {
	if !Ready() {
		t.Skip("skipping TestMPVEngine because libmpv is not installed/downloaded on this host")
	}

	engine, err := NewEngine(Config{})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	// Ensure headless-friendly options in test
	engine.inst.SetOption("ao", "null")

	// Create a dummy WAV file
	tmpDir := t.TempDir()
	wavFile := filepath.Join(tmpDir, "silence.wav")
	// Minimal valid 44-byte WAV header (1 second silence)
	wavHeader := []byte{
		'R', 'I', 'F', 'F',
		36, 0, 0, 0, // file size - 8
		'W', 'A', 'V', 'E',
		'f', 'm', 't', ' ',
		16, 0, 0, 0, // subchunk1 size
		1, 0, // PCM
		1, 0, // mono
		0x44, 0xac, 0, 0, // 44100
		0x44, 0xac, 0, 0, // byte rate
		1, 0, // block align
		8, 0, // 8-bit
		'd', 'a', 't', 'a',
		0, 0, 0, 0,
	}
	if err := os.WriteFile(wavFile, wavHeader, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := engine.Play(wavFile, nil, 0); err != nil {
		t.Fatalf("engine.Play: %v", err)
	}

	// Verify volume
	engine.SetVolume(0.5)
	if engine.volume != 0.5 {
		t.Errorf("got volume %v, want 0.5", engine.volume)
	}

	// Verify pause / resume
	engine.Pause()
	if engine.Playing() {
		t.Error("expected Playing() to be false after Pause()")
	}

	engine.Resume()
	if !engine.Playing() {
		t.Error("expected Playing() to be true after Resume()")
	}

	engine.Seek(1 * time.Second)

	devs := engine.AudioDevices()
	if len(devs) == 0 {
		t.Fatal("expected audio devices from engine")
	}
	engine.SetAudioDevice("auto")
	if dev := engine.inst.GetProperty("audio-device"); dev != "auto" {
		t.Errorf("got audio-device %q, want auto", dev)
	}

	// Test package-level AudioDevices probe
	allDevs := AudioDevices()
	if len(allDevs) == 0 {
		t.Fatal("expected package-level AudioDevices to return devices")
	}

	// Verify the demuxer cache is configured.
	if maxBytes := engine.inst.GetProperty("demuxer-max-bytes"); maxBytes == "" || maxBytes == "0" {
		t.Errorf("expected demuxer-max-bytes to be non-empty, got %q", maxBytes)
	}

	// Test playlist append for gapless prefetching
	if err := engine.Append(wavFile); err != nil {
		t.Errorf("engine.Append: %v", err)
	}

	// Test Stop and Closed
	engine.Stop()
	if engine.Playing() {
		t.Error("expected playing to be false after Stop()")
	}
	if engine.Closed() {
		t.Error("expected engine not to be closed after Stop()")
	}
}

// Tests constructing audio filter strings for graphic equalizer curves.
func TestBuildEqualizerFilter(t *testing.T) {
	// 1. Flat bypass should produce empty filter string.
	flatFilter := eq.BuildEqualizerFilter(0, []float64{}, eq.TypeISO10)
	if flatFilter != "" {
		t.Errorf("expected empty filter for flat curve, got %q", flatFilter)
	}

	// 2. Preamp only.
	preampFilter := eq.BuildEqualizerFilter(-3.0, []float64{}, eq.TypeISO10)
	if preampFilter != "volume=volume=-3.0dB" {
		t.Errorf("expected volume filter, got %q", preampFilter)
	}

	// 3. Bands with gain.
	gains := []float64{3.0, 0, 0, 0, 0, 0, 0, 0, 0, -2.5}
	filter := eq.BuildEqualizerFilter(1.5, gains, eq.TypeISO10)
	want := "volume=volume=1.5dB,equalizer=f=31:g=3.00:t=o:w=1.00,equalizer=f=16000:g=-2.50:t=o:w=1.00"
	if filter != want {
		t.Errorf("got %q, want %q", filter, want)
	}

	// 4. The 15-band curve uses thirds of an octave.
	bands := make([]float64, eq.BandCount(eq.TypeISO15))
	bands[0] = -1
	got := eq.BuildEqualizerFilter(0, bands, eq.TypeISO15)
	if want := "equalizer=f=25:g=-1.00:t=o:w=0.67"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

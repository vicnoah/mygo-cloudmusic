package mpv

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Tests metadata and path resolution for libmpv assets across platforms.
func TestMPVMeta(t *testing.T) {
	if Platform() == "" {
		t.Fatal("expected non-empty platform")
	}

	names := SystemLibNames()
	if len(names) == 0 {
		t.Fatal("expected system lib names")
	}

	dir, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	if dir == "" {
		t.Fatal("expected non-empty CacheDir")
	}

	target, err := TargetFile()
	if err != nil {
		t.Fatalf("TargetFile: %v", err)
	}
	if target == "" {
		t.Fatal("expected non-empty TargetFile")
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
}

// Tests downloading the real libmpv asset from the GitHub release.
func TestDownloadFromRelease(t *testing.T) {
	tmp := t.TempDir()
	SetCacheDir(tmp)
	defer SetCacheDir("")

	var lastDone, lastTotal int64
	err := Download(t.Context(), func(done, total int64) {
		lastDone = done
		lastTotal = total
	})
	if err != nil {
		t.Fatalf("Download from release failed: %v", err)
	}

	if lastDone == 0 || lastDone != lastTotal {
		t.Errorf("expected download to complete, got done=%d total=%d", lastDone, lastTotal)
	}

	if !IsDownloaded() {
		t.Error("expected IsDownloaded to report true after successful download")
	}

	target, _ := TargetFile()
	fi, err := os.Stat(target)
	if err != nil || fi.Size() == 0 {
		t.Fatalf("downloaded file stat error: %v", err)
	}
}

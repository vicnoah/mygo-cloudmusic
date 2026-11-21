package mpv

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// A child validates the installed component with the same native API used by the application.
func TestMPVComponentProbeChild(t *testing.T) {
	if path := os.Getenv("WAGYU_MPV_PROBE_CHILD"); path != "" {
		if err := ProbePath(path, os.Stderr); err != nil {
			t.Fatal(err)
		}
	}
}

// An official component downloads, decodes audio, deactivates while mapped and cleans up on restart.
func TestLiveMPVComponent(t *testing.T) {
	if os.Getenv("LIVE_MPV") != "1" {
		t.Skip("set LIVE_MPV=1 to download and exercise the official MPV component")
	}
	cacheDirOverride = os.Getenv("WAGYU_MPV_LIVE_CHILD")
	if cacheDirOverride == "" {
		root := t.TempDir()
		ctx, cancel := context.WithTimeout(t.Context(), 6*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.v", "-test.run=^TestLiveMPVComponent$")
		configureProbe(cmd)
		cmd.Env = append(os.Environ(), "WAGYU_MPV_LIVE_CHILD="+root)
		output, err := cmd.CombinedOutput()
		t.Logf("%s", output)
		if err != nil {
			t.Fatal(err)
		}
		old := cacheDirOverride
		cacheDirOverride = root
		defer func() { cacheDirOverride = old }()
		if err := os.WriteFile(filepath.Join(root, ".download-abandoned"), []byte("partial ZIP"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := Restore(t.Context()); err != nil || IsDownloaded() || Ready() {
			t.Fatalf("deleted component restored: %v", err)
		}
		if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
			t.Fatalf("restart left decoder files behind: %v, %v", entries, err)
		}
		t.Log("reclaimed the mapped DLL or shared library on restart")
		return
	}
	if dir := os.Getenv("WAGYU_MPV_RELEASE_FIXTURE_DIR"); dir != "" {
		server := httptest.NewServer(http.FileServer(http.Dir(dir)))
		defer server.Close()
		baseDownloadURL = server.URL + "/"
	}
	componentProbe = func(ctx context.Context, path string) error {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMPVComponentProbeChild$")
		configureProbe(cmd)
		cmd.Env = append(os.Environ(), "WAGYU_MPV_PROBE_CHILD="+path)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: %s", err, output)
		}
		return nil
	}
	if err := Load(); err == nil || Ready() {
		t.Fatalf("empty cache used a system MPV library: %v", err)
	}
	var downloaded, total int64
	if err := Download(t.Context(), func(done, size int64) { downloaded, total = done, size }); err != nil {
		t.Fatal(err)
	}
	if !Ready() || downloaded == 0 || downloaded != total {
		t.Fatalf("component not ready after receiving %d/%d bytes", downloaded, total)
	}
	t.Logf("installed and probed %s (%d bytes)", Platform(), downloaded)

	inst, err := NewInstance(map[string]string{"ao": "null"})
	if err != nil {
		t.Fatal(err)
	}
	defer inst.Close()
	const sampleRate = 16000
	samples := make([]int16, sampleRate/2)
	for i := range samples {
		samples[i] = int16((i%20 - 10) * 400)
	}
	var wav bytes.Buffer
	wav.WriteString("RIFF")
	binary.Write(&wav, binary.LittleEndian, uint32(36+len(samples)*2))
	wav.WriteString("WAVEfmt ")
	binary.Write(&wav, binary.LittleEndian, uint32(16))
	binary.Write(&wav, binary.LittleEndian, [2]uint16{1, 1})
	binary.Write(&wav, binary.LittleEndian, [2]uint32{sampleRate, sampleRate * 2})
	binary.Write(&wav, binary.LittleEndian, [2]uint16{2, 16})
	wav.WriteString("data")
	binary.Write(&wav, binary.LittleEndian, uint32(len(samples)*2))
	binary.Write(&wav, binary.LittleEndian, samples)
	path := filepath.Join(t.TempDir(), "test.wav")
	if err := os.WriteFile(path, wav.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := inst.Command([]string{"loadfile", path, "replace"}); code < 0 {
		t.Fatalf("loadfile returned %d", code)
	}
	var played bool
	for attempts := 0; attempts < 32; attempts++ {
		event, code, reason := inst.WaitEvent(0.25)
		if event == EventPlaybackRestart {
			played = true
		}
		if event == EventEndFile {
			if !played || reason != EndFileReasonEOF {
				t.Fatalf("playback did not reach EOF: played=%t, code=%d, reason=%d", played, code, reason)
			}
			break
		}
		if attempts == 31 {
			t.Fatal("timed out waiting for playback to finish")
		}
	}
	t.Logf("decoded local PCM to EOF with %s", inst.GetProperty("mpv-version"))
	inst.Close()
	if err := Delete(); err != nil || Ready() || IsDownloaded() {
		t.Fatalf("delete did not deactivate the mapped library: %v", err)
	}
	if _, err := os.Stat(mapped.path); err != nil {
		t.Fatalf("delete tried to remove the mapped library: %v", err)
	}
	t.Log("deactivated while the library was mapped")
	if err := Download(t.Context(), nil); err != nil || !Ready() || NeedsRestart() {
		t.Fatalf("reinstall of the same component did not enable MPV again: %v", err)
	}
	reinstalled, err := NewInstance(map[string]string{"ao": "null"})
	if err != nil {
		t.Fatalf("opening the reinstalled decoder: %v", err)
	}
	reinstalled.Close()
	t.Log("reinstalled and enabled again in the same process")
	if err := Delete(); err != nil {
		t.Fatal(err)
	}
	// Windows cannot clean the mapped file until this process exits.
}

package mpv

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func withTestComponent(t *testing.T, index, archive []byte, key ed25519.PublicKey) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.json" {
			_, _ = w.Write(index)
		} else {
			_, _ = w.Write(archive)
		}
	}))
	t.Cleanup(server.Close)
	cache, base, public, probe := cacheDirOverride, baseDownloadURL, releaseKey, componentProbe
	previousInstalled, previousMapped := installed, mapped
	previousFailure, previousDisabled := loadFailed, loadDisabled
	cacheDirOverride, baseDownloadURL, releaseKey = t.TempDir(), server.URL+"/", key
	t.Cleanup(func() {
		cacheDirOverride, baseDownloadURL, releaseKey, componentProbe = cache, base, public, probe
		installed, mapped = previousInstalled, previousMapped
		loadFailed, loadDisabled = previousFailure, previousDisabled
	})
}

func signedFixture(t *testing.T, contents []byte) ([]byte, []byte, ed25519.PublicKey) {
	t.Helper()
	const entry = "fixture.dll"
	version := "1.0.0"
	manifest := componentManifest{
		Format: componentFormat, Version: version, MPVVersion: "0.41.0",
		Platform: Platform(), MinimumRuntime: "fixture", Library: entry,
		Files: []componentFile{{entry, int64(len(contents)), fmt.Sprintf("%x", sha256.Sum256(contents))}},
	}
	marshal := func(v any) []byte {
		t.Helper()
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	var buffer bytes.Buffer
	bundle := zip.NewWriter(&buffer)
	for name, data := range map[string][]byte{"component.json": marshal(manifest), entry: contents} {
		w, err := bundle.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := bundle.Close(); err != nil {
		t.Fatal(err)
	}
	archive := buffer.Bytes()
	asset := releaseAsset{
		Platform: Platform(), Archive: "mpv-" + version + "-" + Platform() + ".zip",
		Size: int64(len(archive)), SHA256: fmt.Sprintf("%x", sha256.Sum256(archive)),
	}
	payload := marshal(releaseIndex{componentFormat, version, []releaseAsset{asset}})
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return marshal(signedIndex{payload, ed25519.Sign(private, payload)}), archive, public
}

// Canceling a fully received component leaves the previous installation intact.
func TestDownloadCanceledBeforePublish(t *testing.T) {
	index, archive, key := signedFixture(t, []byte("fixture library"))
	withTestComponent(t, index, archive, key)
	componentProbe = func(ctx context.Context, _ string) error { return ctx.Err() }
	if err := Download(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(cacheDirOverride, "active.json")
	original, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	err = Download(ctx, func(done, total int64) {
		if done == total {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("final publication did not observe cancellation: %v", err)
	}
	current, err := os.ReadFile(record)
	if err != nil || !bytes.Equal(current, original) || !Ready() {
		t.Fatalf("canceled update changed the installed component: %v", err)
	}
	if files, err := os.ReadDir(cacheDirOverride); err != nil || len(files) != 2 {
		t.Fatalf("canceled update left staging files: %v, %v", files, err)
	}
}

// A correctly downloaded archive remains unavailable when its native dependencies cannot load.
func TestDownloadRetainsNativeProbeFailure(t *testing.T) {
	index, archive, key := signedFixture(t, []byte("fixture library"))
	withTestComponent(t, index, archive, key)
	probeErr := errors.New("libass.so.9: cannot open shared object file")
	componentProbe = func(_ context.Context, library string) error {
		if _, err := os.Stat(library); err != nil {
			t.Fatalf("probe didn't receive the extracted library: %v", err)
		}
		return probeErr
	}
	if err := Download(t.Context(), nil); !errors.Is(err, probeErr) {
		t.Fatalf("download lost the native error: %v", err)
	}
	if Ready() || IsDownloaded() {
		t.Fatal("an unusable staged component was installed")
	}
	if files, err := os.ReadDir(cacheDirOverride); err != nil || len(files) != 0 {
		t.Fatalf("failed probe left staging files: %v, %v", files, err)
	}
}

// An index signed by a different key cannot trigger an archive download or native probe.
func TestDownloadRequiresOfficialSignature(t *testing.T) {
	index, archive, _ := signedFixture(t, []byte("fixture library"))
	key, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	withTestComponent(t, index, archive, key)
	componentProbe = func(context.Context, string) error {
		t.Fatal("unverified component reached the probe")
		return nil
	}
	if err := Download(t.Context(), nil); err == nil || IsDownloaded() {
		t.Fatalf("unverified component was accepted: %v", err)
	}
}

// Reinstalling the verified files already mapped by the decoder immediately re-enables playback after removal.
func TestReinstallMappedComponentRestoresReadiness(t *testing.T) {
	index, archive, key := signedFixture(t, []byte("fixture library"))
	withTestComponent(t, index, archive, key)
	componentProbe = func(context.Context, string) error { return nil }
	if err := Download(t.Context(), nil); err != nil || !Ready() {
		t.Fatalf("first install did not enable MPV: %v", err)
	}
	mapped = installed
	original := mapped.path
	if err := Delete(); err != nil || Ready() || IsDownloaded() {
		t.Fatalf("delete did not deactivate MPV: %v", err)
	}
	if _, err := os.Stat(original); err != nil {
		t.Fatalf("mapped library was removed early: %v", err)
	}
	if err := Download(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if !Ready() || !IsDownloaded() || NeedsRestart() {
		t.Fatal("the same reinstalled component required a restart")
	}
	if installed.path == original || mapped.path != original {
		t.Fatal("reinstall did not persist a new path while retaining the mapped library")
	}
	if err := Load(); err != nil {
		t.Fatalf("reinstalled mapped library could not be enabled: %v", err)
	}
}

// A changed payload cannot reactivate a different library left mapped after removal.
func TestReinstallDifferentComponentWaitsForRestart(t *testing.T) {
	index, archive, key := signedFixture(t, []byte("new fixture library"))
	withTestComponent(t, index, archive, key)
	componentProbe = func(context.Context, string) error { return nil }
	mapped = componentLibrary{path: filepath.Join(cacheDirOverride, "package-old", "fixture.dll"),
		files: []componentFile{{Name: "fixture.dll", SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("old")))}}}
	if err := Delete(); err != nil {
		t.Fatal(err)
	}
	if err := Download(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if Ready() || !IsDownloaded() || !NeedsRestart() || !errors.Is(Load(), ErrUnavailable) {
		t.Fatal("a different payload was activated before the previous process released the library")
	}
}

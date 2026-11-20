package mpv

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Canceling a fully received libmpv download before publication leaves the previously installed file intact.
func TestDownloadCanceledBeforePublish(t *testing.T) {
	contents := bytes.Repeat([]byte("libmpv download fixture"), 256)
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(compressed.Len()))
		_, _ = w.Write(compressed.Bytes())
	}))
	defer server.Close()
	dir := t.TempDir()
	base, previousCache := baseDownloadURL, cacheDirOverride
	system := Platform()
	previousAsset, registered := NativeAssets[system]
	defer func() {
		SetBaseURL(base)
		SetCacheDir(previousCache)
		if registered {
			NativeAssets[system] = previousAsset
		} else {
			delete(NativeAssets, system)
		}
	}()
	SetBaseURL(server.URL + "/")
	SetCacheDir(dir)
	NativeAssets[system] = nativeFile{
		Name: "libmpv-fixture", Asset: "fixture.gz", SHA256: fmt.Sprintf("%x", sha256.Sum256(contents)),
	}
	installed := []byte("original decoder")
	path := filepath.Join(dir, "libmpv-fixture")
	if err := os.WriteFile(path, installed, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	err := Download(ctx, func(done, total int64) {
		if done == total {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("final publish did not observe cancellation: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, installed) {
		t.Fatalf("canceled download changed the original library: %q, %v", got, err)
	}
	if files, err := os.ReadDir(dir); err != nil || len(files) != 1 {
		t.Fatalf("canceled download left a staged file: %v, %v", files, err)
	}
}

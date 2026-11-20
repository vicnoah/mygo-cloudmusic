package pluginregistry

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"testing"
)

// Reviewing a community repository pins the signed release and rejects a subsequent substitution.
func TestReviewGitHubPinsManifest(t *testing.T) {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	const repo = "listener/library"
	const tagBase = "https://github.com/" + repo + "/releases/download/v1.0.0/"
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte("test executable")))
	entry := CatalogEntry{Package: "listener-library", Manifest: tagBase + "manifest.json", PublicKey: public}
	release := Release{
		Format: 1, Protocol: 1, Package: entry.Package, Version: "1.0.0", Name: "Library",
		Entries: []string{entry.Package},
		Artifacts: []Artifact{{
			OS: runtime.GOOS, Arch: runtime.GOARCH,
			URL: tagBase + "library.zip", Entry: "library.exe", Size: 24, SHA256: digest, EntrySHA256: digest,
		}},
	}
	var artifactsFetched int
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body []byte
		switch req.URL.String() {
		case "https://github.com/" + repo + latestCatalog:
			body, _ = json.Marshal(Catalog{Format: 1, Packages: []CatalogEntry{entry}})
		case tagBase + "manifest.json":
			raw, _ := json.Marshal(release)
			body, _ = json.Marshal(SignedManifest{Manifest: raw, Signature: ed25519.Sign(private, raw)})
		default:
			artifactsFetched++
			return nil, fmt.Errorf("unexpected request %s", req.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})}
	registry, err := New(t.TempDir(), client)
	if err != nil {
		t.Fatal(err)
	}
	review, err := registry.ReviewGitHub(t.Context(), "Listener/Library", false)
	if err != nil {
		t.Fatal(err)
	}
	if review.Source.Package != "listener-library" || review.Source.GitHubRepo != repo ||
		!bytes.Equal(review.Source.TrustedKey, public) || review.Release.Version != "1.0.0" || artifactsFetched != 0 {
		t.Fatalf("wrong review or binary fetched during review: %+v, requests %d", review, artifactsFetched)
	}
	release.Name = "Changed after confirmation"
	_, err = registry.Install(t.Context(), review.Source, func(_ context.Context, _ Program) error {
		t.Fatal("substituted release was executed")
		return nil
	})
	if err == nil || artifactsFetched != 0 {
		t.Fatalf("substitution reached the download: %v, requests %d", err, artifactsFetched)
	}
	release.Artifacts[0].URL = "https://github.com/other/library/releases/download/v1.0.0/library.zip"
	if _, err := registry.ReviewGitHub(t.Context(), repo, false); err == nil {
		t.Fatal("review accepted an archive from a different repository")
	}
}

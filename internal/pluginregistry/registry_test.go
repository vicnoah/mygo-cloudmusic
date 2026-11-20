package pluginregistry

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
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// A catalog supplies the signing key for a first install, and failed upgrades preserve the installed release.
func TestRegistryInstallUpdateAndRollback(t *testing.T) {
	public, secret, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	replacement, replacementSecret, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	const executable = "platform.exe"
	versions := make(map[string][]byte)
	for _, version := range []string{"1.0", "1.1", "1.2"} {
		var body bytes.Buffer
		z := zip.NewWriter(&body)
		f, err := z.Create(executable)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(version)); err != nil {
			t.Fatal(err)
		}
		if err := z.Close(); err != nil {
			t.Fatal(err)
		}
		versions[version] = body.Bytes()
	}
	var latest atomic.Value
	latest.Store("1.0")
	var badSignature atomic.Bool
	var badProtocol atomic.Bool
	var newPublisher, omitKey atomic.Bool
	var manifestRequests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		base := "https://" + req.Host
		version := latest.Load().(string)
		switch req.URL.Path {
		case "/catalog":
			key := public
			if newPublisher.Load() {
				key = replacement
			}
			if omitKey.Load() {
				key = nil
			}
			_ = json.NewEncoder(w).Encode(Catalog{Format: 1, Packages: []CatalogEntry{{Package: "songloft", Manifest: base + "/release", PublicKey: key}}})
		case "/release":
			manifestRequests.Add(1)
			archive := versions[version]
			var protocolVersion uint16 = 1
			if badProtocol.Load() {
				protocolVersion = 2
			}
			release, _ := json.Marshal(Release{
				Format: 1, Protocol: protocolVersion, Package: "songloft", Version: version, Entries: []string{"songloft"},
				Artifacts: []Artifact{{
					OS: runtime.GOOS, Arch: runtime.GOARCH, URL: base + "/archive", Entry: executable,
					Size: int64(len(archive)), SHA256: fmt.Sprintf("%x", sha256.Sum256(archive)),
					EntrySHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(version))),
				}},
			})
			signer := secret
			if newPublisher.Load() {
				signer = replacementSecret
			}
			signature := ed25519.Sign(signer, release)
			if badSignature.Load() {
				signature[0] ^= 1
			}
			_ = json.NewEncoder(w).Encode(SignedManifest{Manifest: release, Signature: signature})
		case "/archive":
			_, _ = w.Write(versions[version])
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()
	registry, err := New(t.TempDir(), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	source := Source{Catalog: server.URL + "/catalog", Package: "songloft"}
	probe := func(_ context.Context, p Program) error {
		b, err := os.ReadFile(p.Path)
		if err != nil {
			return err
		}
		if string(b) != p.Version.Version {
			return errors.New("program content differs from its advertised version")
		}
		if string(b) == "1.2" {
			return errors.New("program rejected the host")
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	requireInstalled := func(want string) {
		t.Helper()
		p, ok, err := registry.Current("songloft")
		if err != nil || !ok || p.Version.Version != want {
			t.Fatalf("current version = %q, found %t, error %v, want %q", p.Version.Version, ok, err, want)
		}
		if err := probe(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := registry.Install(ctx, source, probe); err != nil {
		t.Fatal(err)
	}
	requireInstalled("1.0")
	latest.Store("1.1")
	before := manifestRequests.Load()
	newPublisher.Store(true)
	if _, err := registry.Install(ctx, source, probe); err == nil || manifestRequests.Load() != before {
		t.Fatalf("an update changed the installed publisher or fetched its manifest: %v", err)
	}
	requireInstalled("1.0")
	newPublisher.Store(false)
	badSignature.Store(true)
	if _, err := registry.Install(ctx, source, probe); err == nil {
		t.Fatal("installed an unsigned update")
	}
	requireInstalled("1.0")
	badSignature.Store(false)
	badProtocol.Store(true)
	if _, err := registry.Install(ctx, source, probe); err == nil || err.Error() != "unsupported platform protocol version" {
		t.Fatalf("incompatible signed release was not rejected: %v", err)
	}
	requireInstalled("1.0")
	badProtocol.Store(false)
	if _, err := registry.Install(ctx, source, probe); err != nil {
		t.Fatal(err)
	}
	requireInstalled("1.1")
	latest.Store("1.2")
	if _, err := registry.Install(ctx, source, probe); err == nil {
		t.Fatal("installed an update that failed its compatibility check")
	}
	requireInstalled("1.1")
	if _, err := registry.Rollback(ctx, "songloft", probe); err != nil {
		t.Fatal(err)
	}
	requireInstalled("1.0")
	if err := registry.Unregister("songloft"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := registry.Current("songloft"); err != nil || ok {
		t.Fatalf("unregistered package still available: %t %v", ok, err)
	}
	latest.Store("1.0")
	omitKey.Store(true)
	if _, err := registry.Install(ctx, source, probe); err == nil {
		t.Fatal("installed a release without a discovered or previously saved publisher key")
	}
	source.TrustedKey = public
	if _, err := registry.Install(ctx, source, probe); err != nil {
		t.Fatal(err)
	}
	requireInstalled("1.0")
}

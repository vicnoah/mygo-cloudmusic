// Package pluginregistry manages verified platform releases in the host's local registry.
package pluginregistry

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/waygu-music-plugin/sdk/protocol"
)

const maxMetadata = 1 << 20
const maxArchive = 256 << 20

type Catalog struct {
	Format   int
	Packages []CatalogEntry
}

type CatalogEntry struct {
	Package   string
	Name      string `json:",omitempty"`
	Manifest  string
	PublicKey ed25519.PublicKey `json:"PublicKey,omitempty"`
}

type Source struct {
	Catalog        string
	Package        string
	UseGitHubProxy bool
	// TrustedKey pins the publisher when its key is already known.
	TrustedKey ed25519.PublicKey
	// GitHubRepo restricts the release and its artifacts to this repository.
	GitHubRepo string
	// ReviewedManifest binds installation to the exact release shown for confirmation.
	ReviewedManifest [sha256.Size]byte
}

type SignedManifest struct {
	Manifest  json.RawMessage
	Signature []byte
}

type Release struct {
	Format           int
	Protocol         uint16
	Package, Version string
	Name             string `json:",omitempty"`
	Entries          []string
	Artifacts        []Artifact
	manifestHash     [sha256.Size]byte
}

type Artifact struct {
	OS, Arch    string
	URL, Entry  string
	Size        int64
	SHA256      string
	EntrySHA256 string
}

func (a Artifact) executablePath() (string, error) {
	path, err := localArchivePath(a.Entry)
	if err != nil || a.Entry == "." ||
		(runtime.GOOS == "windows" && !strings.EqualFold(filepath.Ext(path), ".exe")) {
		return "", errors.New("invalid executable path in plugin manifest")
	}
	return path, nil
}

func (s Source) lookup(ctx context.Context, client *http.Client) (CatalogEntry, error) {
	if !safeID(s.Package) || (len(s.TrustedKey) != 0 && len(s.TrustedKey) != ed25519.PublicKeySize) {
		return CatalogEntry{}, errors.New("invalid plugin source")
	}
	catalog, err := readCatalog(ctx, client, s.Catalog, s.UseGitHubProxy)
	if err != nil {
		return CatalogEntry{}, err
	}
	var selected CatalogEntry
	for _, entry := range catalog.Packages {
		if entry.Package == s.Package {
			selected = entry
			break
		}
	}
	if selected.Manifest == "" {
		return CatalogEntry{}, errors.New("package not found in plugin catalog")
	}
	return selected.withPublisher(s.TrustedKey)
}

func (e CatalogEntry) withPublisher(key ed25519.PublicKey) (CatalogEntry, error) {
	if len(e.PublicKey) == 0 {
		e.PublicKey = key
	}
	if len(e.PublicKey) != ed25519.PublicKeySize {
		return CatalogEntry{}, errors.New("plugin catalog is missing a valid publisher key")
	}
	if len(key) != 0 && !bytes.Equal(key, e.PublicKey) {
		return CatalogEntry{}, errors.New("plugin release is from a different source or key")
	}
	return e, nil
}

func readCatalog(ctx context.Context, client *http.Client, raw string, useProxy bool) (Catalog, error) {
	var catalog Catalog
	if err := getJSON(ctx, client, raw, useProxy, &catalog); err != nil {
		return Catalog{}, err
	}
	if catalog.Format != 1 {
		return Catalog{}, errors.New("unsupported plugin catalog format")
	}
	seen := make(map[string]bool)
	for _, entry := range catalog.Packages {
		if !safeID(entry.Package) || seen[entry.Package] || entry.Manifest == "" {
			return Catalog{}, errors.New("invalid package entry in plugin catalog")
		}
		seen[entry.Package] = true
	}
	return catalog, nil
}

func (e CatalogEntry) release(ctx context.Context, client *http.Client, useGitHubProxy bool) (Release, error) {
	var signed SignedManifest
	if err := getJSON(ctx, client, e.Manifest, useGitHubProxy, &signed); err != nil {
		return Release{}, err
	}
	if !ed25519.Verify(e.PublicKey, signed.Manifest, signed.Signature) {
		return Release{}, errors.New("plugin manifest signature is invalid")
	}
	var release Release
	if err := json.Unmarshal(signed.Manifest, &release); err != nil {
		return Release{}, err
	}
	if release.Format != 1 || release.Package != e.Package || !safeID(release.Version) ||
		len(release.Entries) == 0 {
		return Release{}, errors.New("invalid signed plugin manifest")
	}
	if release.Protocol != protocol.Version {
		return Release{}, errors.New("unsupported platform protocol version")
	}
	for i, id := range release.Entries {
		if !safeID(id) || slices.Contains(release.Entries[:i], id) {
			return Release{}, errors.New("invalid plugin entry ID")
		}
	}
	release.manifestHash = sha256.Sum256(signed.Manifest)
	return release, nil
}

func (r Release) artifact(goos, goarch string) (Artifact, error) {
	var found *Artifact
	for _, item := range r.Artifacts {
		if item.OS == goos && item.Arch == goarch {
			if found != nil {
				return Artifact{}, errors.New("release declares more than one artifact for this platform")
			}
			found = &item
		}
	}
	if found == nil || found.Size <= 0 || found.Size > maxArchive {
		return Artifact{}, errors.New("no supported artifact for this platform")
	}
	if _, err := checksum(found.SHA256); err != nil {
		return Artifact{}, err
	}
	if _, err := checksum(found.EntrySHA256); err != nil {
		return Artifact{}, err
	}
	return *found, nil
}

func checksum(value string) ([32]byte, error) {
	b, err := hex.DecodeString(value)
	if err != nil || len(b) != 32 {
		return [32]byte{}, errors.New("invalid SHA-256 in plugin manifest")
	}
	return [32]byte(b), nil
}

func getJSON(ctx context.Context, client *http.Client, raw string, useGitHubProxy bool, dst any) error {
	response, err := get(ctx, client, raw, useGitHubProxy)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	b, err := io.ReadAll(io.LimitReader(response.Body, maxMetadata+1))
	if err != nil {
		return err
	}
	if len(b) > maxMetadata {
		return errors.New("plugin metadata exceeds size limit")
	}
	return json.Unmarshal(b, dst)
}

func get(ctx context.Context, client *http.Client, raw string, useGitHubProxy bool) (*http.Response, error) {
	u, err := url.Parse(raw)
	if err != nil || !allowedURL(u) {
		return nil, errors.New("plugin source must use an HTTPS URL")
	}
	u.Fragment = ""
	requestURL := u.String()
	if useGitHubProxy && githubDownload(u) {
		requestURL = "https://gh-proxy.com/" + requestURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	c := *client
	c.CheckRedirect = func(req *http.Request, previous []*http.Request) error {
		if len(previous) >= 8 || !allowedURL(req.URL) {
			return errors.New("invalid plugin download redirect")
		}
		if client.CheckRedirect != nil {
			return client.CheckRedirect(req, previous)
		}
		return nil
	}
	response, err := c.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("plugin source returned HTTP %d", response.StatusCode)
	}
	return response, nil
}

func allowedURL(u *url.URL) bool { return u.Scheme == "https" && u.Host != "" && u.User == nil }

func githubDownload(u *url.URL) bool {
	switch strings.ToLower(u.Host) {
	case "github.com", "raw.githubusercontent.com", "raw.github.com", "api.github.com":
		return true
	}
	return false
}

func safeID(s string) bool {
	if s == "" || len(s) > 96 || !filepath.IsLocal(s) || strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") {
		return false
	}
	for _, ch := range s {
		if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' && ch != '_' && ch != '.' {
			return false
		}
	}
	return true
}

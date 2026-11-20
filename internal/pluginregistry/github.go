package pluginregistry

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"runtime"
	"strings"

	"waygu-music/internal/platformcatalog"
)

const github = "https://github.com/"
const latestCatalog = "/releases/latest/download/catalog.json"

var repositoryName = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,37}[a-z0-9])?/[a-z0-9_.-]{1,100}$`)

// GitHubRepository normalizes a repository name to the install link's identity.
func GitHubRepository(value string) (string, error) {
	value = strings.ToLower(value)
	if !repositoryName.MatchString(value) || path.Base(value) == "." || path.Base(value) == ".." {
		return "", errors.New("expected a GitHub repository in owner/repo form")
	}
	return value, nil
}

func repositoryForCatalog(raw string) string {
	repo, hasPrefix := strings.CutPrefix(raw, github)
	repo, hasSuffix := strings.CutSuffix(repo, latestCatalog)
	if !hasPrefix || !hasSuffix {
		return ""
	}
	repo, _ = GitHubRepository(repo)
	return repo
}

// GitHubReleaseAsset reads the repository and tag from a canonical asset URL.
func GitHubReleaseAsset(raw string) (repo, tag, name string) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return "", "", ""
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) != 6 || parts[2] != "releases" || parts[3] != "download" ||
		!safeID(strings.ToLower(parts[4])) || parts[5] == "" || parts[5] == "." || parts[5] == ".." {
		return "", "", ""
	}
	repo, err = GitHubRepository(parts[0] + "/" + parts[1])
	if err != nil {
		return "", "", ""
	}
	return repo, parts[4], parts[5]
}

// GitHubReview contains verified metadata. Preparing it never downloads or
// executes a plugin binary.
type GitHubReview struct {
	Source  Source
	Release Release
}

func (r *Registry) ReviewGitHub(ctx context.Context, repository string, useProxy bool) (GitHubReview, error) {
	repo, err := GitHubRepository(repository)
	if err != nil {
		return GitHubReview{}, err
	}
	source := Source{GitHubRepo: repo, UseGitHubProxy: useProxy}
	if strings.HasPrefix(repo, platformcatalog.Organization+"/") {
		index, err := readCatalog(ctx, r.client, platformcatalog.OfficialCatalog, useProxy)
		if err != nil {
			return GitHubReview{}, err
		}
		for _, entry := range index.Packages {
			owner, _, asset := GitHubReleaseAsset(entry.Manifest)
			if owner == repo && asset == "manifest.json" {
				source.Catalog = platformcatalog.OfficialCatalog
				source.TrustedKey, err = base64.StdEncoding.DecodeString(platformcatalog.OfficialPublisherKey)
				if err != nil {
					return GitHubReview{}, err
				}
				return r.review(ctx, source, entry)
			}
		}
	}
	source.Catalog = github + repo + latestCatalog
	index, err := readCatalog(ctx, r.client, source.Catalog, useProxy)
	if err != nil {
		return GitHubReview{}, err
	}
	if len(index.Packages) != 1 {
		return GitHubReview{}, errors.New("a GitHub plugin release must declare exactly one package")
	}
	// These identifiers may own existing accounts and settings from earlier hosts.
	switch index.Packages[0].Package {
	case "fnmusic", "songloft", "subsonic", "navidrome", "jellyfin", "gonic", "netease", "qqmusic":
		return GitHubReview{}, errors.New("this package ID belongs to a built-in or official plugin")
	}
	return r.review(ctx, source, index.Packages[0])
}

func (r *Registry) review(ctx context.Context, source Source, entry CatalogEntry) (GitHubReview, error) {
	entry, err := entry.withPublisher(source.TrustedKey)
	if err != nil {
		return GitHubReview{}, err
	}
	release, err := source.checkedRelease(ctx, r.client, entry)
	if err != nil {
		return GitHubReview{}, err
	}
	source.Package = entry.Package
	source.TrustedKey = entry.PublicKey
	source.ReviewedManifest = release.manifestHash
	return GitHubReview{Source: source, Release: release}, nil
}

func (review GitHubReview) CurrentArtifact() (Artifact, error) {
	return review.Release.artifact(runtime.GOOS, runtime.GOARCH)
}

func (s Source) checkedRelease(ctx context.Context, client *http.Client, entry CatalogEntry) (Release, error) {
	repo := s.GitHubRepo
	if repo == "" {
		repo = repositoryForCatalog(s.Catalog)
	}
	var tag string
	if repo != "" {
		var manifestRepo, name string
		manifestRepo, tag, name = GitHubReleaseAsset(entry.Manifest)
		if manifestRepo != repo || name != "manifest.json" {
			return Release{}, errors.New("plugin manifest is outside the repository's GitHub release")
		}
	}
	release, err := entry.release(ctx, client, s.UseGitHubProxy)
	if err != nil {
		return Release{}, err
	}
	if s.ReviewedManifest != ([sha256.Size]byte{}) && s.ReviewedManifest != release.manifestHash {
		return Release{}, errors.New("the plugin release changed after confirmation")
	}
	if repo == "" {
		return release, nil
	}
	if strings.TrimPrefix(tag, "v") != release.Version || len(release.Artifacts) == 0 {
		return Release{}, errors.New("plugin manifest version or targets do not match the release")
	}
	for _, item := range release.Artifacts {
		if err := validateGitHubArtifact(item, repo, tag); err != nil {
			return Release{}, err
		}
		if _, err := release.artifact(item.OS, item.Arch); err != nil {
			return Release{}, err
		}
	}
	return release, nil
}

func validateGitHubArtifact(item Artifact, repo, tag string) error {
	assetRepo, assetTag, file := GitHubReleaseAsset(item.URL)
	if assetRepo != repo || assetTag != tag || !strings.HasSuffix(file, ".zip") {
		return errors.New("plugin archive is outside the repository's GitHub release")
	}
	if item.OS != "windows" && item.OS != "darwin" && item.OS != "linux" || item.Arch != "amd64" && item.Arch != "arm64" {
		return fmt.Errorf("unsupported plugin target %s/%s", item.OS, item.Arch)
	}
	executable, err := localArchivePath(item.Entry)
	if err != nil || executable == "." || item.OS == "windows" && !strings.HasSuffix(strings.ToLower(executable), ".exe") {
		return errors.New("invalid executable path in plugin manifest")
	}
	return nil
}

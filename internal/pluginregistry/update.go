package pluginregistry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"

	"golang.org/x/mod/semver"
)

// ReviewUpdate looks for a newer compatible release from the installed
// publisher. It fetches metadata only and binds a later install to that release.
func (r *Registry) ReviewUpdate(ctx context.Context, source Source, entryID string) (GitHubReview, error) {
	r.mu.Lock()
	index, err := r.read()
	r.mu.Unlock()
	if err != nil {
		return GitHubReview{}, err
	}
	record, ok := index[source.Package]
	if !ok || record.Source != source.Catalog || !slices.Contains(record.Current.Platforms, entryID) {
		return GitHubReview{}, nil
	}
	entry, err := source.lookup(ctx, r.client)
	if err != nil {
		return GitHubReview{}, err
	}
	key := sha256.Sum256(entry.PublicKey)
	if hex.EncodeToString(key[:]) != record.Key {
		return GitHubReview{}, errors.New("plugin release is from a different source or key")
	}
	review, err := r.review(ctx, source, entry)
	if err != nil {
		return GitHubReview{}, err
	}
	current := "v" + strings.TrimPrefix(record.Current.Version, "v")
	latest := "v" + strings.TrimPrefix(review.Release.Version, "v")
	if !semver.IsValid(current) || !semver.IsValid(latest) || semver.Compare(latest, current) <= 0 ||
		!slices.Contains(review.Release.Entries, entryID) {
		return GitHubReview{}, nil
	}
	artifact, err := review.CurrentArtifact()
	if err != nil {
		return GitHubReview{}, err
	}
	if _, err := artifact.executablePath(); err != nil {
		return GitHubReview{}, err
	}
	return review, nil
}

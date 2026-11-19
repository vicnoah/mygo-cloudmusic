package pluginapi

import (
	"net/url"
	"strings"
)

const artworkScheme = "artwork://"

// ArtworkURL gives an opaque provider reference an instance-specific cache key.
func ArtworkURL(instance ID, ref string) string {
	if instance == "" || ref == "" {
		return ref
	}
	return artworkScheme + url.PathEscape(string(instance)) + "/" + url.PathEscape(ref)
}

// ArtworkOwner resolves a scoped reference; the outer query holds display options.
func ArtworkOwner(raw string) (instance ID, ref string, ok bool) {
	path, ok := strings.CutPrefix(raw, artworkScheme)
	if !ok {
		return "", "", false
	}
	path, _, _ = strings.Cut(path, "?")
	owner, encoded, ok := strings.Cut(path, "/")
	if !ok || owner == "" || encoded == "" {
		return "", "", false
	}
	id, err := url.PathUnescape(owner)
	if err != nil {
		return "", "", false
	}
	ref, err = url.PathUnescape(encoded)
	if err != nil {
		return "", "", false
	}
	return ID(id), ref, true
}

package subsonic

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	backend "waygu-music/internal/plugins/subsonic/internal/client"
)

// Older queue and history entries stored cover references without an owner.
func (h *Home) artworkURL(raw string) string {
	if strings.HasPrefix(raw, backend.CoverScheme) {
		return pluginapi.ArtworkURL(h.state.Profile().ID, raw)
	}
	return raw
}

func (h *Home) artwork(raw string, size int) (*ui.Bitmap, bool) {
	return h.host.Artwork(h.artworkURL(raw), size)
}

func (h *Home) decodedArtwork(raw string, size int, blurred bool) pluginapi.DecodedArtwork {
	return h.host.DecodedArtwork(h.artworkURL(raw), size, blurred)
}

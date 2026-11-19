package subsonic

import (
	"context"
	"testing"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/paging"
	"waygu-music/internal/plugins/internal/plugintest"
)

// A list left during shuffle cannot replace playback, and the same list can be shuffled on return.
func TestShuffleFollowsActivation(t *testing.T) {
	pages := paging.NewScoped[pluginapi.Track](t.Context(), PageSize, 0, nil, nil)
	pages.Land(0, []pluginapi.Track{{ID: 1}}, 1)
	list := &List{Pages: pages}
	plugintest.CheckShuffleActivation(t, func(ctx context.Context, p pluginapi.PlayerPort, post func(func())) {
		ShuffleList(ctx, p, list, post)
	})
}

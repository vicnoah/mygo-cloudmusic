package fnmusic

import (
	"testing"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/eq"
	"waygu-music/internal/plugins/internal/plugintest"
)

// A headphone profile from a previous activation cannot change playback after returning.
func TestAutoEQResultFollowsActivation(t *testing.T) {
	plugintest.CheckAutoEQActivation(t, Registration(), func(instance pluginapi.Adapter, auto *eq.AutoEQ, profile eq.Meta) {
		h := instance.(*Adapter).Home()
		if h.eqUI.autoeqBusy != "" {
			t.Fatal("the previous request left the profile picker busy")
		}
		h.eqUI.autoeq = auto
		h.applyAutoEQ(profile)
	})
}

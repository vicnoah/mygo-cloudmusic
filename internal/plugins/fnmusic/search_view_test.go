package fnmusic

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/paging"
)

// Opening search starts its first song request and a retry makes the result playable.
func TestSearchStartsSongPageAndRetries(t *testing.T) {
	home := NewHome(NewState(), nopHost{})
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	posted := make(chan func(), 2)
	attempts := 0
	songs := paging.NewScoped(ctx, 20, 0, func(fn func()) { posted <- fn }, func(_ context.Context, _ int) ([]pluginapi.Track, int, error) {
		attempts++
		if attempts == 1 {
			return nil, 0, errors.New("search service unavailable")
		}
		return []pluginapi.Track{{ID: 1, Name: "Search result"}}, 1, nil
	})
	played := -1
	view := SearchView{Play: func(i int) { played = i }}
	tt := ui.NewTester(func(c *ui.Context) {
		home.renderSearch(c, SearchResults{Query: "search", Songs: songs}, view)
	}, 1200, 800)
	landRequest := func(when string) {
		t.Helper()
		select {
		case apply := <-posted:
			apply()
			tt.Frame()
		case <-ctx.Done():
			t.Fatalf("the song request did not complete %s", when)
		}
	}
	landRequest("after opening search")
	if err := tt.Click("重试"); err != nil {
		t.Fatal(err)
	}
	landRequest("after clicking retry")
	if err := tt.Click("Search result"); err != nil {
		t.Fatal(err)
	}
	if played != 0 {
		t.Fatalf("clicking the result played row %d, want 0", played)
	}
}

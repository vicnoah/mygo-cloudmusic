package fnmusic

import (
	"context"
	"testing"
	"time"

	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/paging"
	"waygu-music/internal/plugins/internal/plugintest"
)

// randomPages keeps the already visible physical page first while using every page once.
func TestRandomPagesKeepsBaseAndPermutes(t *testing.T) {
	got := randomPages(250, 42, 0)
	if len(got) != 3 || got[0] != 0 {
		t.Fatalf("pages = %#v", got)
	}
	seen := map[int]bool{}
	for _, pg := range got {
		if seen[pg] {
			t.Fatalf("duplicate page %d", pg)
		}
		seen[pg] = true
	}
}

type queuePlayer struct {
	tracks []pluginapi.Track
	at     int
	feed   func(func([]pluginapi.Track))
	ready  chan struct{}
}

func (p *queuePlayer) Current() *pluginapi.Track        { return nil }
func (p *queuePlayer) Playing() bool                    { return false }
func (p *queuePlayer) Loading() bool                    { return false }
func (p *queuePlayer) Err() string                      { return "" }
func (p *queuePlayer) TogglePlay()                      {}
func (p *queuePlayer) Next()                            {}
func (p *queuePlayer) Queue() []pluginapi.Track         { return nil }
func (p *queuePlayer) PlayOrder() ([]int, int)          { return nil, -1 }
func (p *queuePlayer) PlayQueue([]pluginapi.Track, int) {}
func (p *queuePlayer) AppendQueue([]pluginapi.Track)    {}
func (p *queuePlayer) PlayList(ts []pluginapi.Track, at int, f func(func([]pluginapi.Track))) {
	p.tracks, p.at, p.feed = ts, at, f
	if p.ready != nil {
		close(p.ready)
	}
}
func (p *queuePlayer) PlayRadio(string, []pluginapi.Track, func(func([]pluginapi.Track))) {}
func (p *queuePlayer) EndRadio()                                                          {}
func (p *queuePlayer) Radio() string                                                      { return "" }
func (p *queuePlayer) SetShuffle(bool)                                                    {}

// ShuffleList starts with two physical pages, so the first audible batch is large enough without loading the catalog.
func TestShuffleListUsesTwoPages(t *testing.T) {
	tracks := func(page int) ([]pluginapi.Track, int, error) {
		out := make([]pluginapi.Track, 0, 100)
		for i := 0; i < 100 && (page-1)*100+i < 250; i++ {
			out = append(out, pluginapi.Track{ID: int64((page-1)*100 + i + 1), Ref: "x"})
		}
		return out, 250, nil
	}
	pager := paging.New(100, 0, nil, tracks)
	l := &List{Pages: pager}
	p := &queuePlayer{ready: make(chan struct{})}
	ShuffleList(t.Context(), p, l, nil)
	select {
	case <-p.ready:
	case <-time.After(time.Second):
		t.Fatal("shuffle did not start")
	}
	if len(p.tracks) <= 100 || len(p.tracks) > 200 {
		t.Fatalf("first shuffled batch = %d tracks", len(p.tracks))
	}
	if p.at != 0 {
		t.Fatalf("start offset = %d", p.at)
	}
}

// A list left during shuffle cannot replace playback, and the same list can be shuffled on return.
func TestShuffleFollowsActivation(t *testing.T) {
	pages := paging.NewScoped[pluginapi.Track](t.Context(), PageSize, 0, nil, nil)
	pages.Land(0, []pluginapi.Track{{ID: 1}}, 1)
	list := &List{Pages: pages}
	plugintest.CheckShuffleActivation(t, func(ctx context.Context, p pluginapi.PlayerPort, post func(func())) {
		ShuffleList(ctx, p, list, post)
	})
}

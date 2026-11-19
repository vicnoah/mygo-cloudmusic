package plugintest

import (
	"context"
	"testing"

	"waygu-music/internal/pluginapi"
)

type shufflePlayer struct {
	pluginapi.PlayerPort
	starts int
}

func (p *shufflePlayer) PlayList([]pluginapi.Track, int, func(func([]pluginapi.Track))) {
	p.starts++
}

func CheckShuffleActivation(t *testing.T, shuffle func(context.Context, pluginapi.PlayerPort, func(func()))) {
	t.Helper()
	first, cancel := context.WithCancel(t.Context())
	defer cancel()
	p := &shufflePlayer{}
	var posted []func()
	post := func(fn func()) { posted = append(posted, fn) }

	shuffle(first, p, post)
	if len(posted) != 1 {
		t.Fatalf("the shuffle did not schedule playback: %d callbacks", len(posted))
	}
	cancel()
	shuffle(t.Context(), p, post)
	if len(posted) != 2 {
		t.Fatalf("the previous shuffle blocked the new request: %d callbacks", len(posted))
	}
	posted[0]()
	if p.starts != 0 {
		t.Fatal("the shuffle started playback after its activation ended")
	}
	posted[1]()
	if p.starts != 1 {
		t.Fatal("the new shuffle did not start playback")
	}
}

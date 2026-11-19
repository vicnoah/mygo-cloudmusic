package qqmusic

import (
	"context"
	"testing"

	"waygu-music/internal/pluginapi"
	backend "waygu-music/internal/plugins/qqmusic/internal/client"
)

type commentTestHost struct {
	Host
	pending chan func()
}

func (h commentTestHost) CurrentPlatform() pluginapi.ID { return ID }
func (h commentTestHost) Post(fn func())                { h.pending <- fn }

// A delayed comment page cannot replace a new request for the same track.
func TestQQCommentsIgnoreSupersededPage(t *testing.T) {
	host := commentTestHost{pending: make(chan func())}
	home := NewHome(NewState(), host)
	previous := loadQQCommentPage
	defer func() { loadQQCommentPage = previous }()
	type request struct {
		thread string
		reply  chan int
	}
	started := make(chan request)
	loadQQCommentPage = func(_ context.Context, thread string, _, _ int) (*backend.Comments, error) {
		reply := make(chan int)
		started <- request{thread, reply}
		return &backend.Comments{Total: <-reply}, nil
	}

	track := pluginapi.Track{ID: 17}
	home.commentsFor(&track)
	first := <-started
	home.commentsFor(&pluginapi.Track{ID: 23})
	other := <-started
	other.reply <- 21
	(<-host.pending)()
	home.commentsFor(&track)
	current := <-started

	first.reply <- 45
	(<-host.pending)()
	stillLoading := home.comments.loading
	current.reply <- 12
	(<-host.pending)()
	if first.thread != "17" || current.thread != "17" {
		t.Fatalf("request used threads %q and %q for track 17", first.thread, current.thread)
	}
	if !stillLoading || home.comments.total != 12 {
		t.Fatalf("late page changed the active request, loading = %v, total = %d", stillLoading, home.comments.total)
	}
}

// A completion queued before a platform switch cannot update QQ Music after it returns.
func TestQQResultIgnoresPreviousActivation(t *testing.T) {
	host := commentTestHost{pending: make(chan func(), 1)}
	home := NewHome(NewState(), host)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	home.activate(ctx)
	var applied bool
	home.postResult(ctx, func() { applied = true })
	complete := <-host.pending
	cancel()
	home.CloseTransient()
	home.activate(t.Context())
	complete()
	if applied {
		t.Fatal("page result from an earlier activation was applied")
	}
	home.postResult(t.Context(), func() { applied = true })
	(<-host.pending)()
	if !applied {
		t.Fatal("page result from the current activation was dropped")
	}
}

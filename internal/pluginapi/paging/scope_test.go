package paging

import (
	"context"
	"testing"
)

// A returned plugin keeps loaded rows and can load immediately while an old result is still queued.
func TestPagerScopeKeepsLoadedPages(t *testing.T) {
	old, cancel := context.WithCancel(context.Background())
	defer cancel()
	posts := make(chan func(), 2)
	p := NewScoped(old, 1, 0, func(fn func()) { posts <- fn }, func(ctx context.Context, page int) ([]string, int, error) {
		if ctx == old {
			return []string{"retired"}, 2, nil
		}
		return []string{"current"}, 2, nil
	})
	p.limit = 1
	p.Land(0, []string{"cached"}, 2)
	p.At(1)
	late := <-posts

	cancel()
	p.SetScope(context.Background())
	p.At(1)
	if p.state[1] != pageLoading {
		t.Fatal("an old request blocked the current page from loading")
	}
	(<-posts)()
	late()
	if row := p.Peek(0); row == nil || *row != "cached" {
		t.Fatalf("completed row was lost: %v", row)
	}
	if row := p.Peek(1); row == nil || *row != "current" {
		t.Fatalf("new result was replaced: %v", row)
	}
}

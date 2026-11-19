package fnmusic

// The Feiniu list queue keeps the complete catalog on the server. Sequential
// playback reads contiguous loaded pages; shuffle uses a stable permutation of
// physical pages and combines two non-adjacent pages for each refill.

import (
	"context"
	"math/rand/v2"

	"waygu-music/internal/pluginapi"
)

func PlayList(p PlayerPort, l *List, index int) {
	if p == nil || l == nil || l.Pages == nil {
		return
	}
	items, at, end := l.Pages.Around(index)
	if len(items) == 0 {
		return
	}
	items = append([]pluginapi.Track(nil), items...)
	next := end
	p.PlayList(items, at, func(got func([]pluginapi.Track)) {
		if next >= l.Pages.Len() {
			got(nil)
			return
		}
		pg := next / l.Pages.PageSize()
		l.Pages.When(pg, func(ok bool) {
			if !ok {
				got(nil)
				return
			}
			end := min((pg+1)*l.Pages.PageSize(), l.Pages.Len())
			out := make([]pluginapi.Track, 0, end-next)
			for i := next; i < end; i++ {
				if tr := l.Pages.Peek(i); tr != nil {
					out = append(out, *tr)
				}
			}
			next = end
			got(out)
		})
	})
}

// ShuffleList starts immediately when the first page is available. The page
// already displayed is always part of the first batch, so pressing shuffle
// never discards the list the user is looking at.
func ShuffleList(ctx context.Context, p PlayerPort, l *List, post func(func())) {
	if ctx.Err() != nil || p == nil || l == nil || l.Pages == nil || l.shuffling() {
		return
	}
	l.shuffleScope = ctx
	if post == nil {
		post = func(fn func()) { fn() }
	}
	l.Pages.When(0, func(ok bool) {
		if ctx.Err() != nil {
			return
		}
		if !ok {
			l.shuffleScope = nil
			return
		}
		total := l.Pages.Total()
		seed := rand.Int64()
		pages := randomPages(total, seed, 0)
		feed := &randomFeed{list: l, pages: pages, seed: seed, group: 0}
		feed.next(post, func(first []pluginapi.Track) {
			if ctx.Err() != nil {
				return
			}
			l.shuffleScope = nil
			if len(first) > 0 {
				p.PlayList(first, 0, func(got func([]pluginapi.Track)) { feed.next(post, got) })
			}
		})
	})
}

func randomPages(total int, seed int64, base int) []int {
	if total <= 0 {
		return nil
	}
	n := (total + PageSize - 1) / PageSize
	r := rand.New(rand.NewPCG(uint64(seed), 0x9e3779b97f4a7c15))
	out := r.Perm(n)
	if base >= 0 && base < n {
		for i, pg := range out {
			if pg == base {
				out[0], out[i] = out[i], out[0]
				break
			}
		}
	}
	return out
}

func shuffleBatch(items []pluginapi.Track, seed int64, group int) {
	r := rand.New(rand.NewPCG(uint64(seed)^uint64(group+1)*0x517cc1b727220a95, 0x243f6a8885a308d3^uint64(group)))
	r.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
}

type randomFeed struct {
	list  *List
	pages []int
	seed  int64
	group int
}

func (f *randomFeed) next(post func(func()), done func([]pluginapi.Track)) {
	if f == nil || f.list == nil || f.group*2 >= len(f.pages) {
		done(nil)
		return
	}
	first := f.group * 2
	want := f.pages[first:min(first+2, len(f.pages))]
	items := make(map[int][]pluginapi.Track, len(want))
	left := len(want)
	for _, pg := range want {
		pg := pg
		f.list.Pages.When(pg, func(ok bool) {
			if ok {
				start, end := pg*f.list.Pages.PageSize(), min((pg+1)*f.list.Pages.PageSize(), f.list.Pages.Len())
				for i := start; i < end; i++ {
					if tr := f.list.Pages.Peek(i); tr != nil {
						items[pg] = append(items[pg], *tr)
					}
				}
			}
			left--
			if left != 0 {
				return
			}
			var out []pluginapi.Track
			for _, page := range want {
				out = append(out, items[page]...)
			}
			shuffleBatch(out, f.seed, f.group)
			f.group++
			post(func() { done(out) })
		})
	}
}

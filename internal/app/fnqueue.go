package app

import (
	"encoding/json"
	"math/rand/v2"

	"netease-music-mygo/internal/netease"
)

// A queue of the Feiniu platform is read from a list of songs as it is
// played, in the order of the list or at random: never the whole of it.
//
// At random, the list's pages are shuffled, two pages are read together, and
// the two pages are shuffled once more before they enter the queue. Nothing
// is shuffled that is not there, and every page is used once. The seed and
// how far the queue has got are kept in the session, so that after a restart
// the same order goes on.

// fnBatch is the logical size of a random refill. A normal refill joins two
// physical list pages; the first refill can reuse the page already shown by
// the list and asks for only one more page.
const fnBatch = fnPageSize * 2

// fnRandomLayout identifies the page-pair random queue format kept in a
// session origin. Older origins used per-track positions and cannot resume
// this layout safely.
const fnRandomLayout = 2

type fnQueue struct {
	list     *fnList
	shuffle  bool
	seed     int64
	basePage int // the loaded page used by the first random batch, or -1
	next     int // in order, the position asked for next; at random, tracks batched
	// pos is the position in the list of each song handed to the player,
	// to read on in order from the song playing when shuffle is switched off.
	pos map[int64]int
}

func newFnQueue(l *fnList, shuffle bool) *fnQueue {
	q := &fnQueue{list: l, shuffle: shuffle, basePage: -1, pos: map[int64]int{}}
	if shuffle {
		q.seed = rand.Int64()
	}
	return q
}

// fnOrigin is what the session keeps of a queue: which list, in which order,
// and how far. It is what origin returns.
type fnOrigin struct {
	T       string `json:"t"`
	Ref     string `json:"ref,omitempty"`
	Q       string `json:"q,omitempty"`
	Sort    string `json:"sort,omitempty"`
	Shuffle bool   `json:"shuffle,omitempty"`
	Seed    int64  `json:"seed,omitempty"`
	Base    int    `json:"base"`
	Layout  int    `json:"layout,omitempty"`
	Next    int    `json:"next"`
}

func (q *fnQueue) origin() string {
	s := q.list.spec
	layout := 0
	if q.shuffle {
		layout = fnRandomLayout
	}
	b, _ := json.Marshal(fnOrigin{T: s.typ, Ref: s.ref, Q: s.q, Sort: s.sort, Shuffle: q.shuffle, Seed: q.seed, Base: q.basePage, Layout: layout, Next: q.next})
	return string(b)
}

// fnFeed returns what gives the queue its songs, in order or at random.
func (a *app) fnFeed(q *fnQueue) func(got func([]netease.Track)) {
	if q.shuffle {
		return a.fnRandomFeed(q)
	}
	return a.fnOrderFeed(q)
}

// fnOrderFeed gives the songs of the list from position q.next on, a page at
// a time, as the queue asks: the page may be in already, read ahead.
func (a *app) fnOrderFeed(q *fnQueue) func(got func([]netease.Track)) {
	return func(got func([]netease.Track)) {
		p := q.list.pg
		if p.total >= 0 && q.next >= p.total {
			got(nil)
			return
		}
		page := q.next / p.size
		p.when(page, func(ok bool) {
			if !ok {
				a.toast = "加载更多歌曲失败"
				got(nil)
				return
			}
			end := min((page+1)*p.size, p.total)
			if q.next >= end {
				got(nil)
				return
			}
			items := append([]netease.Track(nil), p.items[q.next:end]...)
			for i, t := range items {
				q.pos[t.ID] = q.next + i
			}
			q.next = end
			got(items)
		})
	}
}

// randomPageOrder makes a stable random order of the physical pages. The
// first page is the one already shown by the list, when it is known; the
// following pages are all still random and each is used once.
func randomPageOrder(total int, seed int64, base int) []int {
	n := (total + fnPageSize - 1) / fnPageSize
	if n == 0 {
		return nil
	}
	pages := rand.New(rand.NewPCG(uint64(seed), 0x9e3779b97f4a7c15)).Perm(n)
	if base >= 0 && base < n {
		for i, page := range pages {
			if page == base {
				pages[0], pages[i] = pages[i], pages[0]
				break
			}
		}
	}
	return pages
}

// shuffleRandomBatch makes the order inside a pair of pages reproducible
// without keeping the whole queue in memory.
func shuffleRandomBatch(tracks []netease.Track, seed int64, batch int) {
	r := rand.New(rand.NewPCG(uint64(seed)^uint64(batch+1)*0x517cc1b727220a95, 0x243f6a8885a308d3^uint64(batch)))
	r.Shuffle(len(tracks), func(i, j int) { tracks[i], tracks[j] = tracks[j], tracks[i] })
}

// fnRandomFeed gives the next logical batch. Its pages are fetched together,
// then shuffled together before one queue update is posted.
func (a *app) fnRandomFeed(q *fnQueue) func(got func([]netease.Track)) {
	return func(got func([]netease.Track)) {
		p := q.list.pg
		if p.total < 0 {
			p.when(0, func(ok bool) {
				if !ok {
					a.toast = "加载更多歌曲失败"
					got(nil)
					return
				}
				a.fnRandomFeed(q)(got)
			})
			return
		}
		if q.next >= p.total {
			got(nil)
			return
		}
		pages := randomPageOrder(p.total, q.seed, q.basePage)
		group := q.next / fnBatch
		first := group * 2
		if first >= len(pages) {
			got(nil)
			return
		}
		want := pages[first:min(first+2, len(pages))]
		loadFnPages(p, want, func(items map[int][]netease.Track, ok bool) {
			if !ok {
				a.toast = "加载更多歌曲失败"
				got(nil)
				return
			}
			var out []netease.Track
			for _, page := range want {
				out = append(out, items[page]...)
			}
			shuffleRandomBatch(out, q.seed, group)
			for _, page := range want {
				for i, tr := range items[page] {
					q.pos[tr.ID] = page*p.size + i
				}
			}
			start := group * fnBatch
			q.next = min(start+fnBatch, p.total)
			a.noteHearts(out)
			if len(out) == 0 {
				a.toast = "加载更多歌曲失败"
			}
			got(out)
		})
	}
}

// loadFnPages waits for a set of physical pages. pager.when coalesces pages
// already loaded or already in flight, so a first random batch reuses the
// page shown by the list and asks for only one new page.
func loadFnPages(p *pager[netease.Track], pages []int, done func(map[int][]netease.Track, bool)) {
	items := make(map[int][]netease.Track, len(pages))
	if len(pages) == 0 {
		done(items, true)
		return
	}
	left := len(pages)
	ok := true
	for _, page := range pages {
		page := page
		p.when(page, func(got bool) {
			if got {
				start := page * p.size
				end := min(start+p.size, p.total)
				if start < end {
					items[page] = append([]netease.Track(nil), p.items[start:end]...)
				}
			} else {
				ok = false
			}
			left--
			if left == 0 {
				done(items, ok)
			}
		})
	}
}

// startQueue plays songs, which are the first of the queue q reads from.
func (a *app) startQueue(q *fnQueue, songs []netease.Track, at int) {
	a.fn.q = q
	a.pl.PlayList(songs, at, a.fnFeed(q), q.origin)
	a.pl.SetShuffle(q.shuffle)
}

// playFn plays the list from song i: the pages in around it and then the
// pages after, as the queue nears the end of what is in; or, with shuffle
// on, from that song at random.
func (a *app) playFn(l *fnList, i int) {
	if a.pl.Shuffle() {
		tr := l.pg.at(i)
		if tr == nil {
			return
		}
		q := newFnQueue(l, true)
		q.pos[tr.ID] = i
		a.startQueue(q, []netease.Track{*tr}, 0)
		return
	}
	items, at, end := l.pg.around(i)
	if items == nil {
		return
	}
	q := newFnQueue(l, false)
	q.next = end
	for k, t := range items {
		q.pos[t.ID] = end - len(items) + k
	}
	a.startQueue(q, items, at)
}

// shuffleFn plays the list in a random order. It reuses the first loaded page
// and reads one other page for the first logical batch.
func (a *app) shuffleFn(l *fnList) {
	if l.shuffle {
		return
	}
	l.shuffle = true
	q := newFnQueue(l, true)
	q.basePage = 0
	a.fnFeed(q)(func(first []netease.Track) {
		l.shuffle = false
		if len(first) > 0 {
			a.startQueue(q, first, 0)
		}
		a.redraw()
	})
}

// fnShuffleToggled is what the player asks when shuffle is switched on or
// off over a queue read from a list: the queue is made over from the song
// playing, at random or in order from it, and the player has nothing of its
// own to shuffle. It returns false for a queue that is not of the service's.
func (a *app) fnShuffleToggled(on bool) bool {
	old, cur := a.fn.q, a.pl.Current()
	if old == nil || cur == nil || a.fn.client == nil {
		return false
	}
	q := newFnQueue(old.list, on)
	q.pos = old.pos
	if !on {
		// In order from the song playing, if its place in the list is
		// known; if not, from the start of it.
		if i, ok := old.pos[cur.ID]; ok {
			q.next = i + 1
		}
	}
	a.fn.q = q
	a.pl.ListFrom(a.fnFeed(q), q.origin)
	return true
}

// restoreFnOrigin makes the queue of a restored session go on reading the
// list it was read from, where it had got to: the songs it held are
// what was loaded, and the rest of the list is read as it plays on.
func (a *app) restoreFnOrigin(raw string) {
	var o fnOrigin
	if json.Unmarshal([]byte(raw), &o) != nil || o.T == "" || a.fn.client == nil || a.pl.Current() == nil {
		return
	}
	l := a.newFnList(fnSpec{typ: o.T, ref: o.Ref, q: o.Q, sort: o.Sort})
	if l == nil {
		return
	}
	q := newFnQueue(l, o.Shuffle)
	if o.Shuffle && o.Layout != fnRandomLayout {
		// A session from the old per-track random queue has a different meaning
		// for Next. Restart its random order rather than replaying the wrong
		// page or duplicating songs.
		q.seed, q.basePage, q.next = rand.Int64(), -1, 0
	} else {
		q.seed, q.basePage, q.next = o.Seed, o.Base, o.Next
	}
	a.fn.q = q
	a.pl.AttachList(a.fnFeed(q), q.origin)
}

// Roaming is the service's own random play, which picks every song and is
// asked for the next with the roam ID of the one playing. It is a radio,
// as NetEase's private FM is.
const fnRoamRadio = "fn:roam"

// startRoam starts the roam, and plays its first song.
func (a *app) startRoam() {
	c := a.fn.client
	if c == nil {
		a.toast = "还没有连接飞牛音乐"
		return
	}
	if a.radioStarting == fnRoamRadio {
		return
	}
	a.radioStarting = fnRoamRadio
	go func() {
		tr, id, err := c.RoamStart()
		a.fnPost(func() {
			if a.radioStarting != fnRoamRadio {
				return
			}
			a.radioStarting = ""
			if err != nil {
				a.toast = "漫游暂时无法开启，请稍后再试"
				return
			}
			a.fn.roamID = id
			a.pl.PlayRadio(fnRoamRadio, []netease.Track{tr}, a.radioFeed(fnRoamRadio))
		})
	}()
}

// roamFeed gives the roam's next song, which the service picks after the one
// whose roam ID it is told. Restored from a session, it has no roam ID, and
// the service starts the roam over.
func (a *app) roamFeed(got func([]netease.Track)) {
	c, after := a.fn.client, a.fn.roamID
	if c == nil {
		got(nil)
		return
	}
	// What is queued: the roam may pick a song again, which the queue does
	// not take twice and would stop for, so the chain is asked on from it.
	queued := map[string]bool{}
	for _, t := range a.pl.Queue() {
		queued[t.Ref] = true
	}
	go func() {
		var tr netease.Track
		var id string
		var err error
		for range roamTries {
			if after == "" {
				tr, id, err = c.RoamStart()
			} else if tr, id, err = c.RoamNext(after); err != nil {
				// A chain the service no longer has (it ends in time): a new
				// one is started.
				tr, id, err = c.RoamStart()
			}
			if err != nil || !queued[tr.Ref] {
				break
			}
			after = id
		}
		a.fnPost(func() {
			if err != nil {
				got(nil)
				return
			}
			a.fn.roamID = id
			got([]netease.Track{tr})
		})
	}()
}

// roamTries is how many songs of a roam are asked for to find one that is not
// queued already, before the queue is given the last.
const roamTries = 6

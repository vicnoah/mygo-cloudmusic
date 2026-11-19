package qqmusic

import (
	"fmt"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
	backend "waygu-music/internal/plugins/qqmusic/internal/client"
)

// commentState is the comments of the song or episode Now Playing shows:
// the hot ones, and the latest, loaded a page at a time as they scroll.
type commentState struct {
	track   int64
	thread  string // whose comments they are, as NetEase names it
	hot     []pluginapi.Comment
	latest  []pluginapi.Comment
	total   int
	more    bool
	loaded  bool
	loading bool
	err     string
	scroll  ui.ScrollState
}

// commentPage is how many comments a page loads.
const commentPage = 30

// The current track has its own pagination and scroll position.
func (h *Home) commentsFor(tr *pluginapi.Track) *commentState {
	cs := &h.comments
	if cs.track != tr.ID {
		h.commentsGeneration++
		*cs = commentState{track: tr.ID, thread: strconv.FormatInt(tr.ID, 10)}
		h.loadComments()
	}
	return cs
}

var loadQQCommentPage = backend.ThreadComments

func (h *Home) loadComments() {
	ctx := h.scope
	cs := &h.comments
	if cs.loading {
		return
	}
	thread, offset, generation := cs.thread, len(cs.latest), h.commentsGeneration
	cs.loading, cs.err = true, ""
	go func() {
		raw, err := loadQQCommentPage(ctx, thread, offset, commentPage)
		h.postResult(ctx, func() {
			if h.commentsGeneration != generation {
				return
			}
			cs.loading = false
			if err != nil {
				cs.err = "评论加载失败"
				return
			}
			page := qqComments(raw)
			cs.loaded = true
			if offset == 0 {
				cs.hot = page.Hot
			}
			seen := make(map[int64]bool, len(cs.latest))
			for _, cm := range cs.latest {
				seen[cm.ID] = true
			}
			for _, cm := range page.Latest {
				if !seen[cm.ID] {
					cs.latest = append(cs.latest, cm)
				}
			}
			cs.total, cs.more = page.Total, page.More && len(page.Latest) > 0
		})
	}()
}

// commentsTab names the comments' pane, with how many there are.
func (h *Home) commentsTab() string {
	if n := h.commentCount(); n > 0 {
		return "评论 " + count(n)
	}
	return "评论"
}

// commentCount is the count the comments' tab shows: the last song's
// while the next one's loads, so that it rolls from one to the other.
func (h *Home) commentCount() int {
	switch cs := &h.comments; {
	case cs.loaded:
		return cs.total
	case cs.err != "":
		return 0
	}
	return h.tabCount.n
}

// rollState is a count as it rolls from one value to another.
type rollState struct {
	n    int
	up   bool       // whether it went up, last it changed
	exit *ui.Motion // where the characters shown go as it changes
}

// rollLine is how far a character rolls, about a line of the tab's text.
const rollLine = 16

// rollCount shows n as text, each character in a slot of its own, as
// the wheels of a counter: one that changes rolls out as the next rolls
// in, upward as n grows and downward as it shrinks, the others staying.
func rollCount(c *ui.Context, st *rollState, n int, text string, fg ui.Color) {
	if n != st.n {
		st.up = n > st.n
		dir := float32(1)
		if !st.up {
			dir = -1
		}
		if st.exit != nil {
			// The characters shown leave from this frame on, which first
			// knows the way.
			st.exit.Y = -dir * rollLine
		}
		st.n, st.exit = n, &ui.Motion{}
	}
	dir := float32(1)
	if !st.up {
		dir = -1
	}
	chars := []rune(text)
	// Clipped to its line, a character leaves out of sight.
	ui.Row(c).ClipY().Children(func() {
		for i, ch := range chars {
			// Slots go by their place from the right, as the wheels of a
			// counter do: the units are always the last.
			k := len(chars) - 1 - i
			roll := ui.Animation{Spring: ui.SpringOptions{Duration: 420 * time.Millisecond, Bounce: 0.15}, Delay: time.Duration(k) * 30 * time.Millisecond}
			slot := ui.Box(c.Key(k)).Transition(ui.ElementTransition{
				Animation: &roll, Size: true,
				Enter: &ui.Motion{Y: dir * rollLine, Collapse: true},
				Exit:  &ui.Motion{Collapse: true},
			})
			slot.Children(func() {
				ui.Text(c.Key(ch), string(ch)).FontSize(13).FontWeight(600).FontFeatures("tnum").TextColor(fg).
					Transition(ui.ElementTransition{Animation: &roll, Colors: true, Enter: &ui.Motion{Y: dir * rollLine}, Exit: st.exit})
			})
		}
	})
}

// count formats a count as people in China read it, to a tenth of a 万.
func count(n int) string {
	if n >= 1e4 && n < 1e6 {
		return fmt.Sprintf("%.1f万", float64(n)/1e4)
	}
	return plays(float64(n))
}

type commentKey struct {
	hot bool
	id  int64
}

// commentsPane lists the comments of tr: the hot ones, then the latest,
// which load on as they scroll into view.
func (h *Home) commentsPane(c *ui.Context, tr *pluginapi.Track) {
	cs := h.commentsFor(tr)
	if len(cs.hot) == 0 && len(cs.latest) == 0 {
		if cs.loading || !cs.loaded && cs.err == "" {
			ui.Column(c).Grow(1).Padding(4, 8, 0, 0).Gap(2).Children(func() {
				s := npShimmer(c)
				s.text(c, 13, 1.3).Width(56).Margin(18, 12, 6, 12)
				commentSkeleton(c, s, 6)
			})
			return
		}
		ui.Column(c).Grow(1).Center().Gap(14).Children(func() {
			switch {
			case cs.err != "":
				ui.Text(c, cs.err).FontSize(15).TextColor(white.Alpha(0.5))
				if pillButton(c.Key("retry"), iconComment, "重试", white.Alpha(0.14), white).Clicked() {
					h.loadComments()
				}
			default:
				ui.Icon(c, iconComment).Size(40, 40).TextColor(white.Alpha(0.3))
				ui.Text(c, "还没有评论，快来抢沙发").FontSize(17).FontWeight(600).TextColor(white.Alpha(0.45))
			}
		})
		return
	}
	now := c.Now()
	scroll := ui.Scroll(c.Key("comments")).Grow(1).TrackScroll(&cs.scroll).Padding(4, 8, 48, 0)
	var end ui.Rect
	scroll.Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			if len(cs.hot) > 0 {
				commentHeading(c, "精彩评论")
				for i := range cs.hot {
					h.commentRow(c.Key(commentKey{true, cs.hot[i].ID}), &cs.hot[i], now)
				}
			}
			commentHeading(c, "最新评论 "+count(cs.total))
			for i := range cs.latest {
				h.commentRow(c.Key(commentKey{false, cs.latest[i].ID}), &cs.latest[i], now)
			}
			if cs.loading {
				commentSkeleton(c.Key("more"), npShimmer(c), 2)
			}
			ui.Row(c.Key("foot")).Height(56).Center().Children(func() {
				switch {
				case cs.loading:
				case cs.err != "":
					retry := ui.ButtonBase(c.Key("retry")).Padding(6, 14).Radius(999).Background(white.Alpha(0.1)).Label("重试")
					retry.Children(func() {
						ui.Text(c, "加载失败，点按重试").FontSize(12.5).TextColor(white.Alpha(0.7))
					})
					if retry.Clicked() {
						h.loadComments()
					}
				case !cs.more:
					ui.Text(c, "没有更多评论了").FontSize(12).TextColor(white.Alpha(0.35))
				}
			})
			end = ui.Box(c.Key("end")).Height(0).Bounds()
		})
	})
	// The next page, once the foot is within a screen of coming into view.
	if box := scroll.Bounds(); cs.more && !cs.loading && cs.err == "" && end.W > 0 && end.Y < box.Y+box.H*2 {
		h.loadComments()
	}
}

func commentHeading(c *ui.Context, title string) {
	ui.Text(c, title).FontSize(13).FontWeight(700).TextColor(white.Alpha(0.55)).LetterSpacing(0.2).Padding(18, 12, 6, 12)
}

// commentRow is a comment: who wrote it, what, and what it replies to,
// when, from where, and how many liked it, with a thumb to like it too.
func (h *Home) commentRow(c *ui.Context, cm *pluginapi.Comment, now time.Time) {
	ui.Row(c).Padding(12).Gap(12).Radius(12).AlignItems(ui.Start).Children(func() {
		h.avatar(c, cm.User.AvatarURL, 34, white.Alpha(0.1))
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(6).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, cm.User.Nickname).FontSize(13).FontWeight(600).TextColor(white.Alpha(0.6)).SingleLine().Shrink(1).MinWidth(0)
				ui.Spacer(c)
				if cm.LikedCount > 0 {
					ui.Text(c, count(cm.LikedCount)+" 赞").FontSize(12).TextColor(white.Alpha(0.6))
				}
			})
			ui.Text(c, cm.Content).FontSize(15).LineHeight(1.55).TextColor(white.Alpha(0.92))
			if len(cm.BeReplied) > 0 {
				r := cm.BeReplied[0]
				ui.Box(c).Padding(8, 12).Radius(10).Background(white.Alpha(0.07)).Children(func() {
					ui.RichText(c,
						ui.Span{Text: "@" + r.User.Nickname + "：", Color: white.Alpha(0.75), Weight: 600},
						ui.Span{Text: r.Content, Color: white.Alpha(0.6)},
					).FontSize(13).LineHeight(1.5).MaxLines(3)
				})
			}
			meta := commentTime(time.UnixMilli(cm.Time), now)
			if loc := cm.IPLocation.Location; loc != "" {
				meta += " · " + loc
			}
			ui.Text(c, meta).FontSize(11.5).TextColor(white.Alpha(0.38))
		})
	})
}

// avatar shows a listener's picture, round, over empty while it loads.
func (h *Home) avatar(c *ui.Context, url string, size float32, empty ui.Color) {
	bmp := bitmap(h.host.DecodedArtwork(url, int(size*3), false))
	box := ui.Box(c).Size(size, size).Radius(size / 2).Shrink(0)
	in := artIn(box, bmp)
	if in < 1 {
		box.Background(empty)
	}
	box.Children(func() { artImage(c, bmp, size/2, in) })
}

// commentTime says when a comment was written, as NetEase does: in
// minutes or hours while recent, then by the day.
func commentTime(t, now time.Time) string {
	d := now.Sub(t)
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d分钟前", int(d/time.Minute))
	case y1 == y2 && m1 == m2 && d1 == d2:
		return t.Format("15:04")
	case now.AddDate(0, 0, -1).Format("20060102") == t.Format("20060102"):
		return "昨天 " + t.Format("15:04")
	case y1 == y2:
		return fmt.Sprintf("%d月%d日", m1, d1)
	default:
		return fmt.Sprintf("%d年%d月%d日", y1, m1, d1)
	}
}

// commentSkeleton is n of commentRow's comments.
func commentSkeleton(c *ui.Context, s shimmer, n int) {
	for i := range n {
		ui.Row(c.Key(i)).Padding(12).Gap(12).AlignItems(ui.Start).Children(func() {
			s.bone(c, 17).Size(34, 34)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(6).Children(func() {
				s.text(c, 13, 1.3).Width(60 + boneWidth(i))
				ui.Column(c).Children(func() {
					s.text(c, 15, 1.55).WidthPercent(94)
					if i%3 != 2 {
						s.text(c, 15, 1.55).WidthPercent(boneWidth(i + 2))
					}
				})
				s.text(c, 11.5, 1.3).Width(96)
			})
		})
	}
}

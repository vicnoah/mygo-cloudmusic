package songloft

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/paging"
)

func (h *Home) renderTracks(c *ui.Context, l *paging.Pager[pluginapi.Track], view TrackView) {
	if l == nil {
		h.centered(c, "找不到这个列表", nil)
		return
	}
	if l.FailedAt(0) {
		h.centered(c, "加载失败，请稍后重试", func() { l.Retry() })
		return
	}
	// At is intentionally called while the list is built. It starts the first
	// page and leaves the pager state independent from UI rebuilds.
	if l.Total() < 0 {
		// Start the hero request while the first page is still a skeleton. The
		// first real frame should never be the one that starts downloading it.
		if view.Cover != "" {
			h.host.Artwork(view.Cover, 636)
		}
		l.At(0)
		h.platformTrackSkeleton(c, view)
		return
	}
	total := l.Len()
	if total == 0 {
		h.centered(c, "这里还没有歌曲", nil)
		return
	}
	if !h.platformHeroReady(c, view.Cover) {
		h.platformTrackSkeleton(c, view)
		return
	}
	h.prefetchTrackArtwork(l, total, c)
	get := l.At
	play := view.OnPlay
	if play == nil {
		play = func(int) {}
	}

	h.trackRowsProviderOptions(c, total, get, play, true, trackRowsOptions{
		Menu:      view.Menu,
		Liked:     view.Liked,
		Favorite:  view.Favorite,
		ShowAlbum: view.ShowAlbum,
	}, func(c *ui.Context) {
		if view.Header != nil {
			view.Header(c)
		} else {
			h.heading(c, view.Title, view.Sub)
		}
	})
}

func (h *Home) platformHeroReady(c *ui.Context, cover string) bool {
	if cover == "" {
		return true
	}
	if h.platformHeroSince.IsZero() {
		h.platformHeroSince = c.Now()
	}
	if _, ready := h.host.Artwork(cover, 636); ready {
		return true
	}
	if c.Now().Sub(h.platformHeroSince) >= 450*time.Millisecond {
		return true
	}
	c.After(40 * time.Millisecond)
	return false
}

func (h *Home) prefetchTrackArtwork(l *paging.Pager[pluginapi.Track], total int, c *ui.Context) {
	if l == nil {
		return
	}
	_, height := c.Size()
	count := max(12, int(height/54)+6)
	count = min(count, total)
	urls := make([]string, 0, count)
	for i := 0; i < count; i++ {
		if tr := l.At(i); tr != nil {
			urls = append(urls, tr.Album.PicURL)
		}
	}
	for _, url := range urls {
		h.host.Artwork(url, 96)
	}
}

func (h *Home) trackRow(c *ui.Context, i int, tr *pluginapi.Track, view TrackView) {
	w, _ := c.Size()
	wide := w-sidebarWidth > 820
	if tr == nil {
		h.pendingTrackRow(c, i, wide, true, view.ShowAlbum)
		return
	}
	play := view.OnPlay
	if play == nil {
		play = func(int) {}
	}
	h.providerTrackRow(c, i, tr, h.currentTrack(), 0, w-sidebarWidth > 820, true, trackRowsOptions{
		Menu: view.Menu, Liked: view.Liked, Favorite: view.Favorite, ShowAlbum: view.ShowAlbum,
	}, play)
}

func (h *Home) pageList(c *ui.Context, key string, state *ui.ListState, rows int, row func(int)) {
	top, bottom := h.insets(c)
	h.underTop, h.underBottom = true, true
	ui.List(c.Key(key), state, rows, func(i int) {
		ui.Column(c).Children(func() {
			if i == 0 {
				h.startMark(c)
			}
			row(i)
			if i == rows-1 {
				h.endMark(c)
			}
		})
	}).Grow(1).Padding(top+pageGap, pagePad-12, bottom+32, pagePad-12).ScrollbarInsets(top, 0, bottom, 0)
}

func (h *Home) trackRowsProviderOptions(c *ui.Context, total int, get func(int) *pluginapi.Track, play func(int), covers bool, options trackRowsOptions, header func(c *ui.Context), footer ...func(c *ui.Context)) {
	cur := h.currentTrack()
	// A song's menu can take it out of the playlist it is listed in.
	var pid int64
	if h.route.Page == PagePlaylist {
		pid = h.route.ID
	}
	w, _ := c.Size()
	// An album's page has no column of the album its songs are on.
	wide := w-sidebarWidth > 820
	top, bottom := h.insets(c)
	// Rows out of view are not built, their marks with them: a list
	// scrolled away from its start or end has content under the bars.
	h.underTop, h.underBottom = true, true
	rows := total + 1
	ui.List(c.Key(h.route), &h.trackList, rows, func(i int) {
		if i == 0 {
			ui.Column(c).Children(func() {
				h.startMark(c)
				header(c)
			})
			return
		}
		if i == rows-1 {
			defer h.endMark(c)
		}
		i--
		tr := get(i)
		if tr == nil {
			h.pendingTrackRow(c, i, wide, covers, options.ShowAlbum)
			return
		}
		h.providerTrackRow(c, i, tr, cur, pid, wide, covers, options, play)
	}).Grow(1).Padding(top+pageGap, pagePad-12, bottom+32, pagePad-12).ScrollbarInsets(top, 0, bottom, 0)
}

func (h *Home) providerTrackRow(c *ui.Context, i int, tr *pluginapi.Track, cur *pluginapi.Track, pid int64, wide, covers bool, options trackRowsOptions, play func(int)) {
	p := paletteFor(c.Theme())
	playing := cur != nil && cur.ID == tr.ID
	row := ui.ButtonBase(c.Key(tr.ID)).Height(54).PaddingX(12).Gap(14).Radius(10).AlignItems(ui.Center).FocusRing(false).Label(tr.Name)
	if options.Menu != nil {
		row.ContextMenu(func(m *ui.Menu) { options.Menu(m, tr, pid) })
	}
	hovered := row.Hovered()
	bg := ui.Transparent
	switch {
	case playing:
		bg = p.accent.Alpha(0.09)
	case hovered:
		bg = p.hover
	}
	row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 100 * time.Millisecond}))
	if !tr.Playable() {
		row.Opacity(0.42)
	}
	row.Children(func() {
		ui.Box(c).Width(26).Center().Children(func() {
			switch {
			case playing:
				h.nowPlayingBars(c)
			case hovered && tr.Playable():
				ui.Icon(c, iconPlaySolid).Size(13, 13).TextColor(p.text)
			default:
				ui.Textf(c, "%d", i+1).FontSize(13).FontFeatures("tnum").TextColor(p.faint)
			}
		})
		if covers {
			h.cover(c, tr.Album.PicURL, 96, 38, 6)
		}
		ui.Column(c).Grow(3).Basis(0).MinWidth(0).Gap(3).Children(func() {
			name := p.text
			if playing {
				name = p.accent
			}
			spans := []ui.Span{{Text: tr.Name, Weight: 500, Color: name}}
			if len(tr.Alias) > 0 {
				spans = append(spans, ui.Span{Text: "（" + tr.Alias[0] + "）", Color: p.faint})
			}
			ui.RichText(c, spans...).FontSize(14).SingleLine()
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if !tr.Playable() {
					ui.Text(c, "VIP").FontSize(9).FontWeight(800).TextColor(p.accent).Padding(0, 4).Radius(3).Border(1, p.accent.Alpha(0.6))
				}
				h.artistLinks(c, tr, p.muted, p.text).FontSize(12.5).SingleLine().Shrink(1).MinWidth(0)
				if tr.Reason != "" {
					ui.Text(c, tr.Reason).FontSize(10.5).FontWeight(600).TextColor(p.accent).Padding(1, 6).Radius(999).Background(p.accent.Alpha(0.1)).SingleLine()
				}
			})
		})
		if wide && options.ShowAlbum {
			album := ui.RichText(c).Grow(2).Basis(0).MinWidth(0).FontSize(12.5).TextColor(p.muted).SingleLine()
			album.Children(func() {
				if tr.Album.ID == 0 {
					ui.Text(c, tr.Album.Name)
				} else {
					h.link(c.Key("album"), tr.Album.Name, p.muted, p.text, Route{Page: "album", Kind: "album", ID: tr.Album.ID})
				}
			})
		}
		// A liked song's heart shows; the others' as the pointer is on them.
		ui.Box(c).Size(32, 32).Center().Children(func() {
			if options.Liked != nil {
				liked := tr.Fav || options.Liked(tr.ID)
				if liked || hovered {
					h.sharedLikeButton(c.Key("platform-like"), tr, liked, p.muted, options.Favorite)
				}
			}
		})
		ui.Text(c, clock(tr.Length())).FontSize(12.5).FontFeatures("tnum").TextColor(p.faint).Width(44).TextAlign(ui.End)
	})
	if row.Clicked() && tr.Playable() {
		play(i)
	}
}

func (h *Home) pendingTrackRow(c *ui.Context, i int, wide, covers, showAlbum bool) {
	h.platformTrackRowSkeleton(c.Key(fmt.Sprintf("pending-%d", i)), pageShimmer(c), i, wide, covers, showAlbum)
}

func (h *Home) sharedLikeButton(c *ui.Context, tr *pluginapi.Track, liked bool, color ui.Color, favorite func(pluginapi.Track, bool)) ui.Element {
	icon, label := iconHeart, "喜欢"
	if liked {
		icon, label, color = iconHeartFill, "取消喜欢", paletteFor(c.Theme()).like
	}
	b := iconButton(c, icon, 15, color, label)
	if b.Clicked() && favorite != nil {
		favorite(*tr, !liked)
	}
	return b
}

type trackRowsOptions struct {
	Menu      func(*ui.Menu, *pluginapi.Track, int64)
	Liked     func(int64) bool
	Favorite  func(pluginapi.Track, bool)
	ShowAlbum bool
}

func (h *Home) currentTrack() *pluginapi.Track {
	if player := h.host.Player(); player != nil {
		return player.Current()
	}
	return nil
}

func (h *Home) artistLinks(c *ui.Context, tr *pluginapi.Track, color, hover ui.Color, album ...bool) ui.Element {
	return ui.RichText(c).TextColor(color).Children(func() {
		for i, artist := range tr.Artists {
			if i > 0 {
				ui.Text(c, ", ")
			}
			if artist.ID == 0 {
				ui.Text(c, artist.Name)
			} else {
				h.link(c.Key(i), artist.Name, color, hover, Route{Page: "artist", Kind: "artist", ID: artist.ID})
			}
		}
		if len(album) > 0 && album[0] && tr.Album.Name != "" {
			ui.Text(c, " — ")
			if tr.Album.ID == 0 {
				ui.Text(c, tr.Album.Name)
			} else {
				h.link(c.Key("album"), tr.Album.Name, color, hover, Route{Page: "album", Kind: "album", ID: tr.Album.ID})
			}
		}
	})
}

func (h *Home) link(c *ui.Context, text string, color, hover ui.Color, route Route) {
	link := ui.Link(c, text, "").TextColor(color)
	if link.Hovered() {
		link.TextColor(hover)
	}
	link.OnClick(func() {
		h.navigate(route)
		h.host.ShowMain()
	})
}

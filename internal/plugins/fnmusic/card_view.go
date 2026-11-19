package fnmusic

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"waygu-music/internal/pluginapi"
	"waygu-music/internal/pluginapi/paging"
)

func (h *Home) renderCards(c *ui.Context, p *paging.Pager[pluginapi.Playlist], view CardView) {
	if p == nil {
		h.centered(c, "没有找到相关内容", nil)
		return
	}
	p.At(0)
	header := func(c *ui.Context) {
		if view.Header != nil {
			view.Header(c)
			return
		}
		ui.Column(c).Gap(4).Margin(0, 0, 18, 0).Children(func() {
			ui.Text(c, view.Title).FontSize(30).FontWeight(700).LetterSpacing(-0.6)
			if view.Sub != "" {
				ui.Text(c, view.Sub).FontSize(12).FontWeight(600).TextColor(h.pal.muted).LetterSpacing(0.4)
			}
			if view.Count > 0 {
				ui.Text(c, fmt.Sprintf("共 %d 个", view.Count)).FontSize(13).TextColor(h.pal.muted)
			}
		})
	}
	top, bottom := h.insets(c)
	if p.FailedAt(0) {
		ui.Column(c).Grow(1).Padding(top+pageGap, pagePad, bottom+48, pagePad).Children(func() {
			header(c)
			ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
				ui.Text(c, "加载失败，请稍后重试").TextColor(h.pal.muted).FontSize(13)
				if ui.Button(c, "重试").Clicked() {
					p.Retry()
				}
			})
		})
		return
	}
	if p.Total() < 0 {
		h.platformCardsSkeleton(c, view)
		return
	}
	h.prefetchCardArtwork(p, c)
	pages, size := p.PageCount(), p.PageSize()
	if pages == 0 {
		ui.Column(c).Grow(1).Padding(top+pageGap, pagePad, bottom+48, pagePad).Children(func() {
			header(c)
			ui.Column(c).Grow(1).Center().Children(func() {
				ui.Text(c, "没有找到相关内容").TextColor(h.pal.muted).FontSize(13)
			})
		})
		return
	}
	h.underTop, h.underBottom = true, true
	renderArt := view.Art
	if renderArt == nil && view.Placeholder != nil {
		renderArt = h.iconCardArt(view.Placeholder)
	}
	r := h.route
	routeKey := fmt.Sprintf("platform-cards-%s-%d-%s", r.Page, r.ID, r.Query)
	rows := pages + 1
	ui.List(c.Key(routeKey), &h.platformCardList, rows, func(row int) {
		if row == 0 {
			ui.Column(c).Children(func() {
				h.startMark(c)
				header(c)
			})
			return
		}
		page := row - 1
		start := page * size
		p.At(start)
		cols, card := h.cardWidth(c)
		ui.Grid(c).Columns(cols).GapX(20).GapY(28).Children(func() {
			for i := start; i < min(start+size, p.Len()); i++ {
				pl := p.Peek(i)
				if pl == nil {
					ui.Box(c.Key(fmt.Sprintf("platform-card-pending-%d", i))).Width(card).Children(func() {
						h.platformCardSkeleton(c, pageShimmer(c), i, view.Meta)
					})
					continue
				}
				ui.Box(c.Key(fmt.Sprintf("platform-card-wrap-%d", pl.ID))).Width(card).Children(func() {
					play := func() {
						if view.OnPlay != nil {
							view.OnPlay(pl.ID)
						}
					}
					key := c.Key(fmt.Sprintf("platform-card-%d", pl.ID))
					var open bool
					switch {
					case view.Round:
						open = h.roundCard(key, pl, card)
					default:
						if pl.Cover() == "" && renderArt != nil {
							card := *pl
							open = h.drawnCard(key, pl.Name, pl.UpdateFreq, func(p *ui.Painter, r ui.Rect, radius float32) {
								renderArt(p, r, &card, radius)
							}, play)
						} else {
							open = h.coverCard(key, pl.Name, pl.Cover(), pl.PlayCount, pl.UpdateFreq, play)
						}
					}
					if open && view.OnOpen != nil {
						view.OnOpen(pl.ID)
					}
				})
			}
		})
		if row == pages {
			defer h.endMark(c)
		}
	}).Grow(1).Padding(top+pageGap, pagePad, bottom+48, pagePad).ScrollbarInsets(top, 0, bottom, 0)
}

// roundCard is an artist's photo, round over their name, opening their page.
func (h *Home) roundCard(c *ui.Context, pl *pluginapi.Playlist, size float32) bool {
	p := h.pal
	b := ui.ButtonBase(c).Column().Gap(10).AlignItems(ui.Stretch).FocusRing(false).Label(pl.Name)
	hovered := b.Hovered()
	b.Children(func() {
		art := ui.Box(c).AspectRatio(1).Radius(size / 2)
		if hovered {
			art.AnimatePose(ui.Pose{Y: -3}, ui.Snappy)
			art.Shadow(0, 12, 28, -6, p.cardShade)
		} else {
			art.Shadow(0, 2, 8, -2, p.cardShade)
		}
		art.Children(func() { h.coverAtSize(c, pl.Cover(), size, size/2) })
		ui.Text(c, pl.Name).FontSize(13).FontWeight(500).SingleLine().AlignSelf(ui.Center).TextAlign(ui.Center)
	})
	return b.Clicked()
}

// drawnCard is coverCard for a card whose cover is drawn, not loaded: it
// lifts, shades and shows its play button the same.
func (h *Home) drawnCard(c *ui.Context, name, sub string, art func(p *ui.Painter, r ui.Rect, radius float32), play func()) bool {
	p := h.pal
	card := ui.ButtonBase(c).Column().Gap(10).AlignItems(ui.Stretch).Justify(ui.Start).FocusRing(false).Label(name)
	hovered := card.Hovered()
	card.Children(func() {
		box := ui.Box(c).AspectRatio(1).Radius(10)
		lift := ui.Pose{}
		if hovered {
			lift = ui.Pose{Y: -3}
			box.Shadow(0, 12, 28, -6, p.cardShade)
		} else {
			box.Shadow(0, 2, 8, -2, p.cardShade)
		}
		box.AnimatePose(lift, ui.Snappy)
		box.Children(func() {
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Draw(func(pt *ui.Painter, r ui.Rect) {
				art(pt, r, 10)
			})
			h.coverPlayButton(c, hovered, play)
		})
		ui.Text(c, name).FontSize(13).FontWeight(500).MaxLines(2).LineHeight(1.35)
		if sub != "" {
			ui.Text(c, sub).FontSize(12).TextColor(p.muted).SingleLine().Margin(-6, 0, 0, 0)
		}
	})
	return card.Clicked()
}

func (h *Home) iconCardArt(icon *ui.SVG) func(p *ui.Painter, r ui.Rect, card *pluginapi.Playlist, radius float32) {
	return func(p *ui.Painter, r ui.Rect, _ *pluginapi.Playlist, radius float32) {
		p.Fill(r, h.pal.hover, radius)
		size := min(r.W, r.H) * 0.3
		p.Icon(icon, ui.Rect{
			X: r.X + (r.W-size)/2,
			Y: r.Y + (r.H-size)/2,
			W: size,
			H: size,
		}, h.pal.faint)
	}
}

func searchPageLoading(p *paging.Pager[pluginapi.Playlist]) bool {
	return p != nil && p.Total() < 0 && !p.FailedAt(0)
}

func searchPageEmpty(p *paging.Pager[pluginapi.Playlist]) bool {
	return p == nil || p.FailedAt(0) || p.Total() >= 0 && p.Len() == 0
}

func (h *Home) renderSearch(c *ui.Context, s SearchResults, view SearchView) {
	if s.Songs == nil {
		h.centered(c, "搜索尚未开始", nil)
		return
	}
	// Search keeps the song list as the primary virtual list. Album and artist
	// cards are rendered above it from their first loaded page, so the page
	// remains useful even when a result has thousands of songs.
	s.Songs.At(0)
	albums, artists := s.Albums, s.Artists
	if albums != nil {
		albums.At(0)
	}
	if artists != nil {
		artists.At(0)
	}
	if s.Songs.FailedAt(0) {
		h.centered(c, "加载失败，请稍后重试", func() { s.Songs.Retry() })
		return
	}
	songsLoading := s.Songs.Total() < 0
	if songsLoading || searchPageLoading(albums) || searchPageLoading(artists) {
		h.platformSearchSkeleton(c)
		return
	}
	if s.Songs.Len() == 0 && searchPageEmpty(albums) && searchPageEmpty(artists) {
		h.centered(c, "没有找到相关内容", nil)
		return
	}
	h.trackRowsProviderOptions(c, s.Songs.Len(), s.Songs.At, func(i int) {
		if view.Play != nil {
			view.Play(i)
		}
	}, true, trackRowsOptions{
		Menu:      view.TrackMenu,
		Liked:     view.Liked,
		Favorite:  view.Favorite,
		ShowAlbum: true,
	}, func(c *ui.Context) {
		h.heading(c, "“"+s.Query+"”", "搜索")
		if albums != nil && albums.Len() > 0 {
			h.searchCardStrip(c, "专辑", albums, view.OpenList, view.PlayCard, func() {
				if view.OpenCards != nil {
					view.OpenCards("albums")
				}
			})
		}
		if artists != nil && artists.Len() > 0 {
			h.searchCardStrip(c, "歌手", artists, view.OpenList, view.PlayCard, func() {
				if view.OpenCards != nil {
					view.OpenCards("artists")
				}
			})
		}
		if s.Songs.Len() > 0 {
			h.section(c, "歌曲")
		}
	})
}

func (h *Home) searchCardStrip(c *ui.Context, title string, p *paging.Pager[pluginapi.Playlist], open func(int64), play func(int64), all func()) {
	cols := h.columns(c, 168)
	ui.Row(c).AlignItems(ui.Center).Margin(34, 0, 14, 0).Children(func() {
		ui.Text(c, title).FontSize(19).FontWeight(700).LetterSpacing(-0.3).Grow(1)
		if p.Total() > cols {
			b := ui.ButtonBase(c.Key("search-all-" + title)).PaddingX(8).Height(26).Radius(7).AlignItems(ui.Center).FocusRing(false).Label("查看全部" + title)
			b.Background(b.AnimateColor("bg", map[bool]ui.Color{true: paletteFor(c.Theme()).hover, false: ui.Transparent}[b.Hovered()], ui.Animation{Duration: 120 * time.Millisecond}))
			b.Children(func() {
				ui.Text(c, fmt.Sprintf("全部 %d", p.Total())).FontSize(12.5).FontWeight(500).TextColor(paletteFor(c.Theme()).muted)
			})
			if b.Clicked() && all != nil {
				all()
			}
		}
	})
	ui.Row(c).Gap(20).Children(func() {
		for i := 0; i < min(cols, p.Len()); i++ {
			if pl := p.Peek(i); pl != nil {
				ui.Box(c.Key(fmt.Sprintf("search-%s-%d", title, pl.ID))).Width(168).Children(func() {
					clicked := h.coverCard(c, pl.Name, pl.Cover(), pl.PlayCount, pl.UpdateFreq, func() {
						if play != nil {
							play(pl.ID)
						}
					})
					if clicked && open != nil {
						open(pl.ID)
					}
				})
			}
		}
	})
}

func (h *Home) prefetchCardArtwork(p *paging.Pager[pluginapi.Playlist], c *ui.Context) {
	if p == nil {
		return
	}
	cols := h.columns(c, 168)
	count := min(p.Len(), cols*2)
	urls := make([]string, 0, count)
	for i := 0; i < count; i++ {
		if pl := p.Peek(i); pl != nil {
			urls = append(urls, pl.Cover())
		}
	}
	for _, url := range urls {
		h.host.Artwork(url, 400)
	}
}

func (h *Home) coverCard(c *ui.Context, name, cover string, playCount float64, sub string, play func()) bool {
	p := paletteFor(c.Theme())
	// From the top: a grid stretches the cards of a row as tall as the
	// tallest, whose name or line under it takes two lines.
	card := ui.ButtonBase(c).Column().Gap(10).AlignItems(ui.Stretch).Justify(ui.Start).FocusRing(false).Label(name)
	hovered := card.Hovered()
	card.Children(func() {
		art := ui.Box(c).AspectRatio(1).Radius(10)
		lift := ui.Pose{}
		if hovered {
			lift = ui.Pose{Y: -3}
			art.Shadow(0, 12, 28, -6, p.cardShade)
		} else {
			art.Shadow(0, 2, 8, -2, p.cardShade)
		}
		art.AnimatePose(lift, ui.Snappy)
		art.Children(func() {
			h.coverFill(c, cover, 400, 10)
			if playCount > 0 {
				ui.Row(c).Absolute().Top(8).Right(8).Padding(2, 7).Radius(999).Gap(3).AlignItems(ui.Center).
					Background(ui.RGBA(0, 0, 0, 0.35)).TextColor(ui.RGB(255, 255, 255)).Children(func() {
					ui.Icon(c, iconHeadset).Size(10, 10)
					ui.Text(c, plays(playCount)).FontSize(10.5).FontWeight(600)
				})
			}
			h.coverPlayButton(c, hovered, play)
		})
		ui.Text(c, name).FontSize(13).FontWeight(500).MaxLines(2).LineHeight(1.35)
		if sub != "" {
			ui.Text(c, sub).FontSize(12).TextColor(p.muted).SingleLine().Margin(-6, 0, 0, 0)
		}
	})
	return card.Clicked()
}

func (h *Home) coverFill(c *ui.Context, url string, px int, radius float32) {
	bmp, _ := h.host.Artwork(url, px)
	box := ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(radius)
	in := artIn(box, bmp)
	if in < 1 {
		box.Background(paletteFor(c.Theme()).hover)
	}
	box.Children(func() { artImage(c, bmp, radius, in) })
}

func (h *Home) coverPlayButton(c *ui.Context, shown bool, play func()) {
	b := ui.ButtonBase(c.Key("play")).Absolute().Bottom(10).Right(10).Size(38, 38).Radius(999).Center().
		Background(paletteFor(c.Theme()).accent).TextColor(ui.RGB(255, 255, 255)).Label("播放").FocusRing(false).
		Shadow(0, 4, 12, 0, ui.RGBA(0, 0, 0, 0.3))
	pose := ui.Pose{Fade: 1, Scale: 0.8, Y: 6}
	if shown || b.Hovered() {
		pose = ui.Pose{Scale: 1}
		if b.Hovered() {
			pose.Scale = 1.08
		}
	}
	b.AnimatePose(pose, ui.Snappy)
	b.Children(func() { ui.Icon(c, iconPlaySolid).Size(15, 15).Margin(0, 0, 0, 2) })
	if b.Clicked() {
		play()
	}
}

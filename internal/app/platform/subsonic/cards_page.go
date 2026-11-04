package subsonic

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform/paging"
	backend "netease-music-mygo/internal/platform/subsonic"
)

func (h *Home) cardsHeader(c *ui.Context, title string, total int) {
	t := c.Theme()
	ui.Column(c).Gap(4).Margin(0, 0, 18, 0).Children(func() {
		ui.Text(c, title).FontSize(30).FontWeight(700).LetterSpacing(-0.6)
		if total >= 0 {
			ui.Text(c, fmt.Sprintf("共 %d 个", total)).FontSize(13).TextColor(t.TextMuted)
		}
	})
}

func (h *Home) cardsHeaderSkeleton(c *ui.Context) {
	ui.Column(c).Gap(4).Margin(0, 0, 18, 0).Children(func() {
		h.host.TextBone(c, 30, 1.2).WidthPercent(32)
		h.host.TextBone(c, 13, 1.3).Width(52)
	})
}

func (h *Home) findCardMeta(id int64) (string, string) {
	if h == nil || h.state == nil {
		return "album", ""
	}
	if meta, ok := backend.LookupMeta(id); ok && meta.Ref != "" {
		return meta.Kind, meta.Ref
	}
	catalog := h.state.Catalog()
	if meta, ok := catalog.Meta[id]; ok && meta.Ref != "" {
		return meta.Kind, meta.Ref
	}
	// Fallback check through loaded card pagers
	for _, p := range []*paging.Pager[netease.Playlist]{catalog.SearchAlbums, catalog.SearchArtists, catalog.Albums, catalog.Artists} {
		if p != nil {
			for i := 0; i < p.Len(); i++ {
				if pl := p.Peek(i); pl != nil && pl.ID == id {
					catalog.Meta[id] = *pl
					return pl.Kind, pl.Ref
				}
			}
		}
	}
	return "album", ""
}

func (h *Home) openCard(id int64) {
	if h == nil || h.host == nil || h.state == nil {
		return
	}
	kind, ref := h.findCardMeta(id)
	h.host.Navigate(Route{Page: PagePlaylist, ID: id, Kind: kind, Query: ref})
}

func (h *Home) playCard(id int64) {
	if h == nil || h.state == nil || h.host == nil {
		return
	}
	catalog := h.state.Catalog()
	kind, ref := h.findCardMeta(id)
	l := catalog.ListForRoute(id, kind, ref)
	if l == nil || l.Pages == nil {
		return
	}
	l.Pages.When(0, func(ok bool) {
		if ok {
			PlayList(h.host.Player(), l, 0)
		}
	})
}

func (h *Home) cardsPage(c *ui.Context) {
	catalog := h.state.Catalog()
	switch h.route.Page {
	case PageSearch:
		var songs *appplatform.TrackList
		if catalog.SearchList != nil {
			songs = catalog.SearchList.View()
		}
		h.host.RenderSearch(c, appplatform.SearchResults{
			Query:   h.route.Query,
			Songs:   songs,
			Albums:  catalog.SearchAlbums,
			Artists: catalog.SearchArtists,
		}, SearchView{
			Play: func(i int) {
				if catalog.SearchList != nil {
					PlayList(h.host.Player(), catalog.SearchList, i)
				}
			},
			PlayCard:  h.playCard,
			TrackMenu: h.trackMenu,
			Liked:     h.state.Liked,
			Favorite:  h.RequestFavorite,
			OpenList:  h.openCard,
			OpenCards: func(kind string) {
				page := PageSearchArtists
				if kind == "albums" {
					page = PageSearchAlbums
				}
				h.host.Navigate(Route{Page: page, Query: h.route.Query})
			},
		})

	case PageSearchAlbums:
		cards := catalog.SearchAlbums
		h.host.RenderCards(c, cards, CardView{
			Title:          "专辑 · “" + h.route.Query + "”",
			Count:          cardsTotal(cards),
			OnOpen:         h.openCard,
			OnPlay:         h.playCard,
			Header:         func(c *ui.Context) { h.cardsHeader(c, "专辑 · “"+h.route.Query+"”", cardsTotal(cards)) },
			HeaderSkeleton: h.cardsHeaderSkeleton,
		})

	case PageSearchArtists:
		cards := catalog.SearchArtists
		h.host.RenderCards(c, cards, CardView{
			Title:          "歌手 · “" + h.route.Query + "”",
			Count:          cardsTotal(cards),
			OnOpen:         h.openCard,
			OnPlay:         h.playCard,
			Round:          true,
			Header:         func(c *ui.Context) { h.cardsHeader(c, "歌手 · “"+h.route.Query+"”", cardsTotal(cards)) },
			HeaderSkeleton: h.cardsHeaderSkeleton,
		})

	case PageAlbums:
		cards := catalog.AlbumsPager()
		h.host.RenderCards(c, cards, CardView{
			Title:          "专辑",
			Count:          cardsTotal(cards),
			OnOpen:         h.openCard,
			OnPlay:         h.playCard,
			Sort:           albumSortChoices(),
			Current:        catalog.AlbumSort,
			OnSort: func(sort string) {
				catalog.SetAlbumSort(sort)
				h.host.Invalidate()
			},
			Header:         func(c *ui.Context) { h.cardsHeader(c, "专辑", cardsTotal(cards)) },
			HeaderSkeleton: h.cardsHeaderSkeleton,
		})

	case PageArtists:
		cards := catalog.ArtistsPager()
		h.host.RenderCards(c, cards, CardView{
			Title:          "歌手",
			Count:          cardsTotal(cards),
			OnOpen:         h.openCard,
			OnPlay:         h.playCard,
			Round:          true,
			Header:         func(c *ui.Context) { h.cardsHeader(c, "歌手", cardsTotal(cards)) },
			HeaderSkeleton: h.cardsHeaderSkeleton,
		})
	}
}

func cardsTotal(p *paging.Pager[netease.Playlist]) int {
	if p == nil {
		return 0
	}
	return p.Total()
}

func albumSortChoices() []appplatform.SortChoice {
	return []appplatform.SortChoice{
		{Label: "最新", Value: "newest"},
		{Label: "最常播放", Value: "frequent"},
		{Label: "最近播放", Value: "recent"},
		{Label: "专辑名称", Value: "alphabeticalByName"},
		{Label: "歌手名称", Value: "alphabeticalByArtist"},
		{Label: "年份", Value: "byYear"},
		{Label: "收藏", Value: "starred"},
		{Label: "随机", Value: "random"},
	}
}

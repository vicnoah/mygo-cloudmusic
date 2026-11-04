package songloft

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/songloft"
)

func (h *Home) listHeader(c *ui.Context, l *List) {
	t := c.Theme()
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 26, 0).Children(func() {
		h.host.Cover(c, l.Cover(), 212, 14).Shadow(0, 18, 40, -10, ui.RGBA(0, 0, 0, 0.24))
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			ui.Text(c, l.Name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			meta := []string{"Songloft"}
			if l.Sub != "" {
				meta = append(meta, l.Sub)
			}
			if l.Pages != nil && l.Pages.Total() >= 0 {
				meta = append(meta, fmt.Sprintf("%d 首", l.Pages.Total()))
			}
			if len(meta) > 0 {
				ui.Text(c, strings.Join(meta, " · ")).FontSize(13).TextColor(t.TextMuted)
			}
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				if h.host.PillButton(c, "songloft-list-play", iconPlay, "播放", true).Clicked() {
					PlayList(h.host.Player(), l, 0)
				}
				shuffleLabel := "随机播放"
				if l.Shuffle {
					shuffleLabel = "正在准备…"
				}
				if h.host.PillButton(c, "songloft-list-shuffle", iconShuffle, shuffleLabel, false).Clicked() {
					ShuffleList(h.host.Player(), l, h.host.Post)
				}
				if l.Spec.Kind == ListRecent || l.ID == backend.RecentID {
					if h.host.PillButton(c, "songloft-list-clear-recent", iconTrash, "清空", false).Clicked() {
						h.ClearRecent()
					}
				}
				ui.Box(c).Grow(1)
				if control, ok := h.listSort(l); ok {
					control.button(c, "songloft-list-sort")
				}
			})
		})
	})
}

func (h *Home) listHeaderSkeleton(c *ui.Context, l *List) {
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 26, 0).Children(func() {
		h.host.Bone(c, 14).Size(212, 212)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			h.host.TextBone(c, 30, 1.2).WidthPercent(48)
			h.host.TextBone(c, 13, 1.3).Width(180)
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				h.host.Bone(c, 18).Size(92, 36)
				h.host.Bone(c, 18).Size(120, 36)
				ui.Box(c).Grow(1)
				if _, ok := h.listSort(l); ok {
					h.host.Bone(c, 15).Size(96, 30)
				}
			})
		})
	})
}

func (h *Home) listPage(c *ui.Context) {
	catalog := h.state.Catalog()
	kind := h.route.Kind
	if kind == "" {
		if h.route.Page == "artist" {
			kind = "artist"
		} else if h.route.Page == "album" {
			kind = "album"
		}
	}
	l := catalog.ListForRoute(h.route.ID, kind, h.route.Query)
	if h.route.Page == PageLibrary {
		l = catalog.ListForRoute(backend.LibraryID, "library", "")
	}

	if l != nil && l.Pages != nil {
		h.host.RenderTracks(c, l.View(), TrackView{
			Title: l.Name, Sub: l.Kind, Cover: l.Cover(), Total: l.Pages.Total(),
			OnPlay:    func(i int) { PlayList(h.host.Player(), l, i) },
			OnShuffle: func() { ShuffleList(h.host.Player(), l, h.host.Post) },
			OnRefresh: func() { l.Refresh(); h.host.Invalidate() },
			Header: func(c *ui.Context) {
				h.listHeader(c, l)
			},
			HeaderSkeleton: func(c *ui.Context) {
				h.listHeaderSkeleton(c, l)
			},
			Menu: func(m *ui.Menu, tr *netease.Track, index int64) {
				if tr != nil {
					h.trackMenu(m, *tr, l)
				}
			},
			Liked:     h.state.Liked,
			Favorite:  h.RequestFavorite,
			ShowAlbum: l.Spec.Kind != ListAlbum,
		})
	} else {
		h.host.TrackSkeleton(c, TrackView{
			HeaderSkeleton: func(c *ui.Context) {
				h.listHeaderSkeleton(c, nil)
			},
		})
	}
}

func (h *Home) trackMenu(m *ui.Menu, tr netease.Track, l *List) {
	if h.host != nil {
		if m.Item("复制歌名").Chosen() {
			h.host.CopyText(tr.Name)
		}
		if m.Item("复制歌名和歌手").Chosen() {
			h.host.CopyText(tr.Name + " - " + tr.ArtistNames())
		}
		if player := h.host.Player(); player != nil {
			if m.Item("添加到队列").Chosen() {
				player.AppendQueue([]netease.Track{tr})
			}
		}
	}
	if h.state == nil || !h.state.Connected() {
		return
	}

	currentPlID := int64(0)
	if l != nil && l.Spec.Kind == ListPlaylist && l.ID != backend.FavoritesID && l.ID != backend.RecentID && l.ID != backend.LibraryID {
		currentPlID = l.ID
	} else if h.route.Page == PagePlaylist && h.route.ID != backend.FavoritesID && h.route.ID != backend.RecentID && h.route.ID != backend.LibraryID {
		currentPlID = h.route.ID
	}

	pls := h.state.Playlists()
	m.Separator()
	m.Submenu("添加到歌单", func(sm *ui.Menu) {
		if sm.Item("新建歌单…").Chosen() {
			h.RequestNewPlaylist([]netease.Track{tr})
		}
		for _, pl := range pls {
			pl := pl
			if pl.ID != currentPlID && sm.Item(pl.Name).Chosen() {
				h.RequestAddTracks(pl, []netease.Track{tr})
			}
		}
	})

	if currentPlID != 0 {
		var currentPl *netease.Playlist
		for _, pl := range pls {
			if pl.ID == currentPlID {
				currentPl = &pl
				break
			}
		}
		if currentPl == nil && l != nil && l.Spec.Ref != "" {
			currentPl = h.state.FindPlaylistByRef(l.Spec.Ref)
		}
		if currentPl != nil {
			m.Separator()
			if m.Item("从歌单中移除").Chosen() {
				h.RequestRemoveTrack(*currentPl, tr)
			}
		}
	}

	if (l != nil && (l.Spec.Kind == ListRecent || l.ID == backend.RecentID)) || (h.route.Page == PagePlaylist && h.route.ID == backend.RecentID) {
		m.Separator()
		if m.Item("从最近播放中移除").Chosen() {
			h.RemoveRecent(tr.ID)
		}
	}
}

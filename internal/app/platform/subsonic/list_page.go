package subsonic

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	backend "netease-music-mygo/internal/platform/subsonic"
)

func (h *Home) listHeader(c *ui.Context, l *List) {
	t := c.Theme()
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 26, 0).Children(func() {
		h.host.Cover(c, l.Cover(), 212, 14).Shadow(0, 18, 40, -10, ui.RGBA(0, 0, 0, 0.24))
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			ui.Text(c, l.Name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			meta := []string{"Subsonic"}
			if l.Sub != "" {
				meta = append(meta, l.Sub)
			}
			if l.Pages.Total() >= 0 {
				meta = append(meta, fmt.Sprintf("%d 首", l.Pages.Total()))
			}
			if len(meta) > 0 {
				ui.Text(c, strings.Join(meta, " · ")).FontSize(13).TextColor(t.TextMuted)
			}
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				if h.host.PillButton(c, "subsonic-list-play", iconPlay, "播放", true).Clicked() {
					PlayList(h.host.Player(), l, 0)
				}
				if h.host.PillButton(c, "subsonic-list-shuffle", iconShuffle, "随机播放", false).Clicked() {
					ShuffleList(h.host.Player(), l, h.host.Post)
				}
				if l.Spec.Kind == ListAlbum {
					h.albumLikeButton(c, l)
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
				if l != nil && l.Spec.Kind == ListAlbum {
					h.host.Bone(c, 18).Size(96, 36)
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
			Menu:     h.trackMenu,
			Liked:    h.state.Liked,
			Favorite: h.RequestFavorite,
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

func (h *Home) albumLikeButton(c *ui.Context, l *List) {
	if l == nil {
		return
	}
	ref := l.Spec.Ref
	if ref == "" && l.ID != 0 {
		if meta, ok := backend.LookupMeta(l.ID); ok {
			ref = meta.Ref
		}
	}
	if ref == "" {
		return
	}

	liked := h.state.AlbumLiked(ref)
	busy := h.state.AlbumLikeBusy(ref)

	t := c.Theme()
	icon, label, color := iconHeart, "收藏", t.TextMuted
	if liked {
		icon, label, color = iconHeartFill, "已收藏", ui.Hex("#e5484d")
	}

	b := ui.ButtonBase(c.Key("subsonic-album-like")).Height(36).PaddingX(16).Gap(6).Radius(999).AlignItems(ui.Center).Label(label)
	b.Background(t.Text.Alpha(0.06))
	if b.Hovered() {
		b.Background(t.Text.Alpha(0.12))
	}
	if busy {
		b.Disabled(true)
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(color)
		ui.Text(c, label).FontSize(14).FontWeight(600).TextColor(t.Text).SingleLine()
	})
	if b.Clicked() {
		want := !liked
		go func() {
			err := h.state.SetAlbumFavorite(context.Background(), ref, want)
			h.host.Post(func() {
				if err != nil {
					h.host.Toast("操作失败，请稍后再试")
					return
				}
				if want {
					h.host.Toast("已收藏专辑")
				} else {
					h.host.Toast("已取消收藏")
				}
				h.host.Invalidate()
			})
		}()
	}
}

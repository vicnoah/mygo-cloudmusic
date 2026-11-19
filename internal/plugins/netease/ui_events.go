package netease

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/plugins/netease/internal/client"
)

// artistEvents lists what an artist posted, a page at a time.
func (a *app) artistEvents(c *ui.Context, d *artistData) {
	if d.profile.AccountID == 0 {
		a.empty(c, "这位歌手还没有自己的账号，没有动态")
		return
	}
	ui.Column(c).Children(func() {
		for i := range d.events {
			a.event(c.Key(d.events[i].ID), &d.events[i])
		}
		id := d.profile.ID
		a.pageFoot(c, &d.eventsPage, func() { a.loadEvents(id) })
		if d.eventsPage.loaded && len(d.events) == 0 {
			a.empty(c, "暂无动态")
		}
	})
}

// event is a post: who posted it, when, what it says, its pictures and
// what it shares, which opens its page.
func (a *app) event(c *ui.Context, e *netease.Event) {
	p := a.pal
	ui.Column(c).Gap(12).Padding(20, 0).BorderWidth(0, 0, 1, 0).BorderColor(p.hairline).Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			a.cover(c, e.User.AvatarURL, 120, 40, 20)
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, e.User.Nickname).FontSize(14).FontWeight(600).SingleLine()
				ui.Text(c, time.UnixMilli(e.Time).Format("2006-01-02 15:04")).FontSize(12).TextColor(p.muted)
			})
		})
		ct := &e.Content
		if msg := strings.TrimSpace(ct.Msg); msg != "" {
			ui.Text(c, msg).FontSize(14).LineHeight(1.6)
		}
		if ct.Forwarded != "" {
			ui.Text(c, ct.Forwarded).FontSize(13).LineHeight(1.5).TextColor(p.muted).Padding(10, 12).Radius(8).Background(p.hover)
		}
		if len(e.Pictures) > 0 {
			ui.Grid(c).Columns(3).Gap(8).MaxWidth(480).Children(func() {
				for i, pic := range e.Pictures {
					ui.Box(c.Key(i)).AspectRatio(1).Children(func() { a.coverFill(c, pic.SquareURL, 400, 8) })
				}
			})
		}
		switch {
		case ct.Song != nil:
			// A song has no page: its album's shows it.
			s := ct.Song
			a.shared(c, s.Name, "单曲 · "+s.ArtistNames(), s.Album.PicURL, route{page: pageAlbum, id: s.Album.ID})
		case ct.Album != nil:
			a.shared(c, ct.Album.Name, "专辑 · "+ct.Album.ArtistNames(), ct.Album.PicURL, route{page: pageAlbum, id: ct.Album.ID})
		case ct.Playlist != nil:
			a.shared(c, ct.Playlist.Name, "歌单 · "+ct.Playlist.Creator.Nickname, ct.Playlist.Cover(), route{page: pagePlaylist, id: ct.Playlist.ID})
		case ct.Artist != nil:
			a.shared(c, ct.Artist.Name, "歌手", ct.Artist.Avatar, route{page: pageArtist, id: ct.Artist.ID})
		case ct.Video:
			ui.Text(c, "分享了视频，暂不支持在应用内观看").FontSize(12.5).TextColor(p.muted)
		}
		ui.Text(c, fmt.Sprintf("%s 赞 · %s 评论 · %s 转发", count(e.Info.LikedCount), count(e.Info.CommentCount), count(e.Info.ShareCount))).
			FontSize(12).TextColor(p.muted)
	})
}

// shared is what an event shares, as a card that opens its page, if it
// has one.
func (a *app) shared(c *ui.Context, name, sub, pic string, r route) {
	p := a.pal
	card := ui.ButtonBase(c.Key("shared")).Gap(12).Padding(8).Radius(10).AlignItems(ui.Center).Background(p.hover).
		FocusRing(false).Label(name).Disabled(r.id == 0).MaxWidth(480)
	if card.Hovered() {
		card.Background(p.press)
	}
	card.Children(func() {
		a.cover(c, pic, 120, 44, 6)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, name).FontSize(13.5).FontWeight(600).SingleLine()
			ui.Text(c, sub).FontSize(12).TextColor(p.muted).SingleLine()
		})
	})
	if card.Clicked() {
		a.open(r)
	}
}

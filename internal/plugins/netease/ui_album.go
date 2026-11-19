package netease

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/plugins/netease/internal/client"
)

// albumDetail loads an album from NetEase; tests replace it.
var albumDetail = netease.AlbumDetail

// albumData is what the app has of an album.
type albumData struct {
	album   *netease.AlbumProfile
	loading bool
	err     string
}

func (a *app) loadAlbum(id int64) {
	ctx := a.scope
	d := a.albums[id]
	if d == nil {
		d = &albumData{}
		a.albums[id] = d
	}
	if d.album != nil || d.loading {
		return
	}
	d.loading, d.err = true, ""
	post := a.scopedPost()
	go func() {
		al, err := albumDetail(ctx, id)
		post(func() {
			d.loading = false
			if err != nil {
				d.err = "专辑加载失败"
				return
			}
			d.album = al
		})
	}()
}

func (a *app) albumPage(c *ui.Context) {
	id := a.route.id
	d := a.albums[id]
	if d.album == nil {
		if !d.loading {
			a.centered(c, d.err, func() { a.loadAlbum(id) })
			return
		}
		s := a.pageShimmer(c)
		a.trackSkeleton(c, s, false, func(c *ui.Context) { a.playlistHeaderSkeleton(c, s) })
		return
	}
	a.trackRows(c, d.album.Songs, false, func(c *ui.Context) { a.albumHeader(c, d.album) })
}

func (a *app) albumHeader(c *ui.Context, al *netease.AlbumProfile) {
	p := a.pal
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 26, 0).Children(func() {
		ui.Box(c).Size(212, 212).Radius(14).Shadow(0, 18, 40, -10, p.cardShade).Children(func() {
			a.coverFill(c, al.PicURL, 600, 14)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			ui.Text(c, "专辑").FontSize(12).FontWeight(600).TextColor(p.accent)
			ui.Text(c, al.Name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			ui.RichText(c).FontSize(13).TextColor(p.muted).Children(func() {
				for i, ar := range al.Artists {
					if i > 0 {
						ui.Text(c, ", ")
					}
					a.link(c.Key(i), ar.Name, p.muted, p.text, route{page: pageArtist, id: ar.ID})
				}
				meta := fmt.Sprintf(" · %d 首", len(al.Songs))
				if al.PublishTime > 0 {
					meta = " · " + time.UnixMilli(al.PublishTime).Format("2006-01-02") + meta
				}
				ui.Text(c, meta)
			})
			if d := strings.TrimSpace(al.Description); d != "" {
				ui.Text(c, strings.ReplaceAll(d, "\n", " ")).FontSize(12.5).TextColor(p.muted).MaxLines(2).LineHeight(1.45).Margin(2, 0, 0, 0)
			}
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(func() {
				if pillButton(c.Key("play"), iconPlay, "播放", p.accent, p.onAccent).Clicked() {
					if a.pl.Shuffle() {
						a.pl.ToggleShuffle()
					}
					a.pl.PlayQueue(al.Songs, firstPlayable(al.Songs))
				}
			})
		})
	})
}

// albumGrid shows albums as cards, each opening its page.
func (a *app) albumGrid(c *ui.Context, albums []netease.AlbumProfile) {
	ui.Grid(c).Columns(a.columns(c, 156)).GapX(20).GapY(28).Children(func() {
		for i := range albums {
			a.albumCard(c.Key(albums[i].ID), &albums[i])
		}
	})
}

// albumCard is an album's cover, lifting under the pointer, over its name
// and the year it came out.
func (a *app) albumCard(c *ui.Context, al *netease.AlbumProfile) {
	p := a.pal
	card := ui.ButtonBase(c).Column().Gap(10).AlignItems(ui.Stretch).FocusRing(false).Label(al.Name)
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
		art.Children(func() { a.coverFill(c, al.PicURL, 400, 10) })
		ui.Text(c, al.Name).FontSize(13).FontWeight(500).MaxLines(2).LineHeight(1.35)
		var meta []string
		if a.route.page != pageArtist {
			meta = append(meta, al.ArtistNames())
		}
		if al.PublishTime > 0 {
			meta = append(meta, time.UnixMilli(al.PublishTime).Format("2006"))
		}
		ui.Text(c, strings.Join(meta, " · ")).FontSize(12).TextColor(p.muted).SingleLine().Margin(-6, 0, 0, 0)
	})
	if card.Clicked() {
		a.open(route{page: pageAlbum, id: al.ID})
	}
}

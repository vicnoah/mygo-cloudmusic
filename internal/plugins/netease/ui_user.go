package netease

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/plugins/netease/internal/client"
)

// userDetail and userPlaylists load a user and their playlists from
// NetEase, and followedArtists and subscribedPodcasts what the listener
// follows; tests replace them.
var (
	userDetail         = netease.UserDetail
	userPlaylists      = netease.UserPlaylists
	followedArtists    = netease.FollowedArtists
	subscribedPodcasts = netease.SubscribedPodcasts
)

// userData is what the app has of a user: who they are, and the playlists
// they made that anyone may see. Of the listener's own page, it has the
// artists they follow and the podcasts they subscribe to too, which
// NetEase shows no one else.
type userData struct {
	user      *netease.User
	playlists []netease.Playlist
	loading   int // how many of the two are on their way
	err       string

	artists       []netease.ArtistProfile
	podcasts      []netease.Podcast
	artistsShown  bool // whether artists has loaded once
	podcastsShown bool
	subsLoading   int
}

func (a *app) loadUser(id int64) {
	ctx := a.scope
	d := a.users[id]
	if d == nil {
		d = &userData{}
		a.users[id] = d
	}
	if a.profile != nil && a.profile.UserID == id {
		a.loadSubscriptions(d)
	}
	if d.user != nil || d.loading > 0 {
		return
	}
	d.loading, d.err = 2, ""
	post := a.scopedPost()
	go func() {
		u, err := userDetail(ctx, id)
		post(func() {
			d.loading--
			if err != nil {
				d.err = "用户加载失败"
				return
			}
			d.user = u
		})
	}()
	go func() {
		made, _, err := userPlaylists(ctx, id)
		post(func() {
			d.loading--
			if err == nil {
				d.playlists = publicPlaylists(made, id)
			}
		})
	}()
}

// loadSubscriptions loads the artists the listener follows and the
// podcasts they subscribe to, each time their page shows: they follow and
// subscribe on other pages, and these keep up. What they had shows while
// the lists are on their way.
func (a *app) loadSubscriptions(d *userData) {
	ctx := a.scope
	if d.subsLoading > 0 {
		return
	}
	d.subsLoading = 2
	post := a.scopedPost()
	go func() {
		ars, err := followedArtists(ctx)
		post(func() {
			d.subsLoading--
			if err == nil {
				d.artists, d.artistsShown = ars, true
			}
		})
	}()
	go func() {
		pods, err := subscribedPodcasts(ctx)
		post(func() {
			d.subsLoading--
			if err == nil {
				d.podcasts, d.podcastsShown = pods, true
			}
		})
	}()
}

// publicPlaylists are the playlists of lists a user made that anyone may
// see: their own, as made lists those they saved too, but not their
// private ones.
func publicPlaylists(lists []netease.Playlist, uid int64) []netease.Playlist {
	var out []netease.Playlist
	for _, pl := range lists {
		if pl.Creator.UserID == uid && pl.Privacy != 10 {
			out = append(out, pl)
		}
	}
	return out
}

func (a *app) userPage(c *ui.Context) {
	p := a.pal
	id := a.route.id
	d := a.users[id]
	if d.user == nil {
		if d.loading == 0 {
			a.centered(c, d.err, func() { a.loadUser(id) })
			return
		}
		s := a.pageShimmer(c)
		a.scrollPage(c, func() {
			a.playlistHeaderSkeleton(c, s)
			a.playlistCardsSkeleton(c, s, a.columns(c, 168), 168, false)
		})
		return
	}
	u := d.user
	own := a.profile != nil && a.profile.UserID == id
	a.scrollPage(c, func() {
		ui.Row(c).Gap(28).AlignItems(ui.End).Children(func() {
			ui.Box(c).Size(artistPhotoSize, artistPhotoSize).Radius(artistPhotoSize/2).Shadow(0, 18, 40, -10, p.cardShade).Children(func() {
				a.coverFill(c, u.AvatarURL, 600, artistPhotoSize/2)
			})
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
				ui.Text(c, "用户").FontSize(12).FontWeight(600).TextColor(p.accent)
				ui.Text(c, u.Nickname).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
				ui.Text(c, fmt.Sprintf("%s 关注 · %s 粉丝", count(u.Follows), count(u.Followeds))).FontSize(13).TextColor(p.muted)
				if u.Signature != "" {
					ui.Text(c, u.Signature).FontSize(12.5).TextColor(p.muted).MaxLines(2).LineHeight(1.45).Margin(2, 0, 0, 0)
				}
			})
		})
		a.section(c, "创建的歌单")
		switch {
		case d.loading > 0:
			a.playlistCardsSkeleton(c, a.pageShimmer(c), a.columns(c, 168), 168, false)
		case len(d.playlists) == 0:
			a.empty(c, "暂无公开歌单")
		default:
			ui.Grid(c).Columns(a.columns(c, 168)).GapX(20).GapY(28).Children(func() {
				for i := range d.playlists {
					a.playlistCard(c.Key(d.playlists[i].ID), &d.playlists[i])
				}
			})
		}
		if own && d.podcastsShown {
			a.section(c, "订阅的播客")
			if len(d.podcasts) == 0 {
				a.empty(c, "还没有订阅播客")
			}
			ui.Grid(c).Columns(a.columns(c, 340)).GapX(8).GapY(4).Margin(0, -12).Children(func() {
				for i := range d.podcasts {
					a.podcastRow(c.Key(d.podcasts[i].ID), &d.podcasts[i])
				}
			})
		}
		if own && d.artistsShown {
			a.section(c, "关注的歌手")
			if len(d.artists) == 0 {
				a.empty(c, "还没有关注歌手")
			}
			ui.Grid(c).Columns(a.columns(c, 132)).GapX(20).GapY(28).Children(func() {
				for i := range d.artists {
					a.artistCard(c.Key(d.artists[i].ID), &d.artists[i])
				}
			})
		}
	})
}

// userRow is a user in a list, opening their page.
func (a *app) userRow(c *ui.Context, u *netease.User) {
	p := a.pal
	row := ui.ButtonBase(c).Padding(10, 12).Gap(14).Radius(10).AlignItems(ui.Center).FocusRing(false).Label(u.Nickname)
	if row.Hovered() {
		row.Background(p.hover)
	}
	row.Children(func() {
		a.cover(c, u.AvatarURL, 120, 48, 24)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, u.Nickname).FontSize(14).FontWeight(600).SingleLine()
			if u.Signature != "" {
				ui.Text(c, u.Signature).FontSize(12.5).TextColor(p.muted).SingleLine()
			}
		})
	})
	if row.Clicked() {
		a.open(route{page: pageUser, id: u.UserID})
	}
}

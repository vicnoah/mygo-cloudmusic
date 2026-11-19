package fnmusic

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	backend "waygu-music/internal/plugins/fnmusic/internal/client"
)

func FavoritesRoute() Route {
	return Route{Page: PagePlaylist, ID: backend.FavoritesID}
}

func ArtistSongsRoute(id int64, ref string) Route {
	return Route{Page: PagePlaylist, ID: id, Kind: backend.KindArtist, Query: ref}
}

func (h *Home) CloseTransient() {
	if h == nil {
		return
	}
	h.accountOpen, h.platformMenuOpen = false, false
	h.closePlaybackView()
	h.connectionGen++
	h.connectionOpen, h.connectionBusy, h.connectionError = false, false, ""
	h.nameOpen, h.deleteOpen = false, false
}

func (h *Home) sidebar(c *ui.Context) {
	if h == nil || h.host == nil || h.state == nil {
		return
	}
	h.searchField(c, "搜索歌曲、歌手、播客", func(query string) {
		if query = strings.TrimSpace(query); query != "" {
			h.navigate(Route{Page: PageSearch, Query: query, Kind: "songs"})
		}
	})
	ui.Scroll(c.Key("fnmusic-nav")).TrackScroll(&h.navScroll).Grow(1).Margin(0, -6).Padding(0, 6).Gap(4).Children(func() {
		if h.state.Connecting() {
			h.sidebarLoading(c)
		} else if !h.state.Connected() {
			h.navItem(c, iconServer, "连接飞牛音乐", Route{Page: PageLibrary})
		} else {
			h.navItem(c, iconNote, "音乐库", Route{Page: PageLibrary})
			if h.state.FolderViewEnabled() {
				h.navItem(c, iconFolder, "文件夹", Route{Page: PageFolders})
			}
			h.navItem(c, iconGenre, "风格", Route{Page: PageGenres})
			h.navItem(c, iconAlbum, "专辑", Route{Page: PageAlbums})
			h.navItem(c, iconUser, "歌手", Route{Page: PageArtists})
			h.navItem(c, iconHeart, "我喜欢", FavoritesRoute())
			h.navItem(c, iconClock, "最近播放", Route{Page: PagePlaylist, ID: backend.RecentID})
			h.navItem(c, iconRadio, "漫游", Route{Page: PageRoam})
		}
		if h.route.Page == PageSearch || h.route.Page == PageSearchAlbums || h.route.Page == PageSearchArtists {
			h.navItem(c, iconSearch, "搜索结果", h.route)
		}
		if h.state.Connected() {
			playlistHeader(c, func() { h.RequestNewPlaylist(nil) })
			for _, pl := range h.state.Playlists() {
				r := Route{Page: PagePlaylist, ID: pl.ID}
				h.playlistItem(c, fmt.Sprintf("fnmusic-playlist-%d", pl.ID), pl, h.route == r, func(m *ui.Menu) { h.playlistMenu(m, pl) }, func() {
					h.navigate(r)
				})
			}
		}
	})
	if player := h.host.Player(); player != nil {
		if tr := player.Current(); tr != nil {
			h.sidebarNowPlaying(c, tr)
		}
	}
	cfg := toConnectionConfig(h.state.Config())
	username, status := cfg.Username, "未连接"
	if h.state.Connecting() {
		if username == "" {
			username = "飞牛音乐"
		}
		status = "正在连接…"
	}
	if username == "" {
		username = "飞牛音乐（未连接）"
	}
	connected := h.state.Connected()
	if connected {
		if username == "飞牛音乐（未连接）" {
			username = "飞牛音乐"
		}
		status = "飞牛音乐"
		if err := h.state.Error(); err != nil {
			status = "连接失败"
		}
	} else if err := h.state.Error(); err != nil {
		status = "连接失败"
	}
	actions := h.accountActions()
	h.accountRow(c, username, status, actions)
	h.platformSwitcher(c)
}

// sidebarLoading keeps the configured platform layout stable while the first
// connection and playlist request are in flight. Showing the connect CTA here
// made the sidebar flash to a different navigation tree on every launch.
func (h *Home) sidebarLoading(c *ui.Context) {
	// As many as the connected sidebar has, so it does not grow once in.
	for i, w := range []float32{56, 44, 36, 36, 36, 52, 64, 36} {
		ui.Row(c.Key(fmt.Sprintf("fnmusic-nav-loading-%d", i))).Height(34).PaddingX(10).Gap(10).AlignItems(ui.Center).Children(func() {
			bone(c, 5).Size(17, 17)
			textBone(c, 13.5, 1.3).Width(w)
		})
	}
	// The playlists' heading, and a few of them.
	ui.Row(c.Key("fnmusic-nav-loading-lists")).Height(30).PaddingX(10).Margin(14, 0, 0, 0).AlignItems(ui.Center).Children(func() {
		textBone(c, 12, 1.3).Width(48)
	})
	for i, w := range []float32{88, 64, 104} {
		ui.Row(c.Key(fmt.Sprintf("fnmusic-nav-loading-list-%d", i))).Height(40).PaddingX(10).Gap(10).AlignItems(ui.Center).Children(func() {
			bone(c, 6).Size(28, 28)
			textBone(c, 13, 1.3).Width(w)
		})
	}
}

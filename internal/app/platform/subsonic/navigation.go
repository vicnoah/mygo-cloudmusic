package subsonic

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	backend "netease-music-mygo/internal/platform/subsonic"
)

func (h *Home) Sidebar(c *ui.Context) {
	if h == nil || h.host == nil || h.state == nil {
		return
	}
	h.host.PlatformSearchField(c, "搜索歌曲、专辑、歌手", func(query string) {
		if query = strings.TrimSpace(query); query != "" {
			h.Open(Route{Page: PageSearch, Query: query, Kind: "songs"})
			h.host.Navigate(h.route)
		}
	})

	prof := h.state.Profile()
	ui.Scroll(c.Key(string(prof.ID)+"-nav")).TrackScroll(&h.navScroll).Grow(1).Margin(0, -6).Padding(0, 6).Gap(4).Children(func() {
		if h.state.Connecting() {
			h.sidebarLoading(c)
		} else if !h.state.Connected() {
			h.host.NavItem(c, iconServer, "连接 "+prof.Name, Route{Page: PageLibrary})
		} else {
			h.host.NavItem(c, iconNote, "音乐库", Route{Page: PageLibrary})
			h.host.NavItem(c, iconAlbum, "专辑", Route{Page: PageAlbums})
			h.host.NavItem(c, iconUser, "歌手", Route{Page: PageArtists})
			h.host.NavItem(c, iconHeart, "我喜欢", Route{Page: PagePlaylist, ID: backend.FavoritesID})
			h.host.NavItem(c, iconFolder, "文件夹", Route{Page: PageFolders})
		}

		if h.route.Page == PageSearch || h.route.Page == PageSearchAlbums || h.route.Page == PageSearchArtists {
			h.host.NavItem(c, iconSearch, "搜索结果", h.route)
		}

		if h.state.Connected() {
			h.host.PlaylistHeader(c, func() { h.RequestNewPlaylist(nil) })
			for _, pl := range h.state.Playlists() {
				r := Route{Page: PagePlaylist, ID: pl.ID}
				h.host.PlaylistItem(c, fmt.Sprintf("%s-playlist-%d", prof.ID, pl.ID), pl, h.route == r, func(m *ui.Menu) {
					h.playlistMenu(m, pl)
				}, func() {
					h.host.Navigate(r)
				})
			}
		}
	})

	if player := h.host.Player(); player != nil {
		if tr := player.Current(); tr != nil {
			h.host.NowPlaying(c, tr)
		}
	}

	cfg := toConnectionConfig(h.state.Config())
	username, status := cfg.Username, "未连接"
	if h.state.Connecting() {
		if username == "" {
			username = prof.Name
		}
		status = "正在连接…"
	}
	if username == "" {
		username = prof.Name + "（未连接）"
	}
	if h.state.Connected() {
		if username == prof.Name+"（未连接）" {
			username = prof.Name
		}
		status = "已连接"
		if info := h.state.Client().ServerInfo(); info != nil && info.ServerVersion != "" {
			status = info.Type + " " + info.ServerVersion
		}
	} else if err := h.state.Error(); err != nil {
		status = "连接失败"
	}

	actions := h.accountActions()
	h.host.AccountRow(c, iconServer, "", username, status, &h.accountOpen, actions)
	h.host.PlatformSwitcher(c)
}

func (h *Home) sidebarLoading(c *ui.Context) {
	for i, w := range []float32{56, 44, 36, 36, 52} {
		ui.Row(c.Key(fmt.Sprintf("subsonic-nav-loading-%d", i))).Height(34).PaddingX(10).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(16, 16).Radius(4).Background(c.Theme().SurfaceHover)
			ui.Box(c).Width(w).Height(12).Radius(4).Background(c.Theme().SurfaceHover)
		})
	}
}

func (h *Home) accountActions() []AccountAction {
	openSettings := func() {
		if h.host != nil {
			h.host.OpenSettings()
		}
	}
	settings := AccountAction{Icon: iconSettings, Label: "设置…", Shortcut: "Ctrl+,", Action: openSettings}
	prof := h.state.Profile()
	name := prof.Name
	if name == "" {
		name = "Subsonic"
	}
	if h.state.Connecting() {
		return []AccountAction{settings}
	}
	if h.state.Connected() {
		return []AccountAction{
			settings,
			{Icon: iconReload, Label: "重新连接…", Separator: true, Action: func() { h.openConnection() }},
			{Icon: iconSignOut, Label: "断开" + name, Danger: true, Action: func() { h.disconnect() }},
		}
	}
	return []AccountAction{
		settings,
		{Icon: iconReload, Label: "连接 " + name + "…", Separator: true, Action: func() { h.openConnection() }},
	}
}

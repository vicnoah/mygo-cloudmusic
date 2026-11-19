package subsonic

import (
	"github.com/egoist/mygo/ui"
	"waygu-music/internal/pluginapi"
	backend "waygu-music/internal/plugins/subsonic/internal/client"
)

func (h *Home) playlistMenu(m *ui.Menu, pl pluginapi.Playlist) {
	if m.Item("重命名…").Chosen() {
		h.RequestRename(pl)
	}
	m.Separator()
	if m.Item("删除歌单…").Chosen() {
		h.RequestDelete(pl)
	}
}

func (h *Home) trackMenu(m *ui.Menu, tr *pluginapi.Track, pid int64) {
	if tr == nil {
		return
	}
	if m.Item("复制歌名").Chosen() {
		h.host.CopyText(tr.Name)
	}
	if m.Item("复制歌名和歌手").Chosen() {
		h.host.CopyText(tr.Name + " - " + tr.ArtistNames())
	}
	if h.state == nil || !h.state.Connected() {
		return
	}
	m.Separator()
	m.Submenu("添加到歌单", func(sub *ui.Menu) {
		if sub.Item("新建歌单…").Chosen() {
			h.RequestNewPlaylist([]pluginapi.Track{*tr})
		}
		for _, pl := range h.state.Playlists() {
			if pl.ID != pid && sub.Item(pl.Name).Chosen() {
				h.RequestAddTracks(pl, []pluginapi.Track{*tr})
			}
		}
	})
	if pid == 0 && h.route.Page == PagePlaylist {
		pid = h.route.ID
	}
	if pid == backend.RecentID && h.route.Page == PagePlaylist {
		if m.Item("从最近播放中移除").Chosen() {
			h.RemoveRecent(tr.ID)
		}
	}
	if pid != 0 && h.route.Page == PagePlaylist && h.route.ID == pid {
		var currentPl *pluginapi.Playlist
		for _, pl := range h.state.Playlists() {
			if pl.ID == pid {
				currentPl = &pl
				break
			}
		}
		if currentPl != nil {
			if m.Item("从歌单中移除").Chosen() {
				h.RequestRemoveTrack(*currentPl, *tr)
			}
		}
	}
}

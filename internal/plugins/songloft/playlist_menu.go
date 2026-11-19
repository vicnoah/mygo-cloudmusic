package songloft

import (
	"github.com/egoist/mygo/ui"
	"waygu-music/internal/pluginapi"
)

func (h *Home) playlistMenu(m *ui.Menu, pl pluginapi.Playlist) {
	if m.Item("播放").Chosen() {
		if cat := h.state.Catalog(); cat != nil {
			l := cat.ListForRoute(pl.ID, "playlist", pl.Ref)
			PlayList(h.host.Player(), l, 0)
		}
	}
	if m.Item("随机播放").Chosen() {
		if cat := h.state.Catalog(); cat != nil {
			l := cat.ListForRoute(pl.ID, "playlist", pl.Ref)
			ShuffleList(h.scope, h.host.Player(), l, h.host.Post)
		}
	}
	m.Separator()
	if m.Item("重命名").Chosen() {
		h.RequestRename(pl)
	}
	if m.Item("删除歌单").Chosen() {
		h.RequestDelete(pl)
	}
}

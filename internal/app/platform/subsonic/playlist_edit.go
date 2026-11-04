package subsonic

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
)

func (h *Home) RequestNewPlaylist(tracks []netease.Track) {
	h.nameOpen, h.name, h.rename, h.delete, h.deleteOpen = true, "", nil, nil, false
	h.addTracks = append([]netease.Track(nil), tracks...)
}

func (h *Home) RequestRename(pl netease.Playlist) {
	h.nameOpen, h.name, h.rename, h.delete, h.deleteOpen = true, pl.Name, &pl, nil, false
}

func (h *Home) RequestDelete(pl netease.Playlist) {
	h.delete, h.deleteOpen = &pl, true
}

func (h *Home) RequestAddTracks(pl netease.Playlist, tracks []netease.Track) {
	if h == nil || h.state == nil {
		return
	}
	provider := h.state.Provider()
	if provider == nil {
		return
	}
	go func() {
		err := provider.AddToPlaylist(context.Background(), pl, tracks)
		if h.host != nil {
			h.host.Post(func() {
				if err != nil {
					h.host.Toast("添加到歌单失败")
				} else {
					h.host.Toast("已添加到歌单")
					_ = h.state.RefreshPlaylists(context.Background())
				}
			})
		}
	}()
}

func (h *Home) RequestRemoveTrack(pl netease.Playlist, tr netease.Track) {
	if h == nil || h.state == nil {
		return
	}
	provider := h.state.Provider()
	if provider == nil {
		return
	}
	go func() {
		err := provider.RemoveFromPlaylist(context.Background(), pl, []netease.Track{tr})
		if h.host != nil {
			h.host.Post(func() {
				if err != nil {
					h.host.Toast("移除失败")
				} else {
					h.host.Toast("已从歌单移除")
					if l, ok := h.state.Catalog().Lists[pl.ID]; ok {
						l.Refresh()
					}
					h.host.Invalidate()
				}
			})
		}
	}()
}

func (h *Home) submitName() {
	name := strings.TrimSpace(h.name)
	if name == "" || h.state == nil {
		return
	}
	provider := h.state.Provider()
	if provider == nil {
		return
	}

	if h.rename != nil {
		pl := *h.rename
		h.nameOpen, h.rename = false, nil
		go func() {
			err := provider.RenamePlaylist(context.Background(), pl, name)
			if h.host != nil {
				h.host.Post(func() {
					if err != nil {
						h.host.Toast("重命名失败")
					} else {
						h.host.Toast("已重命名")
						_ = h.state.RefreshPlaylists(context.Background())
					}
				})
			}
		}()
		return
	}

	tracks := append([]netease.Track(nil), h.addTracks...)
	h.nameOpen = false
	go func() {
		pl, err := provider.CreatePlaylist(context.Background(), name)
		if h.host != nil {
			h.host.Post(func() {
				if err != nil {
					h.host.Toast("创建歌单失败")
					return
				}
				h.host.Toast("已创建歌单")
				_ = h.state.RefreshPlaylists(context.Background())
				if len(tracks) > 0 {
					h.RequestAddTracks(pl, tracks)
				}
				h.host.Navigate(Route{Page: PagePlaylist, ID: pl.ID})
			})
		}
	}()
}

func (h *Home) confirmDelete() {
	if h.delete == nil || h.state == nil {
		return
	}
	pl := *h.delete
	h.delete, h.deleteOpen = nil, false
	provider := h.state.Provider()
	if provider == nil {
		return
	}
	go func() {
		err := provider.DeletePlaylist(context.Background(), pl)
		if h.host != nil {
			h.host.Post(func() {
				if err != nil {
					h.host.Toast("删除歌单失败")
				} else {
					h.host.Toast("已删除歌单")
					_ = h.state.RefreshPlaylists(context.Background())
					if h.route.ID == pl.ID {
						h.Open(Route{Page: PageLibrary})
						h.host.Navigate(h.route)
					}
				}
			})
		}
	}()
}

func (h *Home) dialogs(c *ui.Context) {
	t := c.Theme()
	if h.nameOpen {
		ui.DialogBase(c.Key("subsonic-playlist-name"), &h.nameOpen, func(backdrop, panel ui.Element) {
			backdrop.Background(ui.RGBA(0, 0, 0, .35))
			panel.Width(360).Padding(24).Radius(18).Gap(14).Background(t.Background).Border(1, t.Border)
			title := "新建歌单"
			if h.rename != nil {
				title = "重命名歌单"
			}
			ui.Text(c, title).FontSize(17).FontWeight(700)
			in := ui.TextInput(c.Key("subsonic-playlist-name-input"), &h.name).Placeholder("歌单名称").Label("歌单名称").FontSize(14).AutoFocus()
			if in.Submitted() {
				h.submitName()
			}
			ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
				if ui.Button(c.Key("subsonic-playlist-name-cancel"), "取消").Clicked() {
					h.nameOpen = false
				}
				ok := ui.PrimaryButton(c.Key("subsonic-playlist-name-ok"), "确定")
				if strings.TrimSpace(h.name) == "" {
					ok.Disabled(true)
				}
				if ok.Clicked() {
					h.submitName()
				}
			})
		})
	}
	if h.delete != nil && h.deleteOpen {
		ui.DialogBase(c.Key("subsonic-playlist-delete"), &h.deleteOpen, func(backdrop, panel ui.Element) {
			backdrop.Background(ui.RGBA(0, 0, 0, .35))
			panel.Width(360).Padding(24).Radius(18).Gap(12).Background(t.Background).Border(1, t.Border)
			ui.Text(c, "删除歌单").FontSize(17).FontWeight(700)
			ui.Text(c, "确定删除歌单“"+h.delete.Name+"”吗？删除后无法恢复。").FontSize(13).TextColor(t.TextMuted)
			ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
				if ui.Button(c.Key("subsonic-delete-cancel"), "取消").Clicked() {
					h.delete, h.deleteOpen = nil, false
				}
				if ui.PrimaryButton(c.Key("subsonic-delete-ok"), "删除").Clicked() {
					h.confirmDelete()
				}
			})
		})
	}
}

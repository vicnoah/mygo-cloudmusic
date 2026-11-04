package songloft

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/songloft"
)

func (h *Home) loadFolder(path string) {
	if h.state == nil || !h.state.Connected() {
		return
	}
	client := h.state.Client()
	if client == nil {
		return
	}
	h.folderPath = path
	h.folderLoading = true
	h.folderError = ""
	gen := h.state.Generation()

	go func() {
		res, err := client.ListFolders(context.Background(), path, "")
		if h.host != nil {
			h.host.Post(func() {
				if gen != h.state.Generation() {
					return
				}
				h.folderLoading = false
				if err != nil {
					h.folderError = err.Error()
				} else {
					var songs []netease.Track
					for _, s := range res.Songs {
						if tr, ok := backend.ToTrack(&s, h.state.Liked(s.ID)); ok {
							songs = append(songs, tr)
						}
					}
					h.folderData = backend.FolderPage{
						Path:       res.Path,
						ParentPath: res.ParentPath,
						Folders:    res.Folders,
						Songs:      songs,
					}
					h.updateFolderCrumbs(path)
				}
				h.host.Invalidate()
			})
		}
	}()
}

func (h *Home) openFolder(path, name string) {
	h.Open(Route{Page: PageFolders, Query: path})
}

func (h *Home) updateFolderCrumbs(path string) {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		h.folderCrumbs = []folderCrumb{{Path: "", Name: "文件夹"}}
		return
	}
	parts := strings.Split(path, "/")
	crumbs := []folderCrumb{{Path: "", Name: "文件夹"}}
	cur := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		if cur == "" {
			cur = part
		} else {
			cur += "/" + part
		}
		crumbs = append(crumbs, folderCrumb{Path: cur, Name: part})
	}
	h.folderCrumbs = crumbs
}

func folderTile(c *ui.Context, size, radius, icon float32) ui.Element {
	t := c.Theme()
	return ui.Box(c).Size(size, size).Shrink(0).Radius(radius).Background(t.Accent.Alpha(0.12)).Center().Children(func() {
		ui.Icon(c, iconFolder).Size(icon, icon).TextColor(t.Accent)
	})
}

func (h *Home) folderCrumbBar(c *ui.Context) {
	t := c.Theme()
	crumbs := h.folderCrumbs
	if len(crumbs) == 0 {
		crumbs = []folderCrumb{{Path: "", Name: "文件夹"}}
	}
	ui.Row(c).Gap(4).AlignItems(ui.Center).MinWidth(0).Children(func() {
		for i, crumb := range crumbs {
			if i > 0 {
				ui.Text(c, "›").FontSize(12).TextColor(t.TextMuted)
			}
			idx := i
			cr := crumb
			isLast := i == len(crumbs)-1
			b := ui.ButtonBase(c.Key(fmt.Sprintf("songloft-folder-crumb-%d-%s", idx, cr.Path))).FocusRing(false).Label(cr.Name).Shrink(1).MinWidth(0)
			color := t.Accent
			if isLast {
				color = t.Text
			} else if b.Hovered() {
				color = t.Text
			}
			b.Children(func() {
				weight := 600
				if isLast {
					weight = 700
				}
				ui.Text(c, cr.Name).FontSize(12).FontWeight(weight).TextColor(color).SingleLine()
			})
			if b.Clicked() && !isLast {
				h.openFolder(cr.Path, cr.Name)
			}
		}
	})
}

func (h *Home) folderHeaderSkeleton(c *ui.Context) {
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 12, 26, 12).Children(func() {
		h.host.Bone(c, 14).Size(212, 212)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			h.host.TextBone(c, 12, 1.3).Width(120)
			h.host.TextBone(c, 30, 1.2).WidthPercent(48)
			h.host.TextBone(c, 13, 1.3).Width(180)
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				h.host.Bone(c, 18).Size(92, 36)
				h.host.Bone(c, 18).Size(120, 36)
			})
		})
	})
}

func (h *Home) folderHeader(c *ui.Context, page backend.FolderPage) {
	t := c.Theme()
	name := "文件夹"
	if len(h.folderCrumbs) > 0 {
		name = h.folderCrumbs[len(h.folderCrumbs)-1].Name
	}
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 12, 26, 12).Children(func() {
		folderTile(c, 212, 14, 72).Shadow(0, 18, 40, -10, ui.RGBA(0, 0, 0, 0.18))
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			h.folderCrumbBar(c)
			ui.Text(c, name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			meta := []string{"Songloft"}
			if n := len(page.Folders); n > 0 {
				meta = append(meta, fmt.Sprintf("%d 个文件夹", n))
			}
			if n := len(page.Songs); n > 0 {
				meta = append(meta, fmt.Sprintf("%d 首", n))
			}
			if len(page.Folders) == 0 && len(page.Songs) == 0 {
				meta = append(meta, "空文件夹")
			}
			ui.Text(c, strings.Join(meta, " · ")).FontSize(13).TextColor(t.TextMuted)
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				if len(page.Songs) > 0 {
					if h.host.PillButton(c, "songloft-folder-play", iconPlay, "播放", true).Clicked() {
						if player := h.host.Player(); player != nil {
							player.PlayQueue(page.Songs, 0)
						}
					}
					if h.host.PillButton(c, "songloft-folder-shuffle", iconShuffle, "随机播放", false).Clicked() {
						if player := h.host.Player(); player != nil {
							player.PlayQueue(shuffleTracks(page.Songs), 0)
						}
					}
				}
				if page.ParentPath != "" || h.folderPath != "" {
					if h.host.PillButton(c, "songloft-folder-back", nil, "返回上级", false).Clicked() {
						h.Open(Route{Page: PageFolders, Query: page.ParentPath})
					}
				}
			})
		})
	})
}

func (h *Home) folderRow(c *ui.Context, folder backend.FolderItem) {
	t := c.Theme()
	row := ui.ButtonBase(c.Key("songloft-folder-row-" + folder.Path)).Height(54).PaddingX(12).Gap(14).Radius(10).
		AlignItems(ui.Center).FocusRing(false).Label(folder.Name)
	if row.Hovered() {
		row.Background(t.Text.Alpha(0.05))
	}
	if row.Clicked() {
		h.openFolder(folder.Path, folder.Name)
	}
	row.Children(func() {
		ui.Box(c).Width(26)
		folderTile(c, 38, 6, 18)
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, folder.Name).FontSize(14).FontWeight(500).SingleLine()
			note := "子文件夹"
			if folder.SongCount > 0 {
				note = fmt.Sprintf("%d 首", folder.SongCount)
			}
			ui.Text(c, note).FontSize(12.5).TextColor(t.TextMuted).SingleLine()
		})
		ui.Text(c, "›").FontSize(18).TextColor(t.TextMuted).Margin(0, 8, 0, 0)
	})
}

func (h *Home) folderPage(c *ui.Context) {
	if h.folderLoading {
		h.host.TrackSkeleton(c, TrackView{
			ShowAlbum:      true,
			HeaderSkeleton: h.folderHeaderSkeleton,
		})
		return
	}

	if h.folderError != "" {
		t := c.Theme()
		ui.Column(c).Fill().Center().Gap(10).Children(func() {
			ui.Icon(c, iconFolder).Size(48, 48).TextColor(t.TextMuted)
			msg := h.folderError
			if strings.Contains(msg, "strconv.ParseInt") || strings.Contains(msg, "无效的歌曲ID") {
				msg = "当前 Songloft 服务器版本暂不支持按文件夹层级浏览，请在音乐库或歌单中浏览歌曲。"
			}
			ui.Text(c, "加载目录失败").FontSize(17).FontWeight(700)
			ui.Text(c, msg).FontSize(13).TextColor(t.TextMuted)
			if h.host.PillButton(c, "songloft-folder-retry", iconReload, "重试", true).Clicked() {
				h.loadFolder(h.folderPath)
			}
		})
		return
	}

	page := h.folderData
	tracks := page.Songs
	view := TrackView{
		ShowAlbum: true,
		OnPlay: func(at int) {
			if p := h.host.Player(); p != nil {
				p.PlayQueue(tracks, at)
			}
		},
		Liked:    h.state.Liked,
		Favorite: h.RequestFavorite,
		Menu: func(m *ui.Menu, tr *netease.Track, _ int64) {
			if tr != nil {
				h.trackMenu(m, *tr, nil)
			}
		},
	}

	rows := 1 + len(page.Folders) + len(tracks)
	h.host.PageList(c, "songloft-folder-"+h.folderPath, &h.folderListState, rows, func(i int) {
		if i == 0 {
			h.folderHeader(c, page)
			return
		}
		i--
		if i < len(page.Folders) {
			folder := page.Folders[i]
			h.folderRow(c, folder)
			return
		}
		i -= len(page.Folders)
		tr := &tracks[i]
		h.host.TrackRow(c.Key(fmt.Sprintf("songloft-folder-track-%d", tr.ID)), i, tr, view)
	})
}

func shuffleTracks(tracks []netease.Track) []netease.Track {
	out := append([]netease.Track(nil), tracks...)
	for i := len(out) - 1; i > 0; i-- {
		j := rand.IntN(i + 1)
		out[i], out[j] = out[j], out[i]
	}
	return out
}

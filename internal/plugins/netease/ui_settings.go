package netease

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginapi"
)

func (a *app) settingsWindow(c *ui.Context) {
	if message := a.host.TakeToast(); a.toast == "" {
		a.toast = message
	}
	if a.toast != "" {
		c.Toast(a.toast)
		a.toast = ""
	}
	c.OnShortcut(ui.Cmd, ui.KeyW, a.host.CloseSettings)
	state := a.host.DesktopSettings()
	a.refreshSharedSettings()
	t := *c.Theme()
	p := paletteFor(&t)
	t.Accent, t.AccentText = p.accent, p.onAccent
	c.SetTheme(&t)
	c.Root().Background(p.sidebar)
	s := &a.settings

	ui.Scroll(c).Fill().Padding(24, 28, 32, 28).Gap(8).Children(func() {
		group := func(title string, rows func()) {
			ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(p.muted).Margin(14, 4, 4, 4)
			ui.Column(c).Radius(12).Background(p.bg).Border(1, p.hairline).Dividers(1, p.hairline).Children(rows)
		}
		row := func(label, desc string, control func()) {
			ui.Row(c).Padding(12, 16).Gap(16).AlignItems(ui.Center).MinHeight(48).Children(func() {
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
					if strings.HasPrefix(label, "当前版本") {
						b := ui.ButtonBase(c.Key("version-click")).FocusRing(false).Justify(ui.Start).AlignItems(ui.Start).Label(label).Children(func() {
							ui.Column(c).AlignItems(ui.Start).Gap(3).Children(func() {
								ui.Text(c, label).FontSize(13.5).FontWeight(500)
								if desc != "" {
									ui.Text(c, desc).FontSize(12).TextColor(p.muted).LineHeight(1.4)
								}
							})
						})
						if b.Clicked() {
							a.host.RecordVersionClick()
						}
					} else {
						ui.Text(c, label).FontSize(13.5).FontWeight(500)
						if desc != "" {
							ui.Text(c, desc).FontSize(12).TextColor(p.muted).LineHeight(1.4)
						}
					}
				})
				control()
			})
		}

		// Only on macOS: elsewhere the icon is always in the notification
		// area.
		if runtime.GOOS == "darwin" {
			group("通用", func() {
				row("应用图标", "显示在菜单栏时，关闭窗口后可从菜单栏图标打开", func() {
					labels := make([]string, len(appIcons))
					chosen := appIcons[0].label
					for i, ic := range appIcons {
						labels[i] = ic.label
						if ic.id == s.AppIcon {
							chosen = ic.label
						}
					}
					if ui.Select(c.Key("app-icon"), &chosen, labels).Label("应用图标").Width(150).Changed() {
						for _, ic := range appIcons {
							if ic.label == chosen {
								s.AppIcon = ic.id
							}
						}
						a.applyDesktopPreferences()

					}
				})
			})
		}

		// Only on macOS, the one with a notch player.
		if state.NotchAvailable {
			group("刘海播放器", func() {
				row("显示刘海播放器", "在屏幕顶部的刘海处显示迷你播放器，鼠标悬停即可展开", func() {
					if ui.Switch(c.Key("notch"), &s.Notch).Label("显示刘海播放器").Changed() {
						a.applyDesktopPreferences()
					}
				})
				row("播放时显示实时活动", "在刘海两侧显示封面与音频波形，可能遮挡刘海旁的菜单栏图标", func() {
					if ui.Switch(c.Key("activity"), &s.NotchActivity).Label("播放时显示实时活动").Disabled(!s.Notch).Changed() {
						a.applyDesktopPreferences()
					}
				})
				row("在无刘海的屏幕上常驻显示", "关闭时，将鼠标移至屏幕顶部中央即可唤出，不会遮挡菜单栏", func() {
					if ui.Switch(c.Key("virtual"), &s.NotchVirtual).Label("在无刘海的屏幕上常驻显示").Disabled(!s.Notch).Changed() {
						a.applyDesktopPreferences()
					}
				})
			})
		}

		group("播放", func() {
			row("音质", "无损与 Hi-Res 需要黑胶 VIP，没有时播放所能播放的最高音质；下一首歌曲开始生效", func() {
				labels := make([]string, len(qualityLevels))
				chosen := qualityLevels[2].label
				for i, q := range qualityLevels {
					labels[i] = q.label
					if q.level == s.Quality {
						chosen = q.label
					}
				}
				if ui.Select(c.Key("quality"), &chosen, labels).Label("音质").Width(150).Changed() {
					for _, q := range qualityLevels {
						if q.label == chosen {
							s.Quality = q.level
						}
					}
					a.savePlatformPreferences()
				}
			})
			row("间奏时显示热评", "前奏、间奏与尾奏时，歌词处换成这首歌的热门评论", func() {
				if ui.Switch(c.Key("hot-comments"), &s.HotComments).Label("间奏时显示热评").Changed() {
					a.savePlatformPreferences()
				}
			})
			row("音量均衡", "按网易云音乐测得的响度调整每首歌的音量，让歌曲听起来一样响", func() {
				if ui.Switch(c.Key("normalize"), &s.Normalize).Label("音量均衡").Changed() {
					a.savePlatformPreferences()
					a.pl.SetVolume(a.volume)
				}
			})
		})

		a.renderHifiSettings(c, group, func(key, label, desc string, control func()) {
			row(label, desc, control)
		})

		if a.captionsShown() {
			group("播客", func() { a.captionSettings(row, c) })
		}

		group("缓存", func() {
			row("歌曲缓存", "听过的歌曲保存在本机，再次播放无需等待，也不耗流量；"+cacheUsage(state.CacheUsed), func() {
				labels := make([]string, len(cacheLimits))
				chosen := cacheLimits[2].label
				for i, l := range cacheLimits {
					labels[i] = l.label
					if l.mb == s.CacheMB {
						chosen = l.label
					}
				}
				if ui.Select(c.Key("cache"), &chosen, labels).Label("歌曲缓存").Width(150).Changed() {
					for _, l := range cacheLimits {
						if l.label == chosen {
							s.CacheMB = l.mb
						}
					}
					a.applyDesktopPreferences()

				}
			})
			row("清除缓存", "删除保存在本机的全部歌曲", func() {
				if ui.Button(c.Key("clear-cache"), "清除").Disabled(state.CacheUsed <= 0).Clicked() {
					a.host.ClearAudioCache()
				}
			})
		})

		group("外观", func() {
			row("外观", "", func() {
				modes := []string{"system", "light", "dark"}
				i := 0
				for k, m := range modes {
					if m == s.Appearance {
						i = k
					}
				}
				if segmented(c.Key("appearance"), &i, "跟随系统", "浅色", "深色") {
					s.Appearance = modes[i]
					a.applyDesktopPreferences()
				}
			})
		})

		group("实验性功能", func() {
			row("进度条烟雾特效", "播放时进度条化作拖着烟雾的光点，随音乐起伏；会多用一些电量", func() {
				if ui.Switch(c.Key("smoke"), &s.DesktopPreferences.Smoke).Label("进度条烟雾特效").Changed() {
					a.applyDesktopPreferences()
				}
			})
			row("烟雾背景", "播放页的背景化作封面色调的烟雾，缓缓升起，随低音起伏；仍在打磨，会多用一些电量", func() {
				if ui.Switch(c.Key("fog"), &s.DesktopPreferences.Fog).Label("烟雾背景").Changed() {
					a.applyDesktopPreferences()
				}
			})
		})

		a.platformSettingsGroup(c, group, row)

		group("账号", func() {
			if a.profile == nil {
				row("未登录", "登录后，每日推荐与歌单将根据你的口味生成", func() {
					if ui.PrimaryButton(c.Key("login"), "扫码登录").Clicked() {
						a.showMain()
						a.openLogin()
						a.redraw()
					}
				})
				return
			}
			prof := a.profile
			ui.Row(c).Padding(12, 16).Gap(12).AlignItems(ui.Center).Children(func() {
				a.avatar(c, prof.AvatarURL, 36, p.press)
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, prof.Nickname).FontSize(13.5).FontWeight(600)
					ui.Text(c, "已登录网易云音乐").FontSize(12).TextColor(p.muted)
				})
				if ui.Button(c.Key("logout"), "退出登录").Clicked() {
					a.signOut()
				}
			})
		})

		if state.Updates {
			group("更新", func() {
				desc := "每天检查一次是否有新版本"
				if t := state.LastUpdateCheck; !t.IsZero() {
					desc += "；上次检查：" + commentTime(t, time.Now())
				}
				row("自动检查更新", desc, func() {
					on := state.UpdateChecks
					if ui.Switch(c.Key("update-checks"), &on).Label("自动检查更新").Changed() {
						a.host.SetUpdateChecks(on)
					}
				})
				row("自动下载并安装", "在后台下载新版本，下次打开时生效", func() {
					on := state.UpdateDownloads
					if ui.Switch(c.Key("update-downloads"), &on).Label("自动下载并安装").Disabled(!state.UpdateChecks).Changed() {
						a.host.SetUpdateDownloads(on)
					}
				})
				row("当前版本 "+mygo.App.Version(), versionDesc(state), func() {
					if ui.Button(c.Key("check-updates"), "检查更新").Clicked() {
						a.host.CheckForUpdates()
					}
				})
			})
		} else {
			group("关于", func() {
				row("当前版本 "+mygo.App.Version(), versionDesc(state), func() {})
			})
		}
	})
}

func (a *app) platformSettingsGroup(c *ui.Context, group func(string, func()), row func(string, string, func())) {
	infos := a.host.PlatformOptions()
	currentInInfos := false
	for _, info := range infos {
		if info.ID == a.host.CurrentPlatform() {
			currentInInfos = true
			break
		}
	}
	if !currentInInfos && a.host.CurrentPlatform() != "" {
		infos = append(infos, pluginapi.Info{ID: a.host.CurrentPlatform(), Name: "网易云音乐"})
	}
	if len(infos) == 0 {
		return
	}
	group("平台", func() {
		row("音乐平台", "切换后主页和平台设置使用独立的上下文", func() {
			labels := make([]string, len(infos))
			for i, info := range infos {
				labels[i] = info.Name
			}
			chosen := labels[0]
			for i, info := range infos {
				if info.ID == a.host.CurrentPlatform() {
					chosen = labels[i]
				}
			}
			if ui.Select(c.Key("platform"), &chosen, labels).Label("音乐平台").Width(150).Changed() {
				for _, info := range infos {
					if info.Name == chosen {
						a.host.SwitchPlatform(info.ID)
						break
					}
				}
			}
		})
	})
}

var qualityLevels = []struct{ level, label string }{
	{"standard", "标准（128k）"},
	{"higher", "较高（192k）"},
	{"exhigh", "极高（320k）"},
	{"lossless", "无损（FLAC）"},
	{"hires", "Hi-Res"},
}

// cacheLimits are the sizes the song cache may take, in megabytes.
var cacheLimits = []struct {
	mb    int
	label string
}{
	{0, "关闭"},
	{1024, "1 GB"},
	{2048, "2 GB"},
	{5120, "5 GB"},
	{10240, "10 GB"},
}

func (a *app) applyDesktopPreferences() {
	a.host.ApplyDesktopPreferences(a.settings.DesktopPreferences)
	a.closeUnavailableEqualizer()
}

func cacheUsage(n int64) string {
	switch {
	case n < 0:
		return "正在计算已用空间…"
	case n >= 1<<30:
		return fmt.Sprintf("已用 %.1f GB", float64(n)/(1<<30))
	default:
		return fmt.Sprintf("已用 %d MB", n>>20)
	}
}

func versionDesc(state pluginapi.DesktopSettings) string {
	if n := state.VersionClicksRemaining; n > 0 {
		if state.DeveloperMode {
			return fmt.Sprintf("再点击 %d 次关闭开发者模式", n)
		}
		return fmt.Sprintf("再点击 %d 次开启开发者模式", n)
	}
	if state.DeveloperMode {
		return "开发者模式已开启"
	}
	return ""
}

var appIcons = []struct{ id, label string }{
	{"dock-and-menubar", "程序坞与菜单栏"},
	{"dock", "仅程序坞"},
	{"menubar", "仅菜单栏"},
}

// showsTray tells whether the app's icon shows in the menu bar or the
// notification area.

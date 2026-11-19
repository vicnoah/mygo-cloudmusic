package qqmusic

import (
	"runtime"

	"github.com/egoist/mygo"
)

// setMenu gives the app its menu bar, in Chinese and named as the app
// is, with the settings in the app menu and the player's controls in a
// menu of their own.
func (h *Home) applicationMenu() *mygo.Menu {
	run := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			fn()
			h.host.Invalidate()
		}
	}
	name := "肥牛音乐"
	appMenu := []*mygo.MenuItem{
		{Role: mygo.RoleAbout, Label: "关于" + name},
		{Label: "检查更新…", Click: run(h.host.CheckForUpdates)},
		mygo.Separator(),
		{Label: "设置…", Accelerator: "CmdOrCtrl+,", Click: run(h.host.OpenSettings)},
		mygo.Separator(),
	}
	// Services and hiding are macOS's alone. Elsewhere MyGo leaves their
	// items out but keeps the separators around them, which then stack.
	if runtime.GOOS == "darwin" {
		appMenu = append(appMenu,
			&mygo.MenuItem{Role: mygo.RoleServices, Label: "服务"},
			mygo.Separator(),
			&mygo.MenuItem{Role: mygo.RoleHide, Label: "隐藏" + name},
			&mygo.MenuItem{Role: mygo.RoleHideOthers, Label: "隐藏其他"},
			&mygo.MenuItem{Role: mygo.RoleUnhide, Label: "全部显示"},
			mygo.Separator(),
		)
	}
	appMenu = append(appMenu, &mygo.MenuItem{Role: mygo.RoleQuit, Label: "退出" + name})
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: name, Submenu: appMenu},
		{Label: "文件", Submenu: []*mygo.MenuItem{
			{Label: "新建歌单…", Accelerator: "CmdOrCtrl+N", Click: run(func() {
				h.host.ShowMain()
				h.nowPlaying = false
				if h.state.Profile() == nil {
					h.openLogin()
					return
				}
				h.openNewPlaylist()
			})},
		}},
		{Label: "编辑", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleUndo, Label: "撤销"},
			{Role: mygo.RoleRedo, Label: "重做"},
			mygo.Separator(),
			{Role: mygo.RoleCut, Label: "剪切"},
			{Role: mygo.RoleCopy, Label: "拷贝"},
			{Role: mygo.RolePaste, Label: "粘贴"},
			{Role: mygo.RoleSelectAll, Label: "全选"},
		}},
		{Label: "控制", Submenu: []*mygo.MenuItem{
			{Label: "播放/暂停", Click: run(func() { h.host.Playback().TogglePlay() })},
			{Label: "下一首", Accelerator: "CmdOrCtrl+Right", Click: run(func() { h.host.Playback().Next() })},
			{Label: "上一首", Accelerator: "CmdOrCtrl+Left", Click: run(func() { h.host.Playback().Prev() })},
			mygo.Separator(),
			{Label: "喜欢/取消喜欢", Accelerator: "CmdOrCtrl+D", Click: run(h.likeCurrent)},
			mygo.Separator(),
			{Label: "随机播放", Click: run(func() { h.host.Playback().ToggleShuffle() })},
			{Label: "切换循环模式", Click: run(func() { h.host.Playback().CycleRepeat() })},
			mygo.Separator(),
			{Label: "正在播放", Accelerator: "CmdOrCtrl+L", Click: run(func() {
				h.host.ShowMain()
				h.nowPlaying = !h.nowPlaying && h.host.Playback().Current() != nil
			})},
		}},
		{Label: "窗口", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleMinimize, Label: "最小化"},
			{Role: mygo.RoleZoom, Label: "缩放"},
			{Role: mygo.RoleClose, Label: "关闭"},
			mygo.Separator(),
			{Label: name, Accelerator: "CmdOrCtrl+0", Click: run(h.host.ShowMain)},
			{Role: mygo.RoleFront, Label: "前置全部窗口"},
		}},
	})
}

// trayMenu is the icon's menu: the window, the player's controls,
// settings, updates, and quitting.
func (h *Home) trayMenu() *mygo.Menu {
	run := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			fn()
			h.host.Invalidate()
		}
	}
	// In front, as from the menu bar alone, where the app may not be.
	front := func(fn func()) func() {
		return func() {
			mygo.App.Focus()
			fn()
		}
	}
	name := "肥牛音乐"
	lockLabel := "锁定桌面歌词"
	if h.host.DesktopLyricsLocked() {
		lockLabel = "解锁桌面歌词"
	}
	lockItem := &mygo.MenuItem{Label: lockLabel, Click: run(h.host.ToggleDesktopLyricsLock)}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "打开" + name, Click: run(h.host.ShowMain)},
		{Label: "桌面歌词", Click: run(h.host.ToggleDesktopLyrics)},
		lockItem,
		mygo.Separator(),
		{Label: "播放/暂停", Click: run(func() { h.host.Playback().TogglePlay() })},
		{Label: "下一首", Click: run(func() { h.host.Playback().Next() })},
		{Label: "上一首", Click: run(func() { h.host.Playback().Prev() })},
		{Label: "喜欢/取消喜欢", Click: run(h.likeCurrent)},
		mygo.Separator(),
		{Label: "设置…", Click: run(front(h.host.OpenSettings))},
		{Label: "检查更新…", Click: run(front(h.host.CheckForUpdates))},
		mygo.Separator(),
		{Label: "退出" + name, Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
}

func (h *Home) likeCurrent() {
	if track := h.currentTrack(); track != nil {
		h.favorite(*track, !h.state.Liked(track.ID))
	}
}

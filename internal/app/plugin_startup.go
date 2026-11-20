package app

import (
	"errors"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/waygu-music-plugin/sdk/pluginapi"
)

type pluginStartup struct {
	failed  pluginapi.ID
	failure string
	timer   *time.Timer
	notice  *mygo.Window
}

func (a *app) startupWindowOptions() mygo.WindowOptions {
	options := a.windowOptions(pluginapi.MainWindow)
	if a.managedPlugins != nil && (a.pluginLink.repo != "" || string(a.platformID) != a.settings.Platform) {
		// Keep the host window available for dispatch without mounting a
		// platform until the requested platform is ready.
		a.platformID = ""
		options.Hidden = true
	} else {
		a.activatePlatformView()
	}
	return options
}

func (a *app) resumeStartupPlatform() {
	if a.quitting || a.platformID != "" || a.pluginsWin != nil || a.pluginStartup.failure != "" {
		return
	}
	id := pluginapi.ID(a.settings.Platform)
	if a.hasPlatform(id) {
		a.commitPlatform(id, false)
		return
	}
	if local := a.localPlugins[id]; local != nil && local.program.Path != "" {
		a.startLocalPlugin(id)
		return
	}
	a.failPluginStartup(id, errors.New("找不到上次使用的插件入口"))
}

func (a *app) startupPluginName(id pluginapi.ID) string {
	if local := a.localPlugins[id]; local != nil {
		if pkg := a.managedPlugins[local.program.Identity.Package]; pkg != nil && pkg.name != "" {
			return pkg.name
		}
	}
	return string(id)
}

func (a *app) scheduleStartupNotice(local *localPlugin, request uint64) {
	if a.pluginsWin != nil {
		return
	}
	a.pluginStartup.timer = time.AfterFunc(650*time.Millisecond, func() {
		mygo.RunOnMain(func() {
			if a.quitting || a.platformID != "" || a.pluginsWin != nil || local.pending == nil || local.request != request {
				return
			}
			name := a.startupPluginName(local.entry.ID)
			a.pluginStartup.notice = mygo.NewWindow(mygo.WindowOptions{
				Title: appName, Width: 350, Height: 115, UseContentSize: true,
				DisableResize: true, DisableMaximize: true, DisableMinimize: true, DisableClose: true,
				Content: ui.View(func(c *ui.Context) {
					ui.Column(c).Fill().Padding(22).Gap(8).Children(func() {
						ui.Text(c, "正在启动 "+name).FontSize(17).FontWeight(650).Wrap()
						ui.Text(c, "正在准备插件窗口…").FontSize(13).TextColor(c.Theme().TextMuted)
					})
				}),
			})
			_ = a.pluginStartup.notice.SetIcon(desktopApplicationIcon())
			a.syncDock()
		})
	})
}

func (a *app) dismissStartupNotice() {
	if timer := a.pluginStartup.timer; timer != nil {
		timer.Stop()
		a.pluginStartup.timer = nil
	}
	if notice := a.pluginStartup.notice; notice != nil {
		a.pluginStartup.notice = nil
		notice.Destroy()
		a.syncDock()
	}
}

func (a *app) failPluginStartup(id pluginapi.ID, err error) {
	a.dismissStartupNotice()
	a.pluginStartup.failed = id
	a.pluginStartup.failure = "无法启动 " + a.startupPluginName(id) + "：" + err.Error()
	a.openPluginManager()
	a.redraw()
}

func (a *app) pluginStartupRecovery(c *ui.Context, p pluginManagerColors) {
	if a.pluginStartup.failure == "" {
		return
	}
	id := a.pluginStartup.failed
	local := a.localPlugins[id]
	canRetry := local != nil && local.program.Path != "" && local.pending == nil
	ui.Column(c).FillWidth().Background(p.panel).Border(1, p.border).Radius(12).Padding(12).Gap(8).Children(func() {
		ui.Text(c, a.pluginStartup.failure).Wrap().FontSize(13).TextColor(c.Theme().Danger)
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "重试").Disabled(!canRetry).Clicked() {
				a.startLocalPlugin(id)
			}
			if ui.Button(c, "使用飞牛音乐").Clicked() {
				a.setPlatform(a.defaultPlatform())
			}
		})
	})
}

package app

import (
	"context"
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginregistry"
)

const pluginUpdateInterval = 24 * time.Hour

type pluginUpdateReminder struct {
	Version string
	Until   time.Time
}

type pluginUpdates struct {
	ready  *pluginProcess
	cancel context.CancelFunc
	notice *mygo.Window
}

type pluginUpdateTarget struct {
	process *pluginProcess
	pkg     *managedPlugin
	path    string
	version string
	entry   string
	source  pluginregistry.Source
}

type pluginUpdateOffer struct {
	target pluginUpdateTarget
	review pluginregistry.GitHubReview
}

func (a *app) pluginUpdatesEnabled() bool {
	return a.updates && !a.quitting && updateChecks()
}

func (a *app) currentPluginUpdateTarget(process *pluginProcess) pluginUpdateTarget {
	if process == nil || process != a.pluginUpdates.ready || process != a.remoteActive() || process.owner != a.platformID ||
		!a.pluginUpdatesEnabled() {
		return pluginUpdateTarget{}
	}
	local := a.localPlugins[process.owner]
	if local == nil {
		return pluginUpdateTarget{}
	}
	pkg := a.managedPlugins[local.program.Identity.Package]
	if pkg == nil || pkg.installed.Path == "" || pkg.installed.Source == "" || a.pluginPackageBusy(pkg) {
		return pluginUpdateTarget{}
	}
	source, err := a.pluginInstallSource(pkg.id, pkg.installed.Source)
	if err != nil {
		return pluginUpdateTarget{}
	}
	return pluginUpdateTarget{
		process: process, pkg: pkg, path: pkg.installed.Path, version: pkg.installed.Version.Version,
		entry: local.program.Identity.Entry, source: source,
	}
}

func (a *app) syncPluginUpdateChecks() {
	state := &a.pluginUpdates
	process, registry := state.ready, a.pluginRegistry
	if process == nil || process != a.remoteActive() || registry == nil || a.pluginContext == nil || !a.pluginUpdatesEnabled() {
		a.stopPluginUpdateChecks()
		return
	}
	if state.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(a.pluginContext)
	state.cancel = cancel
	a.pluginWorkers.Go(func() {
		delay := 2 * time.Second
		for {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			var target pluginUpdateTarget
			mygo.RunOnMain(func() {
				if ctx.Err() == nil {
					target = a.currentPluginUpdateTarget(process)
				}
			})
			if ctx.Err() != nil {
				return
			}
			if target.pkg == nil {
				// An update or rollback can be underway in the plugin manager.
				delay = time.Minute
				continue
			}
			delay = pluginUpdateInterval
			request, done := context.WithTimeout(ctx, 20*time.Second)
			review, err := registry.ReviewUpdate(request, target.source, target.entry)
			done()
			if err != nil || review.Release.Version == "" || ctx.Err() != nil {
				continue
			}
			mygo.RunOnMain(func() {
				if ctx.Err() == nil {
					delay = a.receivePluginUpdate(pluginUpdateOffer{target, review}, time.Now())
				}
			})
		}
	})
}

func (a *app) stopPluginUpdateChecks() {
	state := &a.pluginUpdates
	if state.cancel != nil {
		state.cancel()
		state.cancel = nil
	}
	a.closePluginUpdateNotice()
}

func (a *app) validPluginUpdate(offer pluginUpdateOffer) bool {
	target := offer.target
	current := a.currentPluginUpdateTarget(target.process)
	return current.pkg != nil && current.pkg == target.pkg && current.path == target.path &&
		current.version == target.version && current.source.Catalog == offer.review.Source.Catalog &&
		current.source.Package == offer.review.Source.Package
}

func (a *app) receivePluginUpdate(offer pluginUpdateOffer, now time.Time) time.Duration {
	if !a.validPluginUpdate(offer) {
		return pluginUpdateInterval
	}
	reminder := a.settings.PluginUpdateReminders[offer.target.pkg.id]
	if until := reminder.Until.Sub(now); reminder.Version == offer.review.Release.Version && until > 0 {
		return min(until, pluginUpdateInterval)
	}
	if a.pluginUpdates.notice == nil {
		a.showPluginUpdateNotice(offer)
	}
	return pluginUpdateInterval
}

func (a *app) showPluginUpdateNotice(offer pluginUpdateOffer) {
	name := offer.target.pkg.name
	if name == "" {
		name = offer.target.pkg.id
	}
	win := mygo.NewWindow(mygo.WindowOptions{
		Title: "插件更新", Width: 435, Height: 190, UseContentSize: true,
		DisableResize: true, DisableMaximize: true, DisableMinimize: true,
		Content: ui.View(func(c *ui.Context) {
			pluginUpdateNoticeView(c, name, offer.target.version, offer.review.Release.Version, func() {
				a.confirmPluginUpdate(offer)
			}, func() {
				a.remindPluginUpdateLater(offer, time.Now())
			})
		}),
	})
	a.pluginUpdates.notice = win
	_ = win.SetIcon(desktopApplicationIcon())
	win.SetMenu(mygo.NewMenu(nil))
	win.OnClosed(func() {
		if a.pluginUpdates.notice == win {
			a.pluginUpdates.notice = nil
			a.remindPluginUpdateLater(offer, time.Now())
			a.syncDock()
		}
	})
	a.syncDock()
	mygo.App.Focus()
	win.Focus()
}

func pluginUpdateNoticeView(c *ui.Context, name, current, latest string, update, later func()) {
	ui.Column(c).FillWidth().Padding(22).Gap(10).Children(func() {
		ui.Text(c, name+" 有新版本").FontSize(19).FontWeight(650).Wrap()
		ui.Text(c, current+" → "+latest).FontSize(14).Wrap()
		ui.Text(c, "更新完成后，重启应用即可使用新版本。").TextColor(c.Theme().TextMuted).FontSize(13).Wrap()
		ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "稍后").Clicked() {
				later()
			}
			if ui.PrimaryButton(c, "立即更新").Clicked() {
				update()
			}
		})
	})
}

func (a *app) remindPluginUpdateLater(offer pluginUpdateOffer, now time.Time) {
	a.closePluginUpdateNotice()
	if !a.validPluginUpdate(offer) {
		return
	}
	if a.settings.PluginUpdateReminders == nil {
		a.settings.PluginUpdateReminders = make(map[string]pluginUpdateReminder)
	}
	a.settings.PluginUpdateReminders[offer.target.pkg.id] = pluginUpdateReminder{
		Version: offer.review.Release.Version, Until: now.Add(pluginUpdateInterval),
	}
	if err := a.settings.save(); err != nil {
		log.Printf("save plugin update reminder: %v", err)
	}
}

func (a *app) confirmPluginUpdate(offer pluginUpdateOffer) {
	a.closePluginUpdateNotice()
	if !a.validPluginUpdate(offer) {
		return
	}
	pkg := offer.target.pkg
	pkg.source = offer.review.Source.Catalog
	a.catalogPackage = pkg.id
	a.openPluginManager()
	a.changePlugin(pkg, "install", &offer.review.Source)
}

func (a *app) closePluginUpdateNotice() {
	if win := a.pluginUpdates.notice; win != nil {
		a.pluginUpdates.notice = nil
		win.Destroy()
		a.syncDock()
	}
}

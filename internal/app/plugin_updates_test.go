package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/waygu-music-plugin/sdk/platformrpc"
	"github.com/waygu-music-plugin/sdk/pluginapi"

	"waygu-music/internal/pluginregistry"
)

func pluginUpdateTestApp(t *testing.T) (*app, pluginUpdateOffer) {
	t.Helper()
	a := newTestApp()
	a.updates = true
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	const pkgID, entry, catalog = "library", "music", "https://example.invalid/catalog.json"
	process := &pluginProcess{owner: pluginapi.ID(pkgID + "/" + entry)}
	a.platformID = process.owner
	a.activeRemote.Store(process)
	a.pluginUpdates.ready = process
	a.localPlugins = map[pluginapi.ID]*localPlugin{
		process.owner: {program: platformrpc.Program{Identity: platformrpc.Identity{Package: pkgID, Entry: entry}}},
	}
	a.managedPlugins = map[string]*managedPlugin{
		pkgID: {id: pkgID, installed: pluginregistry.Installed{
			Source: catalog, Program: pluginregistry.Program{Path: "/library/v1", Version: pluginregistry.Version{Version: "1.0.0"}},
		}},
	}
	target := a.currentPluginUpdateTarget(process)
	if target.pkg == nil {
		t.Fatal("the fixture did not select the active external plugin")
	}
	return a, pluginUpdateOffer{target: target, review: pluginregistry.GitHubReview{
		Source: target.source, Release: pluginregistry.Release{Version: "1.1.0"},
	}}
}

// Disabling automatic checks cancels pending work and discards an update result already on its way back.
func TestPluginUpdateOptOutDiscardsPendingOffer(t *testing.T) {
	on := true
	oldChecks, oldSet := updateChecks, setUpdateChecks
	updateChecks = func() bool { return on }
	setUpdateChecks = func(value bool) { on = value }
	t.Cleanup(func() { updateChecks, setUpdateChecks = oldChecks, oldSet })
	a, offer := pluginUpdateTestApp(t)
	check, cancel := context.WithCancel(t.Context())
	a.pluginUpdates.cancel = cancel
	(platformHost{a: a}).SetUpdateChecks(false)
	if check.Err() != context.Canceled || a.pluginUpdates.cancel != nil {
		t.Fatal("turning automatic checks off left the check running")
	}
	a.receivePluginUpdate(offer, time.Unix(1000, 0))
	if a.pluginUpdates.notice != nil || a.validPluginUpdate(offer) {
		t.Fatal("the canceled check produced an offer")
	}
}

// Results for an inactive process or a version replaced in the manager cannot offer an update.
func TestPluginUpdateOfferFollowsActivationAndInstallation(t *testing.T) {
	old := updateChecks
	updateChecks = func() bool { return true }
	t.Cleanup(func() { updateChecks = old })
	a, offer := pluginUpdateTestApp(t)
	process := offer.target.process
	a.activeRemote.Store(&pluginProcess{owner: process.owner})
	if a.validPluginUpdate(offer) {
		t.Fatal("an outdated activation could show its update")
	}
	a.activeRemote.Store(process)
	offer.target.pkg.installed.Path = "/library/v2"
	if a.validPluginUpdate(offer) {
		t.Fatal("an outdated installation could show its update")
	}
}

// Later saves a day-long reminder for the offered version and suppresses repeat offers for that version.
func TestPluginUpdateLaterPersistsReminder(t *testing.T) {
	old := updateChecks
	updateChecks = func() bool { return true }
	t.Cleanup(func() { updateChecks = old })
	a, offer := pluginUpdateTestApp(t)
	now := time.Unix(1000, 0)
	view := ui.NewTester(func(c *ui.Context) {
		pluginUpdateNoticeView(c, "Library", offer.target.version, offer.review.Release.Version,
			func() { t.Fatal("Later confirmed the update") },
			func() { a.remindPluginUpdateLater(offer, now) })
	}, 435, 190)
	if err := view.Click("稍后"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	reminder := saved.PluginUpdateReminders[offer.target.pkg.id]
	if reminder.Version != "1.1.0" || !reminder.Until.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("saved the wrong reminder: %+v", reminder)
	}
	a.settings.PluginUpdateReminders = saved.PluginUpdateReminders
	if delay := a.receivePluginUpdate(offer, now.Add(time.Hour)); delay != 23*time.Hour || a.pluginUpdates.notice != nil {
		t.Fatalf("the reminder reopened too early or scheduled the wrong delay: %v", delay)
	}
}

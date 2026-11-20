package app

import (
	"net/url"
	"testing"

	"github.com/egoist/mygo/ui"

	"waygu-music/internal/pluginregistry"
)

// Windows added a root slash to the published link, which used to prevent the installer from opening.
func TestPluginLinkAcceptsWindowsRootSlash(t *testing.T) {
	for _, repository := range []string{"waygu-music-plugin/netease", "listener/library"} {
		for _, path := range []string{"", "/"} {
			raw := "wagyu-music://install-plugin" + path + "?repo=" + url.QueryEscape(repository)
			a := newTestApp()
			a.openPluginLink(raw, "wagyu-music")
			if a.pluginLink.repo != repository || a.pluginLink.message != "" {
				t.Fatalf("%q did not queue %q: %+v", raw, repository, a.pluginLink)
			}
		}
	}
}

// A malformed installation link is reported before startup and cannot be used as an install candidate.
func TestInvalidPluginLinkBeforeMainWindow(t *testing.T) {
	for _, raw := range []string{
		"wagyu-music://install-plugin//?repo=listener/library",
		"wagyu-music://install-plugin/%2F?repo=listener/library",
		"wagyu-music://install-plugin/?repo=listener%ZZlibrary",
		"wagyu-music://install-plugin/?repo=listener/library&repo=listener/other",
	} {
		a := newTestApp()
		a.openPluginLink(raw, "wagyu-music")
		if a.pluginLink.message == "" || a.pluginLink.repo != "" {
			t.Fatalf("%q did not queue only an error: %+v", raw, a.pluginLink)
		}
	}
}

// The browser can queue a repository during launch, and an extra untrusted URI parameter cannot replace it.
func TestPluginLinkBeforeMainWindow(t *testing.T) {
	a := newTestApp()
	a.openPluginLink("wagyu-music://install-plugin?repo=Listener%2FLibrary", "wagyu-music")
	if got := a.pluginLink.repo; got != "listener/library" {
		t.Fatalf("queued repository = %q", got)
	}
	for _, raw := range []string{
		"wagyu-music://install-plugin?repo=listener/a&repo=listener/b",
		"wagyu-music://install-plugin?repo=listener/../library",
		"wagyu-music://install-plugin?repo=listener/library&url=https://example.com/archive",
		"wagyu-music://fake@install-plugin?repo=listener/library",
		"mygo-music://install-plugin?repo=listener/library",
	} {
		a.openPluginLink(raw, "wagyu-music")
	}
	if got := a.pluginLink.repo; got != "listener/library" {
		t.Fatalf("invalid URL replaced the queued repository: %q", got)
	}
}

// Cancelling the installation confirmation leaves the local plugin list untouched.
func TestCancelPluginLink(t *testing.T) {
	a := newTestApp()
	a.managedPlugins = make(map[string]*managedPlugin)
	a.pluginLink = pluginLinkInstaller{
		open: true, repo: "listener/library",
		review: pluginregistry.GitHubReview{
			Source:  pluginregistry.Source{Package: "listener-library"},
			Release: pluginregistry.Release{Name: "Library", Version: "1.0.0"},
		},
	}
	view := ui.NewTester(a.pluginManagerView, 660, 485)
	if err := view.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if a.pluginLink.open || a.pluginLink.repo != "" || len(a.managedPlugins) != 0 {
		t.Fatal("cancelling the installation changed the installed list or retained the candidate")
	}
}

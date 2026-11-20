package app

import (
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/waygu-music-plugin/sdk/bridge"
	"github.com/waygu-music-plugin/sdk/platformrpc"
	"github.com/waygu-music-plugin/sdk/pluginapi"
	"github.com/waygu-music-plugin/sdk/protocol"

	"waygu-music/internal/platformcatalog"
	"waygu-music/internal/pluginregistry"
)

type managedPlugin struct {
	id, name, source string
	installed        pluginregistry.Installed
	busy             bool
	message          string
}

func (a *app) loadPluginRegistry() {
	if a.managedPlugins == nil {
		return
	}
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err == nil {
		a.pluginRegistry, err = pluginregistry.New(filepath.Join(dir, "plugin-registry"), nil)
	}
	if err != nil {
		a.pluginCatalogError = err.Error()
		return
	}
	installed, err := a.pluginRegistry.Installed()
	if err != nil {
		a.pluginCatalogError = err.Error()
		return
	}
	for id, item := range installed {
		if a.builtinPlugin(id) {
			continue
		}
		pkg := &managedPlugin{id: id, name: item.Name, source: item.Source}
		a.managedPlugins[id] = pkg
		a.setInstalled(pkg, item)
	}
}

func (a *app) builtinPlugin(id string) bool {
	return slices.ContainsFunc(a.platformRegistrations, func(entry pluginapi.Registration) bool {
		return string(entry.ID) == id
	})
}

// Alpha saved the original provider namespaces and session IDs. New
// packages use the package and entry together to keep their state separate.
func externalRegistration(pkg, entry string) pluginapi.Registration {
	registration := pluginapi.Registration{Info: pluginapi.Info{ID: pluginapi.ID(pkg + "/" + entry)}}
	switch {
	case pkg == "subsonic" && (entry == "subsonic" || entry == "navidrome" || entry == "jellyfin" || entry == "gonic"),
		pkg == entry && (pkg == "songloft" || pkg == "qqmusic"):
		registration.ID = pluginapi.ID(entry)
		registration.LegacySettingsKey = entry
	case pkg == "netease" && entry == pkg:
		registration.ID = pluginapi.ID(entry)
		registration.LegacySettingsFields = []string{"quality", "normalize", "captions", "hotComments"}
	}
	return registration
}

func (a *app) setInstalled(pkg *managedPlugin, item pluginregistry.Installed) {
	pkg.installed = item
	if item.Name != "" {
		pkg.name = item.Name
	}
	for id, local := range a.localPlugins {
		if local.program.Identity.Package == pkg.id && !slices.Contains(item.Platforms, local.program.Identity.Entry) {
			if id == a.platformID {
				local.program.Path = ""
			} else {
				delete(a.localPlugins, id)
			}
		}
	}
	for _, entry := range item.Platforms {
		registration := externalRegistration(pkg.id, entry)
		local := a.localPlugins[registration.ID]
		if local == nil {
			local = &localPlugin{
				entry: registration, adapter: &processAdapter{id: pluginapi.ID(entry)},
				program: platformrpc.Program{Identity: platformrpc.Identity{Package: pkg.id, Entry: entry}},
			}
			a.localPlugins[registration.ID] = local
		}
		local.program.Path, local.program.SHA256 = item.Path, item.SHA256
	}
}

func (a *app) preferredPluginEntry(pkg *managedPlugin) pluginapi.ID {
	for _, entry := range pkg.installed.Platforms {
		id := externalRegistration(pkg.id, entry).ID
		if string(id) == a.settings.Platform {
			return id
		}
	}
	if remembered := a.settings.LastPluginEntries[pkg.id]; slices.Contains(pkg.installed.Platforms, remembered) {
		return externalRegistration(pkg.id, remembered).ID
	}
	if slices.Contains(pkg.installed.Platforms, pkg.id) {
		return externalRegistration(pkg.id, pkg.id).ID
	}
	if len(pkg.installed.Platforms) != 0 {
		return externalRegistration(pkg.id, pkg.installed.Platforms[0]).ID
	}
	return ""
}

func (a *app) refreshPluginCatalog() {
	if a.pluginRegistry == nil {
		return
	}
	a.pluginCatalogRequest++
	request := a.pluginCatalogRequest
	a.pluginCatalogBusy, a.pluginCatalogError = true, ""
	sources := []string{platformcatalog.OfficialCatalog}
	if additional := a.settings.PluginCatalogURL; additional != "" && additional != sources[0] {
		sources = append(sources, additional)
	}
	for _, config := range a.settings.PluginCatalogs {
		if config.URL != "" && !slices.Contains(sources, config.URL) {
			sources = append(sources, config.URL)
		}
	}
	useProxy := !a.settings.DisablePluginGitHubProxy
	a.pluginWorkers.Go(func() {
		type result struct {
			source string
			pluginregistry.Catalog
		}
		var results []result
		var message string
		for _, source := range sources {
			ctx, cancel := context.WithTimeout(a.pluginContext, 12*time.Second)
			catalog, err := a.pluginRegistry.Catalog(ctx, source, useProxy)
			cancel()
			if a.pluginContext.Err() != nil {
				return
			}
			if err != nil {
				message = "无法刷新插件目录：" + err.Error()
				continue
			}
			results = append(results, result{source: source, Catalog: catalog})
		}
		mygo.RunOnMain(func() {
			if a.quitting || request != a.pluginCatalogRequest {
				return
			}
			a.pluginCatalogBusy, a.pluginCatalogError = false, message
			available := make(map[string]bool)
			for _, result := range results {
				for _, entry := range result.Packages {
					if a.builtinPlugin(entry.Package) || available[entry.Package] {
						continue
					}
					pkg := a.managedPlugins[entry.Package]
					expected := a.settings.PluginCatalogs[entry.Package].URL
					if pkg != nil && pkg.installed.Path != "" {
						expected = pkg.installed.Source
					}
					if expected != "" && expected != result.source {
						continue
					}
					available[entry.Package] = true
					if pkg == nil {
						pkg = &managedPlugin{id: entry.Package}
						a.managedPlugins[entry.Package] = pkg
					}
					if pkg.installed.Name == "" && entry.Name != "" {
						pkg.name = entry.Name
					}
					pkg.source = result.source
				}
			}
			if message == "" {
				for id, pkg := range a.managedPlugins {
					if !available[id] && pkg.installed.Path == "" && !pkg.busy {
						delete(a.managedPlugins, id)
					}
				}
			}
			a.redraw()
		})
	})
}

func checkedPluginCatalog(raw string) (string, error) {
	catalog := strings.TrimSpace(raw)
	u, err := url.Parse(catalog)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return "", errors.New("请输入有效的 HTTPS 插件目录地址")
	}
	return catalog, nil
}

func (a *app) commitPluginSource(catalog string) {
	editor := &a.pluginSource
	settings := a.settings
	settings.PluginCatalogURL = catalog
	settings.DisablePluginGitHubProxy = !editor.useGitHubProxy
	if err := settings.save(); err != nil {
		editor.message = err.Error()
		return
	}
	a.settings.PluginCatalogURL = settings.PluginCatalogURL
	a.settings.DisablePluginGitHubProxy = settings.DisablePluginGitHubProxy
	editor.open = false
	a.refreshPluginCatalog()
}

func (a *app) changeInstalledPlugin(pkg *managedPlugin, action string) {
	if pkg == nil || a.pluginRegistry == nil || pkg.busy {
		return
	}
	for _, local := range a.localPlugins {
		if local.program.Identity.Package == pkg.id && local.pending != nil {
			return
		}
	}
	source := pluginregistry.Source{Package: pkg.id, Catalog: pkg.source, UseGitHubProxy: !a.settings.DisablePluginGitHubProxy}
	if action == "install" {
		pinned := ""
		if source.Catalog == platformcatalog.OfficialCatalog {
			pinned = platformcatalog.OfficialPublisherKey
		} else if config := a.settings.PluginCatalogs[pkg.id]; config.URL == source.Catalog {
			pinned = config.LegacyKey
		}
		var err error
		source.TrustedKey, err = base64.StdEncoding.DecodeString(pinned)
		if err != nil {
			pkg.message = "无法读取此插件的发布者信息"
			a.redraw()
			return
		}
	}
	pkg.busy, pkg.message = true, "正在验证插件…"
	a.redraw()
	a.pluginWorkers.Go(func() {
		ctx, cancel := context.WithTimeout(a.pluginContext, 2*time.Minute)
		defer cancel()
		probe := func(ctx context.Context, p pluginregistry.Program) error {
			for _, entry := range p.Platforms {
				if err := a.probeInstalledPlugin(ctx, pkg.id, entry, p); err != nil {
					return err
				}
			}
			return nil
		}
		var err error
		switch action {
		case "install":
			_, err = a.pluginRegistry.Install(ctx, source, probe)
		case "rollback":
			_, err = a.pluginRegistry.Rollback(ctx, pkg.id, probe)
		case "uninstall":
			err = a.pluginRegistry.Unregister(pkg.id)
		}
		var item pluginregistry.Installed
		if err == nil && action != "uninstall" {
			var installed map[string]pluginregistry.Installed
			installed, err = a.pluginRegistry.Installed()
			item = installed[pkg.id]
		}
		if a.pluginContext.Err() != nil {
			return
		}
		mygo.RunOnMain(func() {
			pkg.busy = false
			if a.quitting {
				return
			}
			defer a.redraw()
			if err != nil {
				pkg.message = err.Error()
				return
			}
			if action == "uninstall" {
				if current := a.localPlugins[a.platformID]; current != nil && current.program.Identity.Package == pkg.id {
					a.commitPlatform(a.defaultPlatform(), true)
				}
				a.setInstalled(pkg, pluginregistry.Installed{})
				pkg.message = "已卸载，账号和设置已保留"
				return
			}
			a.setInstalled(pkg, item)
			pkg.message = "已验证 " + item.Version.Version
			if current := a.localPlugins[a.platformID]; current != nil && current.program.Identity.Package == pkg.id {
				pkg.message += "，下次启用时生效"
			}
		})
	})
}

func (a *app) probeInstalledPlugin(ctx context.Context, pkg, entryID string, p pluginregistry.Program) error {
	entry := externalRegistration(pkg, entryID)
	identity := platformrpc.Identity{Package: pkg, Entry: entryID}
	if !slices.Contains(p.Platforms, identity.Entry) {
		return errors.New("release does not include the selected platform entry")
	}
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return err
	}
	input := PluginPrepare{}
	mygo.RunOnMain(func() {
		input.Settings, err = (pluginSettingsStore{a: a, entry: entry}).raw()
		input.Appearance = a.settings.Appearance
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	stateRoot := filepath.Join(dir, "plugin-ui")
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		return err
	}
	stateDir, err := os.MkdirTemp(stateRoot, ".verify-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stateDir)
	process, err := platformrpc.Start(ctx, platformrpc.Program{
		Path: p.Path, SHA256: p.SHA256, Identity: identity,
		StateDir: stateDir, CredentialService: pluginCredentialService(), ApplicationID: desktopApplicationID(),
	}, hostOffer, func(_ string) any { return installProbeHost{} })
	if err != nil {
		return err
	}
	defer process.Close()
	input.Scope = platformrpc.Scope{Connection: process.Connection(), Activation: 1}
	var out PluginPrepared
	if err := process.Call(ctx, "Prepare", input.Wire(), &out); err != nil {
		return err
	}
	if err := out.Failure.Err(); err != nil {
		return err
	}
	return bridge.ValidateWindowActions(out)
}

type installProbeHost struct{}

func (installProbeHost) Use(_ protocol.PluginHostCall, out *protocol.PluginHostReply) error {
	out.Failure = platformrpc.Failure{Code: "not-active"}
	return nil
}

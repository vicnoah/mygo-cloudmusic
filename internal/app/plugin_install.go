package app

import (
	"context"
	"encoding/base64"
	"errors"
	"maps"
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

func (a *app) loadPluginRegistry() {
	catalogs := a.catalogRepresentatives()
	if len(catalogs) == 0 {
		return
	}
	if preferred := a.localPlugins[pluginapi.ID(a.settings.Platform)]; preferred != nil {
		a.catalogPackage = preferred.program.Identity.Package
	}
	dir, err := mygo.App.Path(mygo.PathUserData)
	var registry *pluginregistry.Registry
	if err == nil {
		registry, err = pluginregistry.New(filepath.Join(dir, "plugin-registry"), nil)
	}
	if err != nil {
		for _, local := range catalogs {
			local.message = err.Error()
		}
		return
	}
	for _, local := range catalogs {
		id := local.program.Identity.Package
		source, configured := a.settings.PluginCatalogs[id]
		local.catalog = source.URL
		if !configured {
			local.catalog = platformcatalog.OfficialCatalog
		}
		local.registry = registry
		program, installed, err := registry.Current(id)
		if err != nil {
			local.message = err.Error()
			continue
		}
		if installed {
			a.setInstalled(id, program)
		}
	}
}

func (a *app) catalogRepresentatives() []*localPlugin {
	var packages []*localPlugin
	seen := make(map[string]bool)
	for _, registration := range a.platformRegistrations {
		if local := a.localPlugins[registration.ID]; local != nil {
			pkg := local.program.Identity.Package
			if !seen[pkg] {
				packages = append(packages, local)
				seen[pkg] = true
			}
		}
	}
	return packages
}

func (a *app) setInstalled(pkg string, p pluginregistry.Program) {
	for id, local := range a.localPlugins {
		if local.program.Identity.Package != pkg {
			continue
		}
		local.program.Path, local.version = "", ""
		if slices.Contains(p.Platforms, string(id)) {
			local.program.Path, local.program.SHA256 = p.Path, p.SHA256
			local.version = p.Version.Version
		}
	}
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
	local := editor.local
	pkg := local.program.Identity.Package
	settings := a.settings
	settings.DisablePluginGitHubProxy = !editor.useGitHubProxy
	if local.catalog != catalog {
		settings.PluginCatalogs = maps.Clone(settings.PluginCatalogs)
		if settings.PluginCatalogs == nil {
			settings.PluginCatalogs = make(map[string]pluginCatalogSettings)
		}
		settings.PluginCatalogs[pkg] = pluginCatalogSettings{URL: catalog}
	}
	if err := settings.save(); err != nil {
		editor.message = err.Error()
		return
	}
	a.settings.PluginCatalogs = settings.PluginCatalogs
	a.settings.DisablePluginGitHubProxy = settings.DisablePluginGitHubProxy
	local.catalog, local.message = catalog, ""
	editor.open = false
}

func (a *app) changeInstalledPlugin(local *localPlugin, action string) {
	if local == nil || local.registry == nil || local.busy || local.pending != nil {
		return
	}
	source := pluginregistry.Source{Package: local.program.Identity.Package}
	if action == "install" {
		source.UseGitHubProxy = !a.settings.DisablePluginGitHubProxy
		var err error
		source.Catalog, err = checkedPluginCatalog(local.catalog)
		if err != nil {
			local.message = err.Error()
			a.redraw()
			return
		}
		config := a.settings.PluginCatalogs[source.Package]
		pinned := ""
		if source.Catalog == platformcatalog.OfficialCatalog {
			pinned = platformcatalog.OfficialPublisherKey
		} else if config.URL == source.Catalog {
			pinned = config.LegacyKey
		}
		source.TrustedKey, err = base64.StdEncoding.DecodeString(pinned)
		if err != nil {
			local.message = "无法读取此插件的发布者信息"
			a.redraw()
			return
		}
	}
	var entries []pluginapi.Registration
	for _, entry := range a.platformRegistrations {
		if platform := a.localPlugins[entry.ID]; platform != nil && platform.program.Identity.Package == source.Package {
			if platform.pending != nil {
				return
			}
			entries = append(entries, platform.entry)
		}
	}
	local.busy, local.message = true, "正在验证插件…"
	a.redraw()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		probe := func(ctx context.Context, p pluginregistry.Program) error {
			if len(p.Platforms) != len(entries) {
				return errors.New("release does not include all expected platform entries")
			}
			for _, entry := range entries {
				if err := a.probeInstalledPlugin(ctx, source.Package, entry, p); err != nil {
					return err
				}
			}
			return nil
		}
		var p pluginregistry.Program
		var err error
		switch action {
		case "install":
			p, err = local.registry.Install(ctx, source, probe)
		case "rollback":
			p, err = local.registry.Rollback(ctx, source.Package, probe)
		case "uninstall":
			err = local.registry.Unregister(source.Package)
		}
		mygo.RunOnMain(func() {
			local.busy = false
			if a.quitting {
				return
			}
			defer a.redraw()
			if err != nil {
				local.message = err.Error()
				return
			}
			if action == "uninstall" {
				if current := a.localPlugins[a.platformID]; current != nil && current.program.Identity.Package == source.Package {
					a.commitPlatform(a.defaultPlatform(), true)
				}
				a.setInstalled(source.Package, pluginregistry.Program{})
				local.message = "已卸载，账号和设置已保留"
				return
			}
			a.setInstalled(source.Package, p)
			local.message = "已验证 " + local.version
			if a.remoteActive() != nil {
				local.message += "，下次切换或启动时生效"
			}
		})
	}()
}

func (a *app) probeInstalledPlugin(ctx context.Context, pkg string, entry pluginapi.Registration, p pluginregistry.Program) error {
	identity := platformrpc.Identity{Package: pkg, Entry: string(entry.ID)}
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
		StateDir: stateDir, CredentialService: pluginCredentialService(),
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

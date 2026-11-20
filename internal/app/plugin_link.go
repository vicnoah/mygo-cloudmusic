package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"waygu-music/internal/platformcatalog"
	"waygu-music/internal/pluginregistry"
)

type pluginLinkInstaller struct {
	open, loading bool
	repo, message string
	cancel        context.CancelFunc
	request       context.Context
	review        pluginregistry.GitHubReview
}

func pluginLinkRepository(raw, scheme string) (string, error) {
	if len(raw) > 512 {
		return "", errors.New("invalid plugin link")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != scheme || u.Host != "install-plugin" ||
		u.Path != "" || u.User != nil || u.Fragment != "" {
		return "", errors.New("invalid plugin link")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(query) != 1 || len(query["repo"]) != 1 {
		return "", errors.New("invalid plugin link")
	}
	return pluginregistry.GitHubRepository(query.Get("repo"))
}

func (a *app) openPluginLink(raw, scheme string) {
	repo, err := pluginLinkRepository(raw, scheme)
	if err != nil || a.quitting {
		return
	}
	if a.win == nil {
		a.pluginLink.repo = repo
		return
	}
	if a.pluginLink.open && a.pluginLink.repo == repo {
		a.openPluginManager()
		return
	}
	a.reviewPluginLink(repo)
}

func (a *app) closePluginLink() {
	if a.pluginLink.cancel != nil {
		a.pluginLink.cancel()
	}
	a.pluginLink = pluginLinkInstaller{}
}

func (a *app) reviewPluginLink(repo string) {
	if a.platformID == "" {
		a.cancelLocalRequest()
	}
	a.openPluginManager()
	mygo.App.Focus()
	a.closePluginLink()
	a.pluginSource = pluginSourceEditor{}
	a.pluginLink = pluginLinkInstaller{open: true, repo: repo}
	if repo == platformcatalog.BundledRepository {
		a.pluginLink.message = "飞牛音乐已随宿主提供，可在插件管理中启用。"
		a.redraw()
		return
	}
	if a.pluginRegistry == nil {
		a.pluginLink.message = "插件目录暂不可用。"
		a.redraw()
		return
	}
	ctx, cancel := context.WithTimeout(a.pluginContext, 25*time.Second)
	a.pluginLink.cancel, a.pluginLink.request, a.pluginLink.loading = cancel, ctx, true
	useProxy := !a.settings.DisablePluginGitHubProxy
	a.redraw()
	a.pluginWorkers.Go(func() {
		review, err := a.pluginRegistry.ReviewGitHub(ctx, repo, useProxy)
		if err == nil {
			_, err = review.CurrentArtifact()
		}
		mygo.RunOnMain(func() {
			if a.quitting || a.pluginLink.request != ctx {
				return
			}
			a.pluginLink.loading = false
			if err == nil {
				err = a.checkPluginLinkSource(review.Source)
			}
			if err != nil {
				a.pluginLink.message = "无法安装此插件：" + err.Error()
			} else {
				a.pluginLink.review = review
			}
			a.redraw()
		})
		cancel()
	})
}

func (a *app) checkPluginLinkSource(source pluginregistry.Source) error {
	if a.builtinPlugin(source.Package) {
		return errors.New("插件 ID 已被内置插件使用")
	}
	if pkg := a.managedPlugins[source.Package]; pkg != nil {
		if pkg.busy {
			return errors.New("此插件正在处理，请稍后再试")
		}
		if pkg.installed.Path != "" && pkg.installed.Source != source.Catalog {
			return errors.New("此插件 ID 已由另一发布来源安装")
		}
		if pkg.source == platformcatalog.OfficialCatalog && pkg.source != source.Catalog {
			return errors.New("此插件 ID 已由官方插件目录使用")
		}
	}
	return nil
}

func (a *app) confirmPluginLink() {
	review := a.pluginLink.review
	if review.Source.Package == "" || a.pluginLink.loading {
		return
	}
	if err := a.checkPluginLinkSource(review.Source); err != nil {
		a.pluginLink.message = err.Error()
		return
	}
	id := review.Source.Package
	pkg := a.managedPlugins[id]
	if pkg == nil {
		pkg = &managedPlugin{id: id}
		a.managedPlugins[id] = pkg
	}
	a.catalogPackage = id
	a.closePluginLink()
	if pluginLinkUpToDate(pkg, review) {
		a.redraw()
		return
	}
	pkg.name, pkg.source = review.Release.Name, review.Source.Catalog
	a.changePlugin(pkg, "install", &review.Source)
}

func pluginLinkUpToDate(pkg *managedPlugin, review pluginregistry.GitHubReview) bool {
	if pkg.installed.Path == "" || pkg.installed.Version.Version != review.Release.Version {
		return false
	}
	artifact, err := review.CurrentArtifact()
	return err == nil && strings.EqualFold(artifact.SHA256, pkg.installed.Archive)
}

func (a *app) pluginLinkDialog(c *ui.Context, p pluginManagerColors, width float32) {
	d := &a.pluginLink
	wasOpen := d.open
	ui.DialogBase(c.Key("plugin-install-link"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.36))
		panel.Width(min(468, width-40)).Padding(22).Radius(14).Gap(11).Background(p.panel).
			Border(1, p.border).Shadow(0, 14, 36, -6, ui.RGBA(0, 0, 0, 0.25))
		ui.Text(c, "从 GitHub 安装插件").FontSize(19).FontWeight(700)
		ui.Text(c, d.repo).FontSize(13).TextColor(p.muted).Wrap()
		review := d.review
		switch {
		case d.loading:
			ui.Text(c, "正在验证发布清单…").FontSize(13)
		case d.message != "":
			ui.Text(c, d.message).FontSize(13).Wrap()
		case review.Source.Package != "":
			name := review.Release.Name
			if name == "" {
				name = review.Source.Package
			}
			ui.Text(c, name+" · v"+review.Release.Version).FontSize(15).FontWeight(650).Wrap()
			ui.Text(c, "适配当前设备 · "+runtime.GOOS+"/"+runtime.GOARCH).FontSize(12).TextColor(p.muted)
			if review.Source.Catalog == platformcatalog.OfficialCatalog {
				ui.Text(c, "肥牛音乐官方发布").FontSize(12).TextColor(p.accent)
			} else {
				fingerprint := sha256.Sum256(review.Source.TrustedKey)
				ui.Text(c, fmt.Sprintf("社区发布 · 签名指纹 %x", fingerprint[:8])).FontSize(12).TextColor(p.muted).Wrap()
				ui.Text(c, "确认后将下载并运行该仓库发布的原生插件。").FontSize(13).Wrap()
			}
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spacer(c)
			if ui.Button(c, "取消").Clicked() {
				a.closePluginLink()
			}
			if !d.loading && d.message == "" && review.Source.Package != "" {
				label := "安装"
				if pkg := a.managedPlugins[review.Source.Package]; pkg != nil && pkg.installed.Path != "" {
					label = "更新"
					if pluginLinkUpToDate(pkg, review) {
						label = "在插件管理中查看"
					}
				}
				if ui.PrimaryButton(c, label).Clicked() {
					a.confirmPluginLink()
				}
			}
		})
	})
	if wasOpen && !d.open {
		a.closePluginLink()
	}
}

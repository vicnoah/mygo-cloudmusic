import { Link, createFileRoute } from '@tanstack/solid-router'
import { For, Show, createMemo, createSignal } from 'solid-js'
import { Layout } from '../components/Layout'
import { GitHubMark, Icon } from '../components/Icon'
import snapshot from '../plugin-directory.json'

export const Route = createFileRoute('/plugins')({
  head: () => ({
    meta: [
      { title: '插件目录 · 肥牛音乐' },
      { name: 'description', content: '发现并安装肥牛音乐的官方和社区插件。业务逻辑和桌面界面均用 Go 原生编写。' },
    ],
  }),
  component: Plugins,
})

type Plugin = (typeof snapshot)[number]
const fnmusic = 'https://github.com/waygu-music-plugin/fnmusic'
const github = 'https://github.com/'
const platformNames: Record<string, string> = { darwin: 'macOS', windows: 'Windows', linux: 'Linux' }
const descriptions: Record<string, string> = {
  'waygu-music-plugin/fnmusic': '连接飞牛自建音乐库，浏览文件夹、专辑与歌手。',
  'waygu-music-plugin/songloft': '在桌面浏览 Songloft 音乐库、歌单和收藏。',
  'waygu-music-plugin/subsonic': '连接 Navidrome、Jellyfin、Gonic 等 Subsonic 兼容服务。',
  'waygu-music-plugin/netease': '支持扫码登录，浏览网易云的歌单、推荐和评论。',
  'waygu-music-plugin/qqmusic': '在原生界面访问 QQ 音乐的曲库、歌单和收藏。',
}

const plugins = [...snapshot].sort((a, b) => Number(Boolean(b.bundled)) - Number(Boolean(a.bundled))
  || Number(b.official) - Number(a.official) || b.stars - a.stars || a.name.localeCompare(b.name))
const deepLink = (repo: string) => `wagyu-music://install-plugin?repo=${encodeURIComponent(repo)}`

function compatibility(plugin: Plugin) {
  return Object.entries(platformNames).flatMap(([os, label]) => {
    const archs = plugin.targets?.filter((target) => target.os === os).map((target) => target.arch)
    return archs?.length ? [`${label} ${archs.join(' / ')}`] : []
  })
}

function Plugins() {
  const [search, setSearch] = createSignal('')
  const filtered = createMemo(() => {
    const query = search().trim().toLowerCase()
    return plugins.filter((plugin) =>
      `${plugin.name} ${plugin.repo} ${descriptions[plugin.repo] ?? plugin.description}`.toLowerCase().includes(query))
  })

  return (
    <Layout>
      <div class="plugins-page">
        <section class="plugins-intro" aria-labelledby="plugins-title">
          <div class="plugins-intro-copy">
            <h1 class="display" id="plugins-title">每个音乐平台，<br />都有自己的界面。</h1>
            <p class="lede">插件的业务逻辑和界面都用 Go 原生编写，无需 JavaScript 插件运行时。开发者可以按自己的音乐服务设计首页、播放器和设置，宿主提供播放引擎、缓存和桌面服务。</p>
            <a class="btn btn-primary" href="#directory-title">浏览插件目录</a>
          </div>
          <figure class="plugins-preview">
            <img src={`${import.meta.env.BASE_URL}subsonic-preview.webp`} alt="Subsonic 插件的原生桌面界面" width="840" height="555" />
            <figcaption>Subsonic 的 Jellyfin 连接界面</figcaption>
          </figure>
        </section>
        <section class="plugins-directory" aria-labelledby="directory-title">
          <div class="plugins-directory-head">
            <div>
              <h2 id="directory-title">插件目录</h2>
              <p>在网页选择插件，再到肥牛音乐中确认安装。<Link to="/download">下载客户端</Link></p>
            </div>
            <label class="plugins-search">
              <Icon name="search" size={17} />
              <input type="search" aria-label="搜索插件" placeholder="搜索插件…" value={search()} onInput={(event) => setSearch(event.currentTarget.value)} />
            </label>
          </div>
          <Show when={filtered().length > 0} fallback={<p class="plugins-empty" role="status">没有匹配的插件。</p>}>
            <div class="plugins-grid">
              <For each={filtered()}>{(plugin) => <PluginCard plugin={plugin} />}</For>
            </div>
          </Show>
          <p class="plugins-help">网页安装需要肥牛音乐 0.2.30 或更新版本。升级后先启动一次客户端。也可以在「插件管理」中按名称安装官方插件。</p>
        </section>
        <section class="plugins-publish" aria-labelledby="publish-title">
          <h2 id="publish-title">发布你的插件</h2>
          <p>将插件发布到公开的 GitHub 仓库，添加 <code>wagyu-music-plugin</code> 话题。最新正式版包含 <code>catalog.json</code>、签名的 <code>manifest.json</code> 和对应系统的 ZIP 包后，即可在目录中自动收录。</p>
          <a href={fnmusic}>查看飞牛音乐源码</a>
        </section>
        <section class="plugins-credits" aria-labelledby="credits-title">
          <h2 id="credits-title">上游与参考项目</h2>
          <p>
            肥牛音乐基于 <a href={github + 'egoist/mygo-cloudmusic'}>MyGo 云音乐</a>，使用 <a href={github + 'egoist/mygo'}>MyGo</a> 绘制原生界面。
            插件发现和网页安装参考 <a href={github + 'egoist/mygo-music'}>MyGo Music</a>，网易云适配参考 <a href={github + 'AkinoKaede/mygo-music-netease-cloud-music'}>AkinoKaede 的网易云音乐插件</a>。
          </p>
        </section>
      </div>
    </Layout>
  )
}

function PluginCard(props: { plugin: Plugin }) {
  const plugin = props.plugin
  return (
    <article class="plugins-card">
      <div class="plugins-card-top">
        <span class="plugins-avatar" aria-hidden="true">
          <Show when={plugin.avatar} fallback={<GitHubMark size={19} />}>
            <img src={plugin.avatar} loading="lazy" alt="" width="42" height="42" />
          </Show>
        </span>
        <div class="plugins-card-name">
          <h3>{plugin.name}</h3>
          <a href={github + plugin.repo}>{plugin.repo}</a>
        </div>
        <span class="plugins-badge" classList={{ official: plugin.official }}>{plugin.official ? '官方' : '社区'}</span>
      </div>
      <p>{descriptions[plugin.repo] ?? plugin.description}</p>
      <Show when={plugin.version}><span class="plugins-version">v{plugin.version}</span></Show>
      <Show when={compatibility(plugin).length > 0}>
        <ul class="plugins-targets" aria-label="支持的系统和架构"><For each={compatibility(plugin)}>{(label) => <li>{label}</li>}</For></ul>
      </Show>
      <div class="plugins-card-action">
        <Show when={!plugin.bundled} fallback={<span class="plugins-bundled">随宿主提供</span>}>
          <a class="btn btn-primary" href={deepLink(plugin.repo)} aria-label={`在肥牛音乐中安装 ${plugin.name}`}>
            <Icon name="download" size={16} />在肥牛音乐中安装
          </a>
        </Show>
      </div>
    </article>
  )
}

export const repo = 'https://github.com/vicnoah/mygo-cloudmusic'

export type Asset = { name: string; size: number; browser_download_url: string }
export type Release = { tag_name: string; published_at: string; assets: Asset[] }

declare const __RELEASE__: Release
// The latest release, as it was when the site was built (see vite.config.ts).
export const release = __RELEASE__
export const mygo = 'https://mygo.egoist.dev'

export const site = {
  name: '肥牛音乐',
  title: '肥牛音乐 — 自建音乐库桌面客户端',
  description:
    '肥牛音乐（Wagyu Music）是用 Go 与 MyGo 写成的原生桌面音乐客户端，没有 WebView。连接飞牛音乐（FN Music）、Songloft 与 Navidrome 等 Subsonic 服务，也支持网易云与 QQ 音乐；边下边播、无损与 Hi-Res、逐字歌词与「律动」、刘海播放器。',
}

# 平台插件

飞牛音乐随宿主提供。打开托盘或应用菜单的「插件管理」，每个插件都有独立的一行，可安装、启用和更新。Subsonic 的连接类型在其自己的「设置」窗口选择。在「来源设置」填写 HTTPS 地址可添加额外插件目录。[官方插件目录](https://github.com/waygu-music-plugin/registry)

插件默认通过 GH-Proxy 获取 GitHub 上的目录、发布清单和安装包。在「插件管理 → 来源设置」关闭「GitHub 下载加速」并保存，即可让所有插件直接访问 GitHub。自建站点使用其原有地址。[GH-Proxy 使用指南](https://gh-proxy.com/docs/github-accelerator)

外置平台在独立进程中绘制主窗口、播放器和设置等界面。宿主管理插件安装与切换，并提供解码、输出、缓存和其他桌面服务。SDK 的[接入和协议说明](https://github.com/waygu-music-plugin/sdk)定义了对应接口。

| 项目 | 仓库 |
| --- | --- |
| SDK | [waygu-music-plugin/sdk](https://github.com/waygu-music-plugin/sdk) |
| 飞牛音乐 | [waygu-music-plugin/fnmusic](https://github.com/waygu-music-plugin/fnmusic) |
| Songloft | [waygu-music-plugin/songloft](https://github.com/waygu-music-plugin/songloft) |
| Subsonic、Navidrome、Jellyfin、Gonic | [waygu-music-plugin/subsonic](https://github.com/waygu-music-plugin/subsonic) |
| 网易云音乐 | [waygu-music-plugin/netease](https://github.com/waygu-music-plugin/netease) |
| QQ 音乐 | [waygu-music-plugin/qqmusic](https://github.com/waygu-music-plugin/qqmusic) |
| 官方目录及构建工具 | [waygu-music-plugin/registry](https://github.com/waygu-music-plugin/registry) |

## 收录到网站

把插件放在公开的 GitHub 仓库，添加话题 `wagyu-music-plugin`。最新正式版带上 `catalog.json`、签名的 `manifest.json` 和目标系统的 ZIP 包。每个仓库发布一个插件包。包 ID 在宿主中标识已安装的插件，建议用作者名作为前缀。GitHub Actions 会验证符合格式的发行并更新[网站目录](https://vicnoah.github.io/mygo-cloudmusic/plugins/)。

插件采用 Go 编写，包括业务逻辑和完整的原生界面。开发者可从飞牛音乐的窗口和适配器入手。外置插件的入口使用 SDK 的 `desktop.RunPlatformPlugin`，由宿主在用户确认后启动。[飞牛音乐源码](https://github.com/waygu-music-plugin/fnmusic)

# 肥牛音乐（Wagyu Music）

用 Go 和 [MyGo](https://github.com/egoist/mygo) 写的第三方桌面音乐客户端，原生界面，无 WebView。

**[插件网站 · 浏览与安装](https://vicnoah.github.io/mygo-cloudmusic/plugins/)** · [下载肥牛音乐](https://github.com/vicnoah/mygo-cloudmusic/releases/latest) · [项目官网](https://vicnoah.github.io/mygo-cloudmusic/)

宿主默认内置 **飞牛音乐（FN Music）**。**Songloft**、**Navidrome / Jellyfin / Gonic 等 Subsonic 兼容服务**及其它音乐平台可通过托盘的「插件管理」单独安装、更新与回退。各平台分别管理账号、歌单、收藏和音质等平台专属设置。

各平台插件绘制自己的完整界面，包括主窗口的底部播放器、正在播放页、歌词，以及设置窗和桌面歌词。插件负责获取平台音源和歌词，宿主提供原生窗口运行、音频解码与输出、队列执行及本地缓存等服务。平台切换统一在宿主的「插件管理」窗口完成。

## 功能

### 飞牛音乐（FN Music）

- 连接 NAS 上的 FN Music 服务：填写地址、用户名与密码即可登录，启用访问码的 NAS 还需填写访问码
- 登录态与连接配置保存在平台独立命名空间与系统安全凭据存储中
- 音乐库按文件夹浏览，封面墙与列表两种视图，歌手、专辑、流派分页浏览
- 喜欢的音乐、最近播放、播放历史，支持删除历史记录
- 歌单的创建、重命名、删除与收藏，把歌曲加入或移出歌单
- 搜索歌曲、歌手、专辑、播客与流派，输入时显示搜索建议
- 本地无法直接播放的格式由服务端转码为 HLS，转码模式（自动 / 总是 / 从不）与码率在设置中可调
- 各页面独立的排序选择，记住每个列表上次的排序方式
- 边下边播与歌单内播放，歌曲缓存在本机
- 迷你唱片机：在资料库中漫无目的地随机播放，遇到喜欢的可一键收藏

### Songloft 自建音乐服务器

- 连接自建 Songloft 音乐服务：填写服务器地址、用户名与密码即可登录，登录态保存在系统安全凭据存储中
- 音乐库全量歌曲浏览、随机播放与多种排序切换（添加时间、标题、歌手、专辑等）
- 文件夹层级树形深入浏览与递归播放，带大尺寸目录封面 Tile、面包屑导航与上级目录跳转
- 专辑与歌手详情聚合展示，直达曲目详情页
- 歌单管理：创建新歌单、重命名、删除，在曲目右键菜单中将歌曲加入或移出歌单
- 实时红心联动：支持单曲收藏/取消收藏（我喜欢），与服务端红心状态实时双向同步
- 全维度检索：歌曲、专辑、歌手多维度检索与直接跳转详情
- 逐字歌词与翻译：内嵌与旁挂歌词自动加载解析并进行特殊实体反转义

### Subsonic 兼容平台（Navidrome / Jellyfin / Gonic 等）

- 支持连接自建 Subsonic / OpenSubsonic 兼容服务（在 Navidrome 等主流服务上完整测试）
- 填写服务器地址、用户名与密码/API Token 即可连接登录，登录凭据保存在平台独立命名空间与系统安全存储中
- 音乐库支持全量歌曲浏览与按目录树层级深入浏览播放
- 专辑与歌手详情，支持按最新、最常播放、最近播放、年份、字母、收藏等维度排序
- 支持单曲红心与专辑收藏/取消收藏，实时同步服务端 Star 标记与即时刷新
- 歌单的创建、重命名、删除与曲目管理（添加/移出歌单）
- 多维度检索（单曲、专辑、歌手），搜索结果直达详情与连续播放
- 支持服务端转码输出与最大码率调节（可在设置中配置自动 / 总是 / 从不转码与码率上限）
- 本地最近播放记录按平台隔离，多平台记录互不串扰

### 通用特性

- 实验性高级音频解码器：支持 DSD (DSF/DFF)、DTS、FLAC、APE、ALAC、WAV、M4A、WMA、CUE 分轨等全格式原生解码直出，告别依赖服务端转码
- DLNA / UPnP 无线投播：支持一键将音乐投送至局域网内的智能音箱、投影仪与智能电视（如小爱音箱、小明投影仪、乐播投屏等），切换输出时保留播放状态和进度；当设备不支持原音源时，尝试使用平台提供的兼容音源或服务端转码
- 听过的歌曲缓存在本机，再次播放无需等待
- 音量均衡，暂停与继续时淡出淡入，拖动进度无杂音
- 退出后再打开，从上次的播放队列与进度继续
- 逐字歌词与翻译，正在播放页的「律动」视图
- 刘海播放器：在屏幕顶部刘海处显示迷你播放器，悬停展开（在设置中开启）
- 喜欢歌曲、播放队列、随机与循环播放
- 浅色 / 深色 / 跟随系统主题

## 插件

### 浏览、安装与切换

在插件网站浏览官方与社区插件。点击「在肥牛音乐中安装」，在客户端核对插件来源、版本和设备兼容性后确认安装。 [打开插件网站](https://vicnoah.github.io/mygo-cloudmusic/plugins/)

也可以从托盘或应用菜单打开「插件管理」，在同一窗口安装、切换、更新或回退插件。飞牛音乐随宿主提供。Subsonic 的连接类型在插件自己的「设置」中选择。在「来源设置」可填写 HTTPS 地址，添加额外的插件目录。

GitHub 上的插件目录、清单和安装包默认通过 GH-Proxy 下载。要直接访问 GitHub，可在「插件管理 → 来源设置」关闭「GitHub 下载加速」并保存。 [GH-Proxy 使用指南](https://gh-proxy.com/docs/github-accelerator)

### 开发与网站收录

插件使用 Go 编写业务逻辑和完整的原生界面。外置插件通过 SDK 的 `desktop.RunPlatformPlugin` 入口运行。开发者可从内置飞牛音乐的窗口和服务适配代码入手。 [飞牛音乐源码](https://github.com/waygu-music-plugin/fnmusic) · [SDK 和协议说明](https://github.com/waygu-music-plugin/sdk)

要收录到插件网站，将插件发布到公开的 GitHub 仓库，添加话题 `wagyu-music-plugin`，并在最新正式版附上 `catalog.json`、签名的 `manifest.json` 及各目标系统的 ZIP 包。每个仓库对应一个插件包。建议用作者名作为插件包 ID 的前缀。网站通过 GitHub Actions 发现并验证符合格式的发行。 [目录与发布工具](https://github.com/waygu-music-plugin/registry)

| 项目 | 源码仓库 |
| --- | --- |
| SDK | [waygu-music-plugin/sdk](https://github.com/waygu-music-plugin/sdk) |
| 飞牛音乐 | [waygu-music-plugin/fnmusic](https://github.com/waygu-music-plugin/fnmusic) |
| Songloft | [waygu-music-plugin/songloft](https://github.com/waygu-music-plugin/songloft) |
| Subsonic 兼容平台 | [waygu-music-plugin/subsonic](https://github.com/waygu-music-plugin/subsonic) |
| 官方目录与发布工具 | [waygu-music-plugin/registry](https://github.com/waygu-music-plugin/registry) |

## 构建

需要 Go 1.27+。

```bash
go tool mygo dev     # 开发模式运行
go tool mygo build   # 打包到 build/
go test ./...        # 测试宿主
```

宿主通过 Go module 引入独立的 SDK 与飞牛音乐。外置插件在各自仓库构建和发版。 [插件开发与收录说明](#开发与网站收录)

## 致谢

- [MyGo](https://github.com/egoist/mygo)：原生 UI 与应用框架
- [folia-major](https://github.com/chthollyphile/folia-major)：歌词动画效果的参考
- [sherpa-onnx](https://github.com/k2-fsa/sherpa-onnx)、[Qwen3-ASR](https://github.com/QwenLM/Qwen3-ASR) 与 [Silero VAD](https://github.com/snakers4/silero-vad)：本机语音识别
- [go-subsonic](https://github.com/supersonic-app/go-subsonic)：Subsonic API Go 客户端实现参考

## 许可证

[AGPL-3.0](LICENSE)

# MyGo 云音乐：更多音质等级与空间音频实现方案

## 0. 结论摘要

MyGo 当前只实现了 5 档网易云音质：`standard / higher / exhigh / lossless / hires`。网易云实际还有 5 个高价值档位：

| API level | 官方名称 | 实测格式 | 会员要求 | 空间属性 |
|---|---|---|---|---|
| `jyeffect` | 高清臻音（Spatial Audio） | FLAC 96 kHz / 24 bit / 2ch | 黑胶 VIP | 立体声空间化，已烘焙进音频 |
| `sky` | 沉浸环绕声（Surround Audio） | FLAC 44.1 kHz / 16 bit / **6ch（5.1 side）** | 黑胶 SVIP | 真实 5.1 声道床 |
| `jymaster` | 超清母带（Master） | FLAC 192 kHz / 24 bit / 2ch | 黑胶 SVIP | 非空间音频，超高解析 |
| `dolby` | 杜比全景声（Dolby Atmos） | MP4 / E-AC-3，最高 7.1.4 | 黑胶 SVIP | 对象音频；本地只能解 E-AC-3 核心 |
| `vivid` | 臻音全景声（Audio Vivid） | MP4 / AV3A，最高 7.1.4 | 黑胶 SVIP（官方主要面向 HarmonyOS） | 对象/声床三维音频 |

建议分两阶段交付：

1. **先交付 `jyeffect / sky / jymaster / dolby` 四档**：全部可由 MPV/FFmpeg 直接解码，能快速覆盖“更多音质等级 + Surround Audio 空间感”。
2. **再交付 `vivid`**：需要 custom libmpv + AV3A 解码器（MOLAN-AV3A 已验证可在 macOS arm64 编译），这是唯一需要改解码器二进制的档位。

> 核心结论：`sky` 的 5.1 空间感可以通过 MPV 多声道输出直接获得；`vivid` 的空间感必须先把 AV3A 解成 12 声道 PCM（7.1.4），再做多声道输出或双耳渲染。二者都不是“加一个音质字符串”就能完成，必须补齐“取链 → 解封装 → 解码 → 声道映射 → 输出/渲染”整条链路。

## 实施状态（0.2.19）

- Phase 0：10 档枚举、`encodeType`、`channelLayout` / `immerseType`、interface3/interfacepc 与 `-460` 回退、HarmonyOS vivid 指纹、逐档降级、播放页「已降档」标记已完成。
- Phase 1：`jyeffect` / `sky` / `jymaster` / `dolby` 的 MPV 解码、缓存、UI、徽标与 DLNA MIME 映射已完成。
- Phase 2：`vivid` 的 AV3A helper 已随 macOS arm64 包分发；取流、解码 12ch 7.1.4 WAV、MPV 播放链路已实测。MOLAN-AV3A 仍为非 OSI 许可，正式分发前需法务确认。
- Phase 3：`audio-channels` 多声道、空间音频模式（自动 / 扬声器 / 耳机 / 下混）、耳机串扰下混、杜比 E-AC-3 直通、设备选择 UI 已完成。当前内置 MPV 未启用 libmysofa，耳机模式在无 HRTF 时明确使用低串扰下混，UI 已标注；`jyeffect` 不重复叠加 HRTF。
- Phase 4：macOS arm64 安装包、Windows/Linux 交叉编译、真实账号播放验证与文档更新已完成；Windows/Linux 无 AV3A helper 时 `vivid` 自动降级。
- 0.2.18/0.2.19 补丁：E-AC-3 在 PCM-only 输出上不再强制 SPDIF 直通；`audio-spdif` 仅在明确选择 HDMI/SPDIF 设备时启用；E-AC-3 曲目自动关闭独占模式，MPV 初始化失败时自动回退 PCM。macOS 原生 E-AC-3 解码能力已通过 `afinfo`/`afconvert` 和 SDK 头文件核验。

---

## 1. Ncrust 的参考价值

参考项目：[GuitaristRin/Ncrust](https://github.com/GuitaristRin/Ncrust)

Ncrust 的 Android 实现已经验证了网易云 URL 获取链路的关键细节（见 `app/src/main/java/com/takahashirinta/ncrust/player/SongUrlFetcher.kt`）：

- 7 档音质：`standard → higher → exhigh → lossless → hires → jyeffect → dolby`；
- `song/enhance/player/url/v1` 请求参数：
  - `ids=[songId]`
  - `level=<level>`
  - `encodeType=dolby ? "mp4" : "flac"`
  - `header` 为 PC 客户端指纹（`os=pc`、`deviceId=pyncm!`、随机 `requestId`）
- 取链 host 使用 `https://interface3.music.163.com`，不是普通 API host；
- 按 `code != 200` / `url == null` 逐档降级；
- `FLAC_TIERS`（`lossless / hires / jyeffect`）在设备无 FLAC 解码器时跳过；
- 播放失败时按 `dolby → jyeffect → hires → lossless → exhigh → higher → standard` 降一档重试；
- 对 `freeTrialInfo`、`level` 被降档、无版权等权益状态做了显式 UI 处理。

可直接借鉴：**分级请求、逐档 fallback、`encodeType` 映射、权益降档识别、重试策略**。

不能照搬：Ncrust 是 Android MediaCodec 路径，`FLAC_TIERS` 设备门控不适用于 MyGo。MyGo 的 MPV/FFmpeg 路径可以解码 5.1 FLAC、192 kHz/24 bit FLAC 和 E-AC-3，因此桌面端的能力上限更高。

---

## 2. 网易云音质档位事实核验

### 2.1 官方品质页数据（2026-11 抓取）

网易云 `st-vip-audio-quality` 页面中的品质说明表给出：

- 臻音全景声（Audio Vivid）：44.1 kHz、16 bit、最高 7.1.4
- 超清母带（Master）：192 kHz、24 bit、2 声道
- 沉浸环绕声（Surround Audio）：44.1 kHz、16 bit、5.1 / 2 声道
- 高清臻音（Spatial Audio）：96 kHz、24 bit、2 声道
- 高解析度无损（Hi-Res）：48–192 kHz、24 bit、2 声道
- 无损（SQ）：44.1–48 kHz、16 bit、2 声道
- 极高（HQ）：320 kbps
- 标准：128 kbps
- 杜比全景声（Dolby Atmos）：48 kHz、最高 7.1.4

来源：[网易云音乐品质页](https://music.163.com/g/st-vip-audio-quality?nm_style=sbt&quality=sky)

### 2.2 实测 sample 文件

从官方品质页的 sample URL 直接下载并 `ffprobe`：

| 档位 | 文件大小 | ffprobe 结果 |
|---|---:|---|
| `sky` | 17.6 MB / 66s | `flac, 16 bit, 6 channels, 44100 Hz, 5.1(side)` |
| `jymaster` | 38 MB / 66s | `flac, 24 bit, 2 channels, 192000 Hz` |
| `jyeffect` | 23 MB / 66s | `flac, 24 bit, 2 channels, 96000 Hz` |

`ffmpeg -i netease-sky.flac -f null -` 与 `-ac 2` 下混均正常，说明：
- `sky` 确实是 5.1 声道 FLAC，不是“虚拟环绕的立体声”；
- `jymaster` 确实是 192 kHz / 24 bit；
- `jyeffect` 的“空间感”已经处理在 96 kHz / 24 bit 立体声文件里，不需要额外 HRTF。

### 2.3 匿名账号实测取链

使用项目现有 eapi 实现（`internal/netease/eapi.go`）请求 `song/enhance/player/url/v1`，匿名账号结果：

| 请求 level | 返回 level | 返回 type | 说明 |
|---|---|---|---|
| `standard` | standard | mp3 | 128 kbps |
| `higher` | higher | mp3 | 192 kbps |
| `exhigh` | exhigh | mp3 | 320 kbps |
| `lossless` / `hires` / `jyeffect` / `sky` / `jymaster` / `dolby` | exhigh | mp3 | 免费账号被服务端降档，不是接口失败 |
| `vivid`（PC 指纹） | — | — | `-110` 或 `-460` |
| `vivid`（HarmonyOS 指纹） | exhigh | mp3 | 接口接受 `vivid`，免费账号降档 |

结论：
- 匿名/免费账号无法验证 SVIP 档位的真实流，但降档行为本身证明接口按权益返回；
- `vivid` 对客户端指纹敏感：PC 指纹会被 `-110/-460`，HarmonyOS 指纹可进入降档路径；
- 正式实现必须用真实 VIP/SVIP 账号做一次全档位矩阵验证。

---

## 3. 获取链路设计

### 3.1 请求

```http
POST https://interface3.music.163.com/eapi/song/enhance/player/url/v1
Content-Type: application/x-www-form-urlencoded

params=<eapi 加密后的 JSON>
```

请求 JSON（逻辑字段）：

```json
{
  "ids": "[1969519579]",
  "level": "vivid",
  "encodeType": "mp4",
  "header": {
    "os": "harmony",
    "appver": "9.5.90",
    "deviceId": "...",
    "requestId": "...",
    "versioncode": "140",
    "mobilename": "",
    "buildver": "...",
    "resolution": "1920x1080",
    "channel": "netease",
    "__csrf": ""
  },
  "e_r": false
}
```

`encodeType` 映射：

| level | encodeType |
|---|---|
| `standard` / `higher` / `exhigh` | `mp3` |
| `lossless` / `hires` / `jyeffect` / `sky` / `jymaster` | `flac` |
| `dolby` / `vivid` | `mp4` |

host 策略：
1. 第一优先 `interface3.music.163.com`（Ncrust 实测取 URL 专用）；
2. `-460` / 连接失败时回退 `interfacepc.music.163.com`；
3. `vivid` 使用 `os=harmony` 的 header profile；若 `-110` / `-460`，回退 `sky` 或 `jyeffect` 并在 UI 说明“当前账号/客户端不支持该档位”。

### 3.2 响应

`data[0]` 中需要保留的字段：

```go
type Stream struct {
    URL           string  // 播放地址
    Type          string  // mp3 / flac / mp4 / m4a
    Level         string  // 实际返回档位，可能低于请求档位
    BR            int64   // 实际码率
    Size          int64
    SR            int     // 采样率
    ChannelLayout string  // 例如 5.1(side)、7.1.4
    ImmerseType   any     // 沉浸类型（保留原始 JSON）
    Fee           int
    Code          int
    FreeTrialInfo json.RawMessage
}
```

权益判定：
- `URL` 非空且 `freeTrialInfo != null` → 试听片段（45 秒），UI 显示“试听”；
- `URL` 非空且 `Level` 低于请求档位 → `capped`，显示实际档位并提示“当前账号最高可播放 XX”；
- `URL` 为空 / `code != 200` → 按 `privilege.st`、`fee` 判定无版权 / 需要 VIP / 需要登录；
- `vivid` 的 `-110` 视为“档位不可用”，不是“歌曲无版权”。

---

## 4. 解码与空间渲染链路

### 4.1 总链路

```text
网易云 eapi
  └─ SongURL(level) → Stream{URL, Type, Level, ChannelLayout}
       └─ MPV / libavformat 解封装
            ├─ MP3 / FLAC stereo
            │    └─ FFmpeg 解码 → 立体声 PCM → CoreAudio/WASAPI/ALSA
            ├─ FLAC 5.1 (sky)
            │    └─ FFmpeg 解码 → 6ch PCM → 5.1(side) 输出或 HRTF 下混
            ├─ FLAC 192/24 (jymaster)
            │    └─ FFmpeg 解码 → 立体声 PCM → 设备重采样/独占输出
            ├─ MP4 / E-AC-3 (dolby)
            │    └─ FFmpeg 解码 E-AC-3 核心 → 5.1/7.1 bed
            │    └─ 或 `--audio-spdif` 直通 HDMI 功放
            └─ MP4 / AV3A (vivid)
                 └─ libmolan_av3a 解码 → 12ch interleaved s16 PCM (7.1.4)
                      └─ 7.1.4 输出或 HRTF 双耳渲染
```

### 4.2 各档位解码路径

| 档位 | 容器/编码 | 解码器 | MyGo 现有能力 | 结论 |
|---|---|---|---|---|
| `jyeffect` | FLAC 96/24 stereo | FFmpeg / MPV | MPV 可解 | 直接支持 |
| `sky` | FLAC 44.1/16 5.1 | FFmpeg / MPV | MPV 可解；内置 Go FLAC 只取前 2 声道 | 必须走 MPV |
| `jymaster` | FLAC 192/24 stereo | FFmpeg / MPV | MPV 可解 | 直接支持 |
| `dolby` | MP4 / E-AC-3 | FFmpeg eac3 | MPV/FFmpeg 可解 5.1/7.1 核心 | 支持核心解码；Atmos 对象需直通或 Dolby 授权 |
| `vivid` | MP4 / AV3A | **MOLAN-AV3A** | 当前 libmpv 不支持 AV3A | 需要定制 libmpv |

### 4.3 `vivid` 的 AV3A 解码

[MOLAN-AV3A](https://github.com/baiji6/MOLAN-AV3A) 是纯 C 的 AVS3-P3（Audio Vivid）参考解码器封装：

- 支持 MP4/M4A 中的 `av3a` track 与裸 AV3A ES；
- 输出交错 `s16` PCM，最多 16 声道；
- 已在真实网易云 12 声道（7.1.4）文件上验证；
- 在本机 macOS arm64 上已成功编译：`libmolan_av3a.a` 约 383 KB，CLI 约 370 KB，模型约 78 KB。

集成方式建议：

1. 给 FFmpeg 增加 `AV_CODEC_ID_AV3A` 解码器包装：
   - 解析 MP4 的 `av3a` sample entry；
   - 每帧调用 `molan_av3a_decode()`；
   - 输出 `AVFrame`，`ch_layout = AV_CHANNEL_LAYOUT_7POINT1POINT4_BACK`（12ch）。
2. 用该 FFmpeg 编译 custom libmpv（macOS arm64 / Windows / Linux 分别构建）；
3. 在 `internal/player/mpv/natives.go` 增加新的 `libmpv-av3a` 资源版本与 SHA256，作为独立可选组件下载；
4. 启动时检测 libmpv 是否带 `av3a` 解码器；没有则 `vivid` 档位自动降级到 `sky`。

> 注意：MOLAN 上游是 UWA Code Sharing Policy 的参考软件，不是 OSI 开源许可证，正式分发前必须做法务确认。若不能接受，替代方案是 `ffmpeg-av3a` + ArcVideo AV3A SDK（需要商业授权，且仓库本身不带解码库）。

### 4.4 空间感实现

`sky`（5.1）和 `vivid`（7.1.4）的“空间感”来自多声道，不是普通立体声。播放端有两种输出模式：

1. **多声道直通（音箱 / HDMI 功放）**
   - MPV：`--audio-channels=7.1.4,5.1,stereo`
   - `--audio-exclusive=yes` 绕过系统混音器
   - `sky` 使用 `5.1(side)`；`vivid` 使用 `7.1.4`（12ch 顺序：FL FR FC LFE BL BR SL SR TFL TFR TBL TBR）
   - `dolby` 若要保留 Atmos 对象，使用 `--audio-spdif` 直通到支持 Atmos 的功放；否则只能解 E-AC-3 核心。

2. **耳机双耳渲染（HRTF）**
   - 将 5.1/7.1.4 下混为双耳立体声，而不是普通 stereo downmix；
   - MPV 可通过 `--af=lavfi=[sofalizer=sofa=...]`（需 FFmpeg 开启 libmysofa）或 `headphone` 滤镜；
   - 如果 custom FFmpeg 不启用 libmysofa，可先提供“普通立体声下混”，并在 UI 标注“空间感仅在多声道设备上生效”。

3. **`jyeffect` 不需要再做 HRTF**
   - 它已经是网易云处理好的 96/24 立体声空间化版本；再套 HRTF 会二次染色。

---

## 5. MyGo 当前代码改造点

以下路径均相对仓库根目录，行号为 2026-11-05 的 `v0.2.14` 代码。

### 5.1 音质数据模型与请求

`internal/netease/netease.go:395-461`

- `Stream` 增加 `BR`、`SR`、`ChannelLayout`、`ImmerseType`、`Code`、`Fee` 字段；
- `Levels` 从 5 档扩展到 10 档；
- `Lossless()` 改为 `EncodeType(level)` 映射；
- `SongURL` 按 `encodeType` 发送，接受 `mp3/flac/mp4/m4a`，保留服务端 `Level`；
- 增加 fallback ladder（参考 Ncrust）：
  - `vivid → sky → jyeffect → hires → lossless → exhigh → higher → standard`
  - `dolby → sky → jyeffect → hires → lossless → exhigh → higher → standard`
  - `sky → jyeffect → hires → lossless → exhigh → higher → standard`
  - `jyeffect → hires → lossless → exhigh → higher → standard`
  - `jymaster → hires → lossless → exhigh → higher → standard`

`internal/netease/eapi.go:72-126`

- 现在 `eapiPost` 固定 `interfacepc`；抽出 `eapiPostHost()`，让取 URL 使用 `interface3`；
- `vivid` 增加 `os=harmony` 的 header profile；
- 增加 `-460` 重试/回退。

### 5.2 设置 UI

`internal/app/settings.go:203-210`

```go
var qualityLevels = []struct{ level, label string }{
    {"standard", "标准（128k）"},
    {"higher", "较高（192k）"},
    {"exhigh", "极高（320k）"},
    {"lossless", "无损（FLAC）"},
    {"hires", "Hi-Res"},
    {"jyeffect", "高清臻音（96kHz/24bit）"},
    {"sky", "沉浸环绕声（5.1）"},
    {"jymaster", "超清母带（192kHz/24bit）"},
    {"dolby", "杜比全景声（E-AC-3）"},
    {"vivid", "臻音全景声（Audio Vivid 7.1.4）"},
}
```

`internal/app/ui_settings.go:131-147`

- 音质行说明从“无损与 Hi-Res 需要黑胶 VIP”改为“无损/Hi-Res/高清臻音需 VIP；沉浸环绕声/超清母带/杜比/臻音全景声需 SVIP”；
- 增加“高级解码引擎为必选”的提示；如果 MPV 未安装，`sky/dolby/vivid` 置灰或显示“需下载解码组件”。

`internal/app/ui_nowplaying.go:351-362`

- `qualityBadge` 从只标 `无损`/`Hi-Res` 扩展到：`高清臻音`、`沉浸环绕声`、`超清母带`、`杜比全景声`、`臻音全景声`；
- 增加“试听”/“已降档”状态。

### 5.3 播放器与缓存

`internal/player/player.go:1019-1046`

- `open()` 对空间档位强制走 MPV：`if spatial(level) && !p.UsingMPV() { return ErrNeedMPV }`；
- 缓存 `FilePath`/`lookup` 要支持“请求高档位但实际返回低档位”的查找策略（按实际 `Level` 查找，不要只按请求 level 索引）。

`internal/player/cache.go:44-46,193-201`

- `keepable` 增加 `mp4/m4a`；
- `songs()` 扩展 `.mp4`/`.m4a`；
- `vivid` 12ch 文件可能很大，保留“单曲不超过缓存上限 1/4”的限制，避免挤爆缓存。

`internal/player/flac.go:77-78,134-154`

- 内置 Go FLAC 只输出前 2 声道；`sky` 必须走 MPV，不要尝试用内置路径播放 5.1。

### 5.4 MPV 配置

`internal/player/mpv/engine.go:49-65`

增加配置：

```go
type Config struct {
    // ...现有字段
    AudioChannels string // "auto-safe" / "7.1.4,5.1,stereo"
    AudioSpdif    string // "eac3" 或 ""
    SpatialMode   string // "speakers" / "headphones" / "stereo"
    HrtfProfile   string // SOFA 文件路径或内置 profile
}
```

`NewEngine` 中设置：

```go
if cfg.AudioChannels != "" { inst.SetProperty("audio-channels", cfg.AudioChannels) }
if cfg.AudioSpdif != "" { inst.SetProperty("audio-spdif", cfg.AudioSpdif) }
```

`internal/app/app_hifi_dlna.go:14-26`

- 把 `AudioChannels`、`AudioSpdif`、`SpatialMode` 从 settings 同步到 `mpv.Config`；
- 在“高级音频与硬件解码”下增加“空间音频输出”设置组。

### 5.5 DLNA / 投送

`internal/player/dlna/proxy.go:490-505`

- 增加 `mp4` → `audio/mp4`、`m4a` → `audio/mp4`、`av3a` → `audio/mp4`；
- 但大多数 DLNA 接收器不支持 5.1/7.1.4，投送时建议自动降到 `lossless`/`exhigh`，或提示“目标设备不支持空间音频”。

---

## 6. 分阶段实施计划

| 阶段 | 内容 | 预计 | 风险 |
|---|---|---|---|
| Phase 0 | API 层：10 档枚举、`encodeType`、`channelLayout`、host/header 回退、fallback ladder、权益提示 | 1–2 天 | 低 |
| Phase 1 | `jyeffect / sky / jymaster / dolby`：MPV 解码、缓存、UI、徽标、DLNA 降级 | 3–5 天 | 低–中 |
| Phase 2 | `vivid`：MOLAN-AV3A → FFmpeg → custom libmpv；macOS arm64 先跑通 | 5–8 天 | 高（许可/构建） |
| Phase 3 | 空间渲染：`audio-channels` 多声道、HRTF/sofalizer、HDMI 直通、设备模式 UI | 3–5 天 | 中 |
| Phase 4 | 全平台构建、真实账号矩阵测试、文档与发布 | 2–3 天 | 中 |

建议第一个 PR 只做 Phase 0 + Phase 1（不含 `vivid`），先让用户真正听到 `sky` 的 5.1 和 `jymaster` 的 192/24；`vivid` 单独作为实验特性发布，避免被 AV3A 许可和 custom libmpv 拖住整个版本。

---

## 7. 风险与待确认事项

1. **`vivid` 的客户端限制**：官方品质页只对 HarmonyOS ≥ 9.5.85 展示 `vivid`；桌面端是否长期允许 `os=harmony` 指纹取流，需要真实 SVIP 账号验证。若被风控，建议将 `vivid` 标记为“实验性/仅部分账号”。
2. **AV3A 许可**：MOLAN-AV3A 上游是 UWA Code Sharing Policy，非标准开源许可；需法务确认是否能随 MyGo 分发。
3. **Dolby Atmos**：FFmpeg 只能解 E-AC-3 核心，不能渲染 Atmos 对象；真正 Atmos 需要 `--audio-spdif` 直通 + 支持 Atmos 的功放，或 Dolby 商业解码器。UI 不应把“E-AC-3 核心”标成完整 Atmos。
4. **macOS 多声道输出**：Mac 内置扬声器通常只有立体声；5.1/7.1.4 需要 HDMI/USB 多声道设备。耳机空间感必须靠 HRTF，普通 stereo downmix 会丢失定位。
5. **缓存体积**：`vivid` 12ch 44.1/16 文件可能达到数百 MB/首，现有“单曲不超过缓存 1/4”策略要保留。
6. **非官方 API**：所有档位依赖 eapi 非公开接口，平台改协议或风控会失效；需要 fallback 和错误提示，不能静默跳歌。

---

## 8. 验证清单

### 8.1 API 矩阵

用免费 / 黑胶 VIP / 黑胶 SVIP 三种账号，对同一首歌依次请求 10 档，记录：

- `data[0].code`
- `data[0].level`（实际档位）
- `data[0].type`
- `data[0].br` / `sr` / `size`
- `data[0].channelLayout` / `immerseType`
- `freeTrialInfo`

### 8.2 解码矩阵

| 测试 | 期望 |
|---|---|
| `sky` FLAC 5.1 | `ffprobe` 6ch 44.1k；MPV 输出 5.1(side) 或 HRTF 双耳 |
| `jymaster` FLAC 192/24 | 解码为 192k/24bit；设备不支持时重采样并提示 |
| `jyeffect` FLAC 96/24 | 立体声正常，套 HRTF 时应禁用 |
| `dolby` MP4/E-AC-3 | 解 5.1 核心；HDMI 直通时功放显示 Dolby Atmos |
| `vivid` MP4/AV3A | MOLAN 解 12ch 7.1.4；`channelLayout=7.1.4` |
| 内置 Go 播放器 | 只允许 mp3/flac stereo；空间档位自动要求 MPV |

### 8.3 设备矩阵

- Mac 内置扬声器（stereo）
- 3.5mm / USB-C 耳机（HRTF）
- USB DAC（stereo / 多声道）
- HDMI AV 功放（5.1 / 7.1.4 / Atmos 直通）
- Windows WASAPI / Linux ALSA/Pulse（回归）

---

## 9. 参考链接

- [Ncrust 仓库](https://github.com/GuitaristRin/Ncrust)
- [Ncrust SongUrlFetcher](https://github.com/GuitaristRin/Ncrust/blob/master/app/src/main/java/com/takahashirinta/ncrust/player/SongUrlFetcher.kt)
- [Ncrust 音质 fixtures](https://github.com/GuitaristRin/Ncrust/blob/master/spec/fixtures/quality/quality.json)
- [网易云官方品质页](https://music.163.com/g/st-vip-audio-quality?nm_style=sbt&quality=sky)
- [MOLAN-AV3A](https://github.com/baiji6/MOLAN-AV3A)
- [ffmpeg-av3a](https://github.com/WoKee/ffmpeg-av3a)
- [mpv audio-channels 文档](https://mpv.io/manual/master/#options-audio-channels)

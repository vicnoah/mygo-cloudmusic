# 发布 Wagyu Music

宿主在 [vicnoah/mygo-cloudmusic](https://github.com/vicnoah/mygo-cloudmusic) 发版。SDK、内置飞牛和外置插件在 [waygu-music-plugin](https://github.com/waygu-music-plugin) 分别发布。

## 插件

先发布 SDK 标签，再在插件仓库更新 Go module 依赖。飞牛发布源码标签，由宿主引用。外置插件通过[目录仓库的构建脚本](https://github.com/waygu-music-plugin/registry)在 Linux 交叉构建 Windows、macOS 和 Linux 的 amd64、arm64 版本。用维护者持有的 Ed25519 私钥签名发行清单，上传六个 ZIP、清单及校验和。发布并验证下载后，再更新官方目录。网易云的 sherpa-onnx 资源随网易云插件仓库维护。

## Alpha

更新 `mygo.json` 和 `CHANGELOG.md` 中的版本，在 Linux 构建。需要 Go、jq、zip 和 NSIS。

```bash
go test ./internal/...
bash scripts/build-alpha.sh
```

上传生成的六个平台资产及 `SHA256SUMS`，在 GitHub 将该版本标为预发行。macOS 产物是含 `.app` 的 ZIP，Linux 构建省去 Developer ID 签名及 Apple 公证。Windows 安装程序未签名。Alpha 通过 GitHub Release 手动下载。

## 稳定版

提交源码及版本，推送 `v*` 标签。GitHub Actions 在 Ubuntu 上构建全部六个目标并上传到草稿。稳定版的更新清单使用 `MYGO_UPDATER_PRIVATE_KEY` 签名。验收后将草稿发布为最新稳定版。macOS 构建使用包含 `.app` 的归档。

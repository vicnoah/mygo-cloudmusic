# 发布 Wagyu Music

宿主在 [vicnoah/mygo-cloudmusic](https://github.com/vicnoah/mygo-cloudmusic) 发版。SDK、内置飞牛和外置插件在 [waygu-music-plugin](https://github.com/waygu-music-plugin) 分别发布。

## 插件

先推送 SDK 的版本标签，再在插件仓库更新 Go module 依赖。飞牛发布源码标签，由宿主引用。外置插件仓库的 Release workflow 在 Ubuntu 交叉构建 Windows、macOS 和 Linux 的 amd64、arm64 版本，使用仓库 secret `PLUGIN_SIGNING_KEY` 签名发行清单，随后发布正式版。四个发行均完成后，运行[官方目录](https://github.com/waygu-music-plugin/registry)的 Publish catalog workflow。网易云的 sherpa-onnx 资源由其独立的 GitHub Actions 工作流维护。

## Alpha

更新 `mygo.json` 和 `CHANGELOG.md` 中的版本，然后推送匹配的 `v*-alpha.*` 标签。Release workflow 在 Ubuntu 构建全部六个目标、上传校验和，并发布为预发行。也可以在 Linux 先验证本地构建，需要 Go、jq、zip 和 NSIS。

```bash
go test ./internal/...
bash scripts/build-alpha.sh
```

macOS 产物是含 `.app` 的 ZIP，Linux 构建省去 Developer ID 签名及 Apple 公证。Windows 安装程序未签名。Alpha 通过 GitHub Release 手动下载。

## 稳定版

提交源码及版本，推送稳定版的 `v*` 标签。GitHub Actions 在 Ubuntu 上构建全部六个目标，使用 `MYGO_UPDATER_PRIVATE_KEY` 签名更新清单，上传完成后发布为最新稳定版。macOS 构建使用包含 `.app` 的归档。

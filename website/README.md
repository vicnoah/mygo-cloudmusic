# 网站

肥牛音乐的主页，用 TanStack Start 与 Solid 写成。构建时预渲染为静态文件，部署到 Cloudflare（Workers 静态资源，不运行 Worker 脚本）。

```bash
bun install          # 在仓库根目录
bun run dev          # http://localhost:3000
bun run build        # 输出到 website/dist/client
bun run deploy       # 构建并用 wrangler 部署
```

## 插件目录

GitHub Pages 在主分支有网站改动时构建，也每六小时刷新一次。工作流通过 GitHub 话题 `wagyu-music-plugin` 发现公开插件仓库，验证发行清单和产物后生成 `website/src/plugin-directory.json`。仓库中的快照可供本地构建使用。

```bash
GOWORK=off go run ./scripts/plugin-directory > /tmp/wagyu-plugin-directory.json
cp /tmp/wagyu-plugin-directory.json website/src/plugin-directory.json
```

网页通过 `wagyu-music://install-plugin?repo=owner/repo` 将仓库传给宿主。安装前由宿主确认发布者、版本和当前设备兼容性。

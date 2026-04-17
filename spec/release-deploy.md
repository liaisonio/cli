# CLI 发布与部署

## 概述

`@liaisonio/cli` 的发版涉及三个分发渠道，需按顺序完成：

1. **GitHub Release** — 存放跨平台二进制和 SHA256SUMS，CI 自动创建
2. **liaison.cloud CDN** — 自有服务器托管二进制，npm/curl 安装时的**首选**下载源
3. **npm Registry** — 发布 `@liaisonio/cli` 包（thin wrapper + postinstall 下载二进制）

下载优先级：**liaison.cloud > GitHub Releases**。GitHub 在国内访问慢，
liaison.cloud 作为主下载源可显著提升安装速度。

---

## 前置条件

| 工具 | 用途 |
|------|------|
| Go 1.22+ | 交叉编译二进制 |
| `gh` CLI（已认证） | 创建 GitHub Release |
| `npm`（已 `npm login` 到 `@liaisonio` scope） | 发布 npm 包 |
| SSH 免密登录 `root@82.156.194.159`（PROD_MANAGER_HOST） | SCP 上传二进制到 liaison.cloud |

---

## 发版流程

以 `v0.2.5` 为例。

### 1. 提交代码并打 tag

```bash
# 确保 npm/package.json 中的 version 与即将发布的版本一致
# 例如 "version": "0.2.5"

git add -A && git commit -m "chore: bump to v0.2.5"
git tag v0.2.5
git push origin main
git push origin v0.2.5
```

> **注意**：推送 tag 后 GitHub Actions（`.github/workflows/release.yml`）会
> 自动构建二进制、创建 Release 并发布 npm。如果 CI 配置了 `PUBLISH_NPM=true`
> 且有 `NPM_TOKEN` secret，npm 发布也会自动完成。
>
> 如果 CI 未配置自动发布，则需要手动执行以下步骤。

### 2. 构建跨平台二进制

```bash
make release VERSION=v0.2.5
```

产出在 `dist/` 目录：

```
dist/
  liaison-v0.2.5-darwin-amd64
  liaison-v0.2.5-darwin-arm64
  liaison-v0.2.5-linux-amd64
  liaison-v0.2.5-linux-arm64
  liaison-v0.2.5-windows-amd64.exe
  SHA256SUMS
```

### 3. 创建 GitHub Release（如 CI 未自动创建）

```bash
gh release create v0.2.5 \
  dist/liaison-v0.2.5-* \
  dist/SHA256SUMS \
  install.sh \
  --title "v0.2.5" \
  --notes "Release notes here"
```

### 4. 上传到 liaison.cloud

使用 `deploy-liaison.sh` 中的 `sync_remote_cli_releases` 函数，或手动 SCP：

```bash
MANAGER_HOST=82.156.194.159
VERSION=v0.2.5

# 创建远程目录
ssh root@${MANAGER_HOST} "mkdir -p /opt/liaison/nginx/cli-releases/${VERSION}"

# 上传二进制 + SHA256SUMS
scp dist/liaison-${VERSION}-* dist/SHA256SUMS \
    root@${MANAGER_HOST}:/opt/liaison/nginx/cli-releases/${VERSION}/

# 上传 install.sh（供 /install-cli.sh 端点使用）
scp install.sh root@${MANAGER_HOST}:/opt/liaison/nginx/cli-releases/install.sh
```

上传后验证：

```bash
curl -sI "https://liaison.cloud/releases/${VERSION}/SHA256SUMS"
# 应返回 HTTP 200

curl -sI "https://liaison.cloud/install-cli.sh"
# 应返回 HTTP 200
```

### 5. 发布 npm（如 CI 未自动发布）

```bash
cd npm && npm publish --access public
```

验证：

```bash
npm view @liaisonio/cli@0.2.5 version
# 应输出 0.2.5
```

---

## 服务器目录结构

liaison.cloud 的 nginx 通过 Docker volume 挂载提供 CLI 下载服务。

```
/opt/liaison/nginx/cli-releases/
  install.sh                          # curl 安装脚本
  v0.2.3/                             # 每个版本一个目录
    liaison-v0.2.3-darwin-amd64
    liaison-v0.2.3-darwin-arm64
    liaison-v0.2.3-linux-amd64
    liaison-v0.2.3-linux-arm64
    liaison-v0.2.3-windows-amd64.exe
    SHA256SUMS
  v0.2.4/
    ...
  v0.2.5/
    ...
```

### nginx 路由（在 `deploy/nginx/templates/default.conf.template` 中）

```nginx
# curl 安装脚本：curl -fsSL https://liaison.cloud/install-cli.sh | sh
location = /install-cli.sh {
    alias /usr/share/nginx/cli-releases/install.sh;
}

# 二进制下载：https://liaison.cloud/releases/v0.2.5/liaison-v0.2.5-linux-amd64
location /releases/ {
    alias /usr/share/nginx/cli-releases/;
    try_files $uri =404;
}
```

### docker-compose volume 挂载

```yaml
# deploy/docker-compose.yml — nginx 服务
volumes:
  - ./nginx/cli-releases:/usr/share/nginx/cli-releases:ro
```

---

## deploy-liaison.sh 集成

`deploy-liaison.sh` 提供 `sync_remote_cli_releases` 函数：

```bash
sync_remote_cli_releases <host> <user> <port> <version> [cli_dist_dir]
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `host` | 服务器 IP | — |
| `user` | SSH 用户 | — |
| `port` | SSH 端口 | — |
| `version` | 版本号，如 `v0.2.5` | — |
| `cli_dist_dir` | 本地 dist 目录 | `../liaison-cli/dist` |

示例：

```bash
sync_remote_cli_releases 82.156.194.159 root 22 v0.2.5
```

---

## 下载优先级

### npm postinstall（`npm/scripts/install.js`）

```
1. 如果设置了 LIAISON_CLI_MIRROR 环境变量 → 仅使用该 mirror
2. 否则 → 先试 liaison.cloud，失败后 fallback 到 GitHub Releases
```

### curl 安装脚本（`install.sh`）

```
1. 版本解析 → 始终通过 GitHub /releases/latest 重定向获取最新 tag
2. 如果设置了 LIAISON_CLI_RELEASE_BASE → 仅使用该源
3. 否则 → 先试 liaison.cloud，失败后 fallback 到 GitHub Releases
```

用户可通过环境变量覆盖下载源：

```bash
# npm 安装时指定 mirror
LIAISON_CLI_MIRROR=https://my-mirror.example.com/releases npm i -g @liaisonio/cli

# curl 安装时指定源
LIAISON_CLI_RELEASE_BASE=https://my-mirror.example.com/releases \
  curl -fsSL https://liaison.cloud/install-cli.sh | sh
```

---

## 注意事项

1. **nginx 模板文件**：生产环境使用 `deploy/nginx/templates/default.conf.template`，
   **不是** `etc/nginx.conf`。前者包含多个 server 块（子域名代理、OPS 站点、API 子域名、
   主站），后者是开发用的单 server 简化版，**禁止直接上传到生产服务器**。

2. **版本号一致性**：`npm/package.json` 中的 `version` 字段决定了 npm 包版本，
   也决定了 postinstall 下载哪个版本的二进制（`v${pkg.version}`）。必须确保
   git tag、package.json version、dist 目录中的二进制文件名三者一致。

3. **bin/liaison.js 权限**：`npm/bin/liaison.js` 必须有执行权限（`chmod +x`），
   否则 Linux 上 npm 全局安装后运行 `liaison` 会 `Permission denied`。

4. **先上传再发 npm**：npm 包的 postinstall 会立即尝试从 liaison.cloud 下载二进制，
   所以必须确保二进制已上传到 liaison.cloud 后再 `npm publish`。

5. **nginx 重启**：如果是首次添加 `/releases/` 路由或修改了 docker-compose volume，
   需要重启 nginx 容器（`docker restart nginx`）。后续新版本只需 SCP 上传，无需重启。

# CLI Login Flow

## TL;DR

`liaison login` 启动一个本地 HTTP server，打开浏览器让用户完成常规登录（包括
滑块验证码），Web UI 在用户批准后生成一个 PAT 并把它通过 localhost 回传给 CLI。
整个过程用户只需要在浏览器里点一次"批准"。

```
$ liaison login
Opening https://liaison.cloud/cli-auth?... in your browser.
Waiting for authorization... (Ctrl+C to abort)
✓ Authorized as alice@example.com
✓ Token saved to ~/.liaison/config.yaml
```

SSH/headless 环境走 `liaison login --no-browser`，CLI 打印 URL，用户在另一台有
浏览器的机器上打开、登录、复制回显的 token 粘贴回 CLI。

依赖后端的 PAT 支持，见 `liaison-cloud/spec/features/auth/personal-access-tokens.md`。

---

## 用户流程

### 主路径（有浏览器）

1. 用户运行 `liaison login`
2. CLI 做三件事：
   - 生成 32 字节随机 `state` 字符串（base64）
   - 在 `127.0.0.1` 上监听一个随机空闲端口（如 45123），注册 `/callback` handler
   - 构造 URL `https://liaison.cloud/cli-auth?callback=http://127.0.0.1:45123/callback&state=<state>&name=<host>-<date>` 并调用系统命令打开浏览器
3. 用户在浏览器里：
   - 若未登录 → 走常规 `/login`（含滑块验证码）→ 登录后自动带参跳回 `/cli-auth`
   - 看到授权卡片 "Liaison CLI wants to create a token named `cli-foo-20260414`" → 点 [Approve]
   - 浏览器 302 到 `http://127.0.0.1:45123/callback?state=<state>&token=liaison_pat_xxx`
4. CLI 本地 server 收到回调：
   - 验证 `state` 匹配
   - 把 token 塞进内存 channel，返回给浏览器一个 "You can close this tab" 成功页
   - 关闭 HTTP server
5. CLI 主流程从 channel 读到 token：
   - 调 `GET /api/v1/iam/profile_json` 验证 token 可用 & 拿到用户名
   - 写 `~/.liaison/config.yaml`（0600）
   - 打印确认信息，退出 0

全程 5 分钟超时；超时则打印"Authorization timed out, rerun `liaison login`"退出 1。

### 备选路径（headless / `--no-browser`）

```
$ liaison login --no-browser
Visit this URL to authorize the CLI:

  https://liaison.cloud/cli-auth?mode=manual&name=cli-foo-20260414

After approving, copy the displayed token and paste here:
Token: ▊
```

在 `mode=manual` 下，Web UI 不做 302 回调，而是在页面上显示 token 让用户手动
复制。CLI 读 stdin 拿到 token 后同样走验证 + 保存。

区别：

| | 主路径 | `--no-browser` |
|---|---|---|
| 需要本地浏览器 | 是 | 否 |
| 需要本地开端口 | 是 | 否 |
| 用户动作 | 点一次批准 | 点批准 + 手动复制 |
| 适用场景 | 笔记本、工作站 | SSH 到服务器、Docker exec |

---

## CLI 端实现细节

### 端口选择

用 `net.Listen("tcp", "127.0.0.1:0")` 让内核分配空闲端口，从 Listener 拿到实际
端口号，塞进 callback URL。避免硬编码常见端口冲突。

### State 与 CSRF

```go
state := make([]byte, 32)
rand.Read(state)
stateStr := base64.URLEncoding.EncodeToString(state)
```

回调 handler 必须校验 `r.URL.Query().Get("state") == stateStr`，不匹配直接
400。防止攻击者诱导用户访问伪造 URL 把攻击者的 token 写进受害者的 CLI 配置。

### 本地 server 只绑 127.0.0.1

`net.Listen("tcp", "127.0.0.1:0")` —— 不是 `0.0.0.0`。确保同一局域网里其他
机器不能扫到这个临时 server 和拦截回调。

### 浏览器打开

跨平台：

```go
func openBrowser(url string) error {
    switch runtime.GOOS {
    case "darwin":
        return exec.Command("open", url).Start()
    case "linux":
        return exec.Command("xdg-open", url).Start()
    case "windows":
        return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
    }
    return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
}
```

如果 `openBrowser` 失败（比如 linux 没装 xdg-open）→ 降级打印 URL 让用户手动
访问，流程继续等待。

### 超时

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()

select {
case token := <-tokenCh:
    // success
case err := <-errCh:
    // callback reported error (state mismatch / user denied)
case <-ctx.Done():
    return fmt.Errorf("authorization timed out after 5 minutes")
}
```

Ctrl+C 走 signal handler，同样 cancel context，打印 "aborted" 退出。

### 回调成功页

极简 HTML，没有外部资源：

```html
<!doctype html>
<html><head><meta charset="utf-8"><title>Liaison CLI</title>
<style>body{font-family:sans-serif;text-align:center;padding:4em}</style>
</head><body>
<h1>✓ Authorized</h1>
<p>You can close this tab and return to your terminal.</p>
</body></html>
```

不嵌入 token，不做 JS，尽量缩短页面在浏览器里停留的攻击面。

---

## Command 设计

```
$ liaison login [flags]

Flags:
  --no-browser       Print the URL instead of auto-opening a browser
  --token <pat>      Skip the flow and save an already-obtained PAT directly
  --name <name>      Token name shown in the approval page (default: cli-<host>-<date>)
  --server <url>     Override server URL (e.g. staging)

Examples:
  liaison login                           # 主路径
  liaison login --no-browser              # 在 SSH session 里
  liaison login --token liaison_pat_xxx   # 从另一台机器拷贝过来
  liaison login --name ci-runner-42       # CI 环境，自定义名字方便吊销
```

`--token` 作为逃生舱保留，用户能从 Web UI 的 Settings → API Tokens 手动创建
token 再粘贴过来 —— 如果 `/cli-auth` 页面暂时不可用，这条路径仍然工作。

---

## Token 存储

`~/.liaison/config.yaml`：

```yaml
server: https://liaison.cloud
token: liaison_pat_a1b2c3d4...
token_name: cli-host-20260414        # 便于 `liaison logout` 提示用户
token_expires_at: 2026-07-14T10:00Z  # CLI 在过期前 7 天开始提醒
```

文件权限 `0600`，父目录 `0700`。

`liaison logout` 的行为改为：清空 `token`、`token_name`、`token_expires_at`；
**不调后端的 revoke 接口**（用户可能只是想切用户，token 本身还想留着）。
增加 `liaison logout --revoke` 显式吊销当前 token 再清本地配置。

---

## 过期提醒

CLI 在每次命令执行前读 `token_expires_at`，如果距离过期小于 7 天，往 stderr 打一行：

```
warning: your token expires in 3 days (on 2026-07-14). Run `liaison login` to refresh.
```

过期后的调用：后端返回 401 + `reason: token_expired`，CLI 捕获这个 reason 后
打印 "token expired, run `liaison login`" 退出。

---

## 安全考量

| 风险 | 缓解 |
|------|------|
| 局域网抓包拿 token | 回调只绑 127.0.0.1；即使抓到 state 也不知道该把 token 送到哪个进程 |
| 用户在公共电脑上登录忘退出 | `liaison logout --revoke` 一键吊销；Web UI 也能撤销 |
| 回调被重放 | state 一次性，使用后从内存删除 |
| CLI 进程 fork/日志泄露 token | token 只在内存里短暂存在，写盘即清空；日志用 `--verbose` 也只打 URL，不打 Authorization |
| 第三方 CLI 伪造成本 CLI | 无法完全防御，但 Web UI 的授权卡片会显示 token 名字和目标服务器，用户在点"批准"前能看到异常 |

---

## 实现顺序

| 阶段 | 内容 |
|------|------|
| C1 | `liaison login --token <pat>` 手动粘贴路径（等 PAT 后端 P1+P2 完成后就能跑）|
| C2 | 本地 HTTP server + state + 浏览器打开，对接 `/cli-auth` |
| C3 | `--no-browser` headless 降级 |
| C4 | 过期提醒 + `logout --revoke` |

C1 已经能让 agent 开始用 CLI 了；C2 是体验升级；C3 解决 SSH 场景；C4 是长期
稳定性润色。

---

## 和 gh / gcloud 的异同

- **相同**：本地 127.0.0.1 回调 server 是 CLI 界的事实标准
- **差异**：我们的 `/cli-auth` 页面是业务内路由，不是标准 OAuth2 授权端点；
  好处是不用实现完整 OAuth2 服务端（client_id / redirect_uri 白名单 / scope
  协商），坏处是第三方 CLI 没法复用同样的机制。目前只有自家 CLI，可以接受

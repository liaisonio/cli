# Liaison Cloud CLI

## 背景

目前 liaison-cloud 只有 Web 控制台可以管理连接器/应用/入口等资源。随着 LLM
agent 被越来越多地用来执行运维任务，我们需要一个**供 agent 调用**的一等公民
入口。Web UI 要求浏览器 + 滑块验证码 + 人类操作，agent 无法使用；后端虽然提供
REST API，但每个 agent 都要自己封装 HTTP + 认证 + 错误处理，重复而且容易出错。

本 spec 描述一个独立仓库 `github.com/liaisonio/cli` 中的命令行工具，作为
liaison-cloud 的标准自动化入口。

---

## 目标

1. **Agent 优先**：JSON 默认输出，非交互默认路径，全部 flag 都能从 `--help`
   读懂含义和典型用法。
2. **与 Web UI 共用认证**：使用 Web 同一套 JWT token，同一个用户模型，没有
   平行的权限体系。
3. **独立仓库**：不和 liaison-cloud 主仓库耦合，可以独立发版、独立 tag。
4. **安全默认**：destructive 操作必须显式确认；token 文件权限 0600；无隐式的
   `--insecure`。
5. **对人类也可用**：`--output table` 给人看，错误信息明确告诉下一步怎么办。

## 非目标

- 不做 `kubectl` 那种完整的声明式 apply/diff 工作流
- 不做 Terraform provider（那是另一个 repo 的事）
- 不替代 Web UI 里的复杂交互（扫描应用流程、账单支付、滑块登录等）
- MVP 阶段不做 shell 自动补全之外的 UX 糖

---

## 目标用户

按优先级：

1. **LLM agent / 脚本** — 通过 stdin/stdout 和 exit code 消费，不会交互
2. **SRE / 运维** — 在终端快速查状态、临时修改
3. **CI / CD 流水线** — 自动创建/销毁测试用的连接器

---

## 架构

```
┌──────────────┐        ┌──────────────────────────┐
│ agent / user │──exec──▶│  liaison (this CLI)     │
└──────────────┘        │  ┌────────────────────┐  │
                        │  │ cobra commands     │  │
                        │  ├────────────────────┤  │
                        │  │ internal/client    │──┼──HTTPS──▶ liaison.cloud
                        │  ├────────────────────┤  │          /api/v1/...
                        │  │ internal/config    │──┼──read/write──▶ ~/.liaison/config.yaml
                        │  └────────────────────┘  │
                        └──────────────────────────┘
```

- **cobra commands**：`liaison <resource> <action>` 两层结构
- **internal/client**：薄 HTTP 封装，负责 bearer token 注入、envelope 解包、401
  快速失败
- **internal/config**：读写 `~/.liaison/config.yaml`，处理 flag/env/file
  三级覆盖
- **internal/output**：统一 JSON/YAML/table 格式化

CLI 不做本地缓存、不做长连接、不做 WebSocket — 每个命令都是独立的 HTTP 请求，
方便 agent 并发调用和失败重试。

---

## 认证

### 现状痛点

Web UI 的登录接口 `/api/v1/iam/login` 需要一个滑块验证码挑战：

```
POST /api/v1/iam/login_challenge     → challenge_id + 旋转图片
用户手动拖拽滑块                      → rotate_angle, slider_duration_ms
POST /api/v1/iam/login                → identifier + password + challenge proof
                                      ← JWT token
```

agent 无法解滑块。现有后端也没有给用户发放长期 API key 的机制（edge 那套
AccessKey/SecretKey 是给边缘节点用的，不是给人类用户用的）。

### 本期方案：Token 透传

CLI **不实现登录**。token 从 Web 登录后手动拿到（浏览器 DevTools →
LocalStorage → `authorization` 字段），然后通过以下任一方式注入：

| 方式 | 优先级 | 用途 |
|------|--------|------|
| `--token` flag | 最高 | 一次性临时覆盖 |
| `LIAISON_TOKEN` 环境变量 | 中 | agent 首选，不落盘 |
| `~/.liaison/config.yaml` | 低 | 人类开发者的默认路径 |

`liaison login --token <jwt>`：
- 调 `GET /api/v1/iam/profile_json` 校验 token 有效
- 有效则写入 `~/.liaison/config.yaml`（0600）
- 无效则报错退出，不落盘

`liaison logout`：清空 config 里的 token，server 字段保留。

### 未来方案（非本期）

两条路径之一：

1. **Personal Access Token**：后端加一张 `user_api_tokens` 表，Web UI 里让用户
   生成长期 token（可命名、可撤销、带权限 scope）。CLI `liaison login` 变成把这
   种 token 存起来，不再依赖 Web session JWT。
2. **CLI-mode login 绕过 captcha**：后端加一个 `/api/v1/iam/cli_login`，仅接受
   用户名密码（无 challenge），加 IP 限流 + 告警。简单但削弱了 captcha 对密码爆破
   的保护，需要和安全评估后再定。

推荐方案 1，因为它同时解决"长期凭据"和"权限最小化"两个问题。见 [Future Work](#future-work)。

---

## 命令分类

`liaison <resource> <action> [flags]` — 两层结构，对齐 REST 资源。

| 资源 | action 集 | 说明 |
|------|----------|------|
| `login` / `logout` / `whoami` | — | 认证与身份查询 |
| `edge`（别名 `connector`） | `list get create update delete` | 连接器，`update --status stopped` 会触发后端踢下线 |
| `proxy`（别名 `entry`） | `list get create update delete` | 公网入口 |
| `application`（别名 `app`） | `list get create update delete` | 连接器后端的业务应用 |
| `device` | `list get delete` | 连接器所在的物理主机 |
| `version` | — | 打印版本 |

**未来迭代候选**（不在 MVP）：
- `edge kick <id>` — 显式踢下线（当前通过 `edge update --status stopped`
  间接做到；显式命令更直观）
- `firewall list/set/clear` — 入口级防火墙规则
- `traffic list` — 流量监控快照
- `audit list` — 访问审计日志
- `billing snapshot` — 账单状态

---

## 输出契约

### 默认：JSON

```bash
$ liaison edge list
{
  "total": 3,
  "edges": [
    { "id": 100017, "name": "...", "status": 1, "online": 1, ... },
    ...
  ]
}
```

- **stdout** 只写数据；错误和进度写 stderr
- JSON 是**服务端 envelope 里的 data 字段**去封装后的原始结构，保留所有字段
- 保留 `null` 和空字符串，不做"美化"或字段重命名 —— agent 需要稳定的结构

### 其他格式

| `-o` 值 | 用途 |
|---------|------|
| `json`（默认） | agent / `jq` pipeline |
| `yaml` | 人类对照 |
| `table` | 终端快速查看，列是手选的子集 |

table 模式是"有损"的：只显示每种资源最重要的几列（如 edge 的 ID/NAME/STATUS
/ONLINE/APPS/CREATED），完整数据还是应该用 JSON。

### Exit code

- `0` 成功
- `1` 任何错误（认证、网络、API 返回 code ≠ 200、参数错误）

MVP 阶段只有两个 exit code。后续可能细化为 `2`（参数错）、`3`（认证错）、`4`
（API 错）等，如果有 agent 明确需要区分的话。

---

## 配置

### 文件

`~/.liaison/config.yaml`

```yaml
server: https://liaison.cloud
token: eyJhbGciOi...
```

- 权限 `0600`，父目录 `0700`
- 只记录必要字段，不做额外 profile / context（单用户单服务器，MVP 够用）

### 覆盖优先级

flag > env > file > 内置默认值

| 字段 | flag | env | 默认 |
|------|------|-----|------|
| server | `--server` | `LIAISON_SERVER` | `https://liaison.cloud` |
| token  | `--token`  | `LIAISON_TOKEN`  | 无 |
| output | `--output` | — | `json` |

---

## 错误处理

所有错误走 stderr + 非零 exit code。错误消息遵循三个原则：

1. **说清楚发生了什么**：`api error 403: WAF_BLOCKED`
2. **说清楚可能的原因**：`token missing or invalid`
3. **说清楚下一步**：`run \`liaison login\``

例：

```
$ liaison edge list
Error: unauthorized (HTTP 401): token missing or invalid — run `liaison login`
```

不做 retry：网络瞬断/502 这类问题交给调用方（agent 或 shell `until` 循环）处
理，CLI 本身保持确定性。

---

## 安全考量

| 风险 | 缓解 |
|------|------|
| Token 泄露到命令行历史 | 鼓励使用 `LIAISON_TOKEN` 环境变量；文档明说 `--token` 只用于一次性场景 |
| Token 文件被他人读 | 写文件时强制 `0600`，父目录 `0700` |
| 误删资源 | 所有 `delete` 必须 `--yes`；后续考虑加 `--dry-run` |
| 中间人攻击 | 默认校验 TLS；`--insecure` 存在但需要显式传入，且 `--help` 明确说"只用于自签名测试" |
| 日志里打印 token | `--verbose` 只打 method + URL，绝不打 Authorization 头 |

---

## 对 Agent 的使用指引

README 里会专门写一段给 agent 看的（注意第二人称）：

```
If you are an LLM agent:
1. Read LIAISON_TOKEN from the user's secret store.
2. Call `liaison <resource> <action>`; parse stdout as JSON.
3. For discovery, use `liaison <resource> --help`; every flag has examples.
4. Never omit --yes for delete actions; the CLI will refuse without it.
5. Do not retry on error code 1 — read the message first.
```

这段的目的是**让 agent 在第一次调用前就知道 CLI 的边界**，减少试错。

---

## 项目结构

```
liaison-cli/
├── cmd/liaison/main.go             # 入口，只做 cli.NewRootCmd().Execute()
├── internal/
│   ├── cli/                        # cobra 命令，每个资源一个文件
│   │   ├── root.go
│   │   ├── login.go / logout.go / whoami.go
│   │   ├── edge.go / proxy.go / application.go / device.go
│   │   └── version.go
│   ├── client/                     # HTTP 客户端 + API 类型
│   ├── config/                     # ~/.liaison/config.yaml 读写
│   └── output/                     # JSON/YAML/table 格式化
├── spec/cli.md                     # 本文档
├── Makefile                        # build / install / test
├── README.md                       # 用户 & agent 使用文档
├── go.mod                          # module github.com/liaisonio/cli
└── .gitignore
```

依赖最小化：`github.com/spf13/cobra` + `gopkg.in/yaml.v3`。不引入
viper（只用环境变量和 YAML，没必要多一层抽象）、不引入 logrus/zap（stderr
直接 `fmt.Fprintln` 够用）。

---

## Future Work

按优先级从高到低：

1. **Personal Access Token**（后端 + CLI）
   - 后端：`POST /api/v1/iam/tokens` 生成 token，`DELETE /api/v1/iam/tokens/:id`
     撤销，`GET /api/v1/iam/tokens` 列出
   - CLI：`liaison auth token create --name agent-ci` / `liaison auth token list`
     / `liaison auth token revoke <id>`
   - 价值：长期凭据 + 可审计 + 可撤销，替代"从浏览器拷 JWT"的尴尬流程

2. **`edge kick <id>` 显式命令**
   - 当前靠 `update --status stopped` 间接触发；显式命令更直观且可以做成幂等

3. **剩余资源命令**：firewall、traffic metrics、proxy access audit、billing
   snapshot、edge scan applications task

4. **`--dry-run`**：对 `create/update/delete` 打印"将要发送的请求"而不实际调用

5. **Shell completion**：`liaison completion bash|zsh|fish` —— cobra 自带，只是
   需要文档化安装步骤

6. **OCI container image**：`ghcr.io/liaisonio/cli:latest`，方便在 CI 里直接
   `docker run`

---

## 实现顺序

| 阶段 | 范围 | 状态 |
|------|------|------|
| M0 | 项目骨架、config、client、root 命令、login/logout/whoami | ✅ 已完成 |
| M1 | edge/proxy/application/device CRUD | ✅ 已完成 |
| M2 | README + agent 指南 + 初始 commit | ✅ 已完成 |
| M3 | Personal Access Token（后端 + CLI）| 待启动 |
| M4 | 剩余资源命令 + `edge kick` 显式命令 | 待启动 |
| M5 | 发布流程：tag、Goreleaser、OCI 镜像 | 待启动 |

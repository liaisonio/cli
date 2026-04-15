# Liaison Cloud CLI

Official command-line interface for [liaison.cloud](https://liaison.cloud), designed
to be **scripted and agent-friendly**.

- **One-shot bootstrap** — `liaison quickstart` creates a connector + application + public entry in a single call
- **5 Agent Skills** — drop-in Skill files for AI agents (Claude/Cursor/etc.), installable via `npx skills add liaisonio/cli`
- JSON output by default — pipe into `jq` or parse from any LLM agent
- `--output table` for humans, `--output yaml` when you prefer it
- Credentials from env var (`LIAISON_TOKEN`), config file, or explicit `--token` flag
- Browser-based PAT login (or `--no-browser` for headless / SSH)
- Every command has `-h` / `--help` with examples
- Non-interactive by default — destructive operations require `--yes`

## Install

Pick whichever fits your environment. All paths land at the same versioned binary.

### One-line installer (curl, recommended)

```bash
curl -fsSL https://github.com/liaisonio/cli/releases/latest/download/install.sh | sh
```

Auto-detects OS/arch, verifies SHA256, drops the binary in `~/.local/bin` (or
`/usr/local/bin` with sudo if `~/.local/bin` is not writable). Pin a version with
`LIAISON_CLI_VERSION=v0.1.0`.

### npx / npm

```bash
# Run once without installing
npx @liaisonio/cli edge list

# Or install globally
npm i -g @liaisonio/cli
liaison edge list
```

The npm wrapper is a thin Node.js shim that downloads the matching native binary
from the GitHub release on `postinstall`, verifies its SHA256, and execs it.

### Go install

```bash
go install github.com/liaisonio/cli/cmd/liaison@latest
```

Requires Go 1.22+. Best for Go developers who already have `$GOPATH/bin` in their PATH.

### Build from source

```bash
git clone https://github.com/liaisonio/cli
cd cli
make build           # ./bin/liaison           (current platform)
make release         # ./dist/liaison-*        (all 5 platforms + SHA256SUMS)
```

### Verify

```bash
liaison version
```

## Authenticate

The CLI uses long-lived **Personal Access Tokens** (PATs) — `liaison_pat_xxx...`
issued by the Liaison dashboard. Three ways to provide one:

```bash
# 1) Browser flow (recommended for humans)
liaison login
# Opens https://liaison.cloud/dashboard/cli-auth in your default browser,
# you click "Authorize", a fresh PAT is minted and persisted to ~/.liaison/config.yaml.

# 2) SSH / headless / no browser
liaison login --no-browser
# Prints the URL — open it on any device that has a browser, click Authorize,
# the CLI receives the token via a localhost callback.

# 3) Already have a token (CI, agent secrets store)
LIAISON_TOKEN=liaison_pat_a1b2c3... liaison whoami
# Or: liaison login --token liaison_pat_a1b2c3...
```

Precedence (highest wins): `--token` flag → `LIAISON_TOKEN` env → `~/.liaison/config.yaml` → no token.

Tokens can be revoked any time at **liaison.cloud → Settings → API Tokens**, or by running:

```bash
liaison logout
```

## Quick Start

The fastest way to expose a local service:

```bash
# 1) authenticate once
liaison login

# 2) bootstrap a connector + register your service + expose it publicly
liaison quickstart --name mybox \
  --app-name web --app-ip 127.0.0.1 --app-port 8080 --app-protocol http \
  --expose --wait-online 2m

# The output JSON includes:
#   - install_command  → run this on your target host (curl|bash one-liner)
#   - entry.port       → public TCP port (or entry.domain for http)
#   - online_achieved  → whether the connector successfully connected
```

`liaison quickstart` is a single command that:

1. Creates the connector (and returns the install command for the host)
2. Optionally runs the install script locally (`--install`, requires sudo)
3. Optionally polls for the connector to come online (`--wait-online <duration>`)
4. Optionally registers a backend application (`--app-*` flags)
5. Optionally exposes it via a public entry (`--expose`)

See `liaison quickstart --help` for the full flag list.

## Agent Skills

This CLI ships **5 [Skill files](./skills/)** so AI agents (Claude, Cursor, Continue, etc.)
know how to use it without a learning curve. Each skill is a self-contained Markdown
spec with frontmatter — installable with one command:

```bash
# Install all liaison skills into the agent's skills directory
npx skills add liaisonio/cli -y -g
```

| Skill | Purpose |
|-------|---------|
| `liaison-shared` | Auth, install, token precedence, error handling, output format (auto-loaded by other skills) |
| `liaison-quickstart` | One-shot bootstrap: connector + application + entry in a single call |
| `liaison-connector` | Connector lifecycle: create / list / inspect / enable+disable / delete |
| `liaison-application` | Backend service metadata: register / list / update / delete |
| `liaison-entry` | Public exposure: HTTP domains, TCP ports, enable+disable, delete |

After installing the skills, point your agent at `liaison.cloud` and ask it
things like:

- "Set up a public SSH endpoint for my home server"
- "List all my connectors and tell me which ones are offline"
- "Disable connector 100017 — I'm doing maintenance"
- "Expose the local Postgres on 5432 via Liaison"

## Usage

```bash
liaison whoami                                    # who am I logged in as?

# Connectors (edges)
liaison edge list
liaison edge list --online 1                      # only online connectors
liaison edge list --output table
liaison edge get 100017
liaison edge create --name lab-server --description "office lab"
liaison edge update 100017 --status stopped       # disable + kick
liaison edge update 100017 --status running       # re-enable
liaison edge delete 100017 --yes

# Backend applications (IP:port exposed by a connector)
liaison application list
liaison application create --name my-ssh --protocol ssh --ip 192.168.1.10 --port 22 --edge-id 100017
liaison application update 123 --port 2222
liaison application delete 123 --yes

# Entries (public proxies)
liaison proxy list
liaison proxy create --name my-ssh-entry --protocol ssh --application-id 123
liaison proxy update 456 --status stopped
liaison proxy delete 456 --yes

# Devices
liaison device list
liaison device get 789
```

## Global flags

| Flag          | Env              | Description                                           |
|---------------|------------------|-------------------------------------------------------|
| `--server`    | `LIAISON_SERVER` | Liaison base URL (default `https://liaison.cloud`)    |
| `--token`     | `LIAISON_TOKEN`  | JWT bearer token                                      |
| `--config`    |                  | Config file path (default `~/.liaison/config.yaml`)   |
| `--output,-o` |                  | `json` (default), `yaml`, or `table`                  |
| `--insecure`  |                  | Skip TLS verification (self-signed testing only)      |
| `--verbose,-v`|                  | Print each HTTP request to stderr                     |

## Output formats

```bash
liaison edge list                    # pretty JSON (default)
liaison edge list -o yaml            # YAML
liaison edge list -o table           # aligned text table
liaison edge get 100017 | jq .name   # pipe into jq
```

## Exit codes

- `0` — success
- `1` — any error (auth, network, API error, invalid args)

Error messages go to stderr; output goes to stdout — safe to redirect.

## Agent guidance

If you are an LLM agent, here's the minimal path:

**Install** (pick the first one that works in your environment):

```bash
# Option 1: npx — no install, works in any node-equipped sandbox
npx -y @liaisonio/cli@latest <command>

# Option 2: curl one-liner — works on any unix shell with curl
curl -fsSL https://github.com/liaisonio/cli/releases/latest/download/install.sh | sh

# Option 3: go install — if Go toolchain is already present
go install github.com/liaisonio/cli/cmd/liaison@latest
```

**Authenticate**:

```bash
# Best: ask the user to put a Personal Access Token in your secrets store as LIAISON_TOKEN.
# Then every command works without any login flow:
LIAISON_TOKEN=liaison_pat_... liaison whoami
```

**Use**:

1. Always parse stdout as JSON (it's the default output format).
2. Discover commands with `liaison --help` and `liaison <resource> --help`. Every flag
   has a description and examples.
3. Never omit `--yes` for `delete` actions — the CLI refuses to proceed without it.
4. Don't retry on exit code 1 — read the error message on stderr first.
5. Errors go to stderr; data goes to stdout. Safe to redirect them separately.

## License

Apache 2.0

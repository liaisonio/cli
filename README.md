# Liaison Cloud CLI

Official command-line interface for [liaison.cloud](https://liaison.cloud), designed
to be **scripted and agent-friendly**.

- **One-shot bootstrap** — `liaison quickstart` creates a connector + application + public entry in a single call
- **5 Agent Skills** — drop-in Skill files for AI agents (Claude/Cursor/etc.)
- JSON output by default — pipe into `jq` or parse from any LLM agent
- `--output table` for humans, `--output yaml` when you prefer it
- Credentials from env var (`LIAISON_TOKEN`), config file, or explicit `--token` flag
- Browser-based PAT login (or `--no-browser` for headless / SSH)
- Every command has `-h` / `--help` with examples
- Non-interactive by default — destructive operations require `--yes`

## Install

Install the CLI **and** Agent Skills together — the CLI is the tool, the skills
teach your AI agent how to use it.

```bash
# 1) Install the CLI (pick one)
npm i -g @liaisonio/cli                # npm (recommended)
npx -y @liaisonio/cli@latest whoami    # or run without installing
curl -fsSL https://liaison.cloud/install-cli.sh | sh   # or curl one-liner

# 2) Install Agent Skills (for Claude / Cursor / Continue / etc.)
npx skills add liaisonio/cli -y -g
```

That's it. Your AI agent now has both the binary and the knowledge to drive it.

### Alternative install methods

<details>
<summary>Go install</summary>

```bash
go install github.com/liaisonio/cli/cmd/liaison@latest
```

Requires Go 1.22+. Best for Go developers who already have `$GOPATH/bin` in their PATH.
</details>

<details>
<summary>Build from source</summary>

```bash
git clone https://github.com/liaisonio/cli
cd cli
make build           # ./bin/liaison           (current platform)
make release         # ./dist/liaison-*        (all 5 platforms + SHA256SUMS)
```
</details>

### Verify

```bash
liaison version
```

## Agent Skills

The CLI ships **5 [Skill files](./skills/)** so AI agents know how to use it
without a learning curve. Each skill is a self-contained Markdown spec with
frontmatter.

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

Precedence (highest wins): `--token` flag > `LIAISON_TOKEN` env > `~/.liaison/config.yaml` > no token.

Tokens can be revoked any time at **liaison.cloud > Settings > API Tokens**, or by running:

```bash
liaison logout
```

## Quick Start

The fastest way to expose a local service — one command does everything:

```bash
# 1) authenticate once
liaison login

# 2) create connector + install it locally + register app + expose publicly
#    (needs sudo for the install step)
liaison quickstart --name mybox \
  --app-name web --app-ip 127.0.0.1 --app-port 8080 --app-protocol http \
  --expose --install --wait-online 2m
```

This single command:

1. Creates a connector on liaison.cloud
2. Downloads and installs the connector agent on this machine (`--install`, needs sudo)
3. Waits up to 2 minutes for the connector to come online (`--wait-online 2m`)
4. Registers a backend application (`--app-*` flags)
5. Creates a public entry so the service is reachable from the internet (`--expose`)

The output JSON includes everything you need:

```json
{
  "connector": { "id": 100042, "name": "mybox", "access_key": "...", "secret_key": "..." },
  "install_command": "curl ... | bash -s -- ...",
  "installed": true,
  "online_achieved": true,
  "application": { "id": 1, "name": "web", "ip": "127.0.0.1", "port": 8080 },
  "entry": { "id": 10, "name": "web", "port": 34567, "domain": "web-username.liaison.cloud" }
}
```

If you're installing on a **different** machine (not the one running the CLI), omit
`--install` and run the `install_command` from the output on the target host manually:

```bash
liaison quickstart --name mybox \
  --app-name web --app-ip 127.0.0.1 --app-port 8080 --app-protocol http \
  --expose --wait-online 2m

# Then on the target host:
# curl -k -sSL https://liaison.cloud/install.sh | bash -s -- --access-key=... --secret-key=...
```

See `liaison quickstart --help` for the full flag list.

## Step-by-Step Guide

If you need more control than `quickstart`, you can create resources one at a time.

### 1. Create a connector

```bash
liaison edge create --name my-server --description "home lab"
```

Output includes `access_key`, `secret_key` and a one-line install command:

```json
{
  "access_key": "MTc3...",
  "secret_key": "20S...",
  "command": "curl -k -sSL https://liaison.cloud/install.sh | bash -s -- --access-key=... --secret-key=..."
}
```

### 2. Install the connector agent

Run the `command` from step 1 on the target machine (needs `curl` + `bash` + `sudo`):

```bash
curl -k -sSL https://liaison.cloud/install.sh | bash -s -- \
  --access-key=MTc3... --secret-key=20S... \
  --server-http-addr=liaison.cloud --server-edge-addr=liaison.cloud:30012
```

The connector agent will start automatically and connect to liaison.cloud.

### 3. Register a backend application

Once the connector is online, register the service running behind it:

```bash
# SSH service
liaison application create \
  --name my-ssh --protocol ssh \
  --ip 127.0.0.1 --port 22 \
  --edge-id 100017

# HTTP web app
liaison application create \
  --name my-web --protocol http \
  --ip 127.0.0.1 --port 8080 \
  --edge-id 100017
```

Supported protocols: `tcp`, `http`, `ssh`, `rdp`, `mysql`, `postgresql`, `redis`, `mongodb`.

### 4. Expose via a public entry

```bash
# SSH entry — gets an auto-allocated public port
liaison proxy create --name my-ssh-entry --protocol ssh --application-id 100015
# => access at: ssh -p <port> user@liaison.cloud

# HTTP entry — gets a subdomain like myapp-username.liaison.cloud
liaison proxy create --name my-web-entry --protocol http --application-id 100038
# => access at: https://my-web-entry-username.liaison.cloud
```

### All-in-one (equivalent of the 4 steps above)

```bash
# On the same machine — install + wait + app + expose in one shot
liaison quickstart --name mybox \
  --app-name web --app-ip 127.0.0.1 --app-port 8080 --app-protocol http \
  --expose --install --wait-online 2m

# On a different machine — omit --install, run install_command manually
liaison quickstart --name mybox \
  --app-name web --app-ip 127.0.0.1 --app-port 8080 --app-protocol http \
  --expose --wait-online 2m
```

## Command Reference

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

## License

Apache 2.0

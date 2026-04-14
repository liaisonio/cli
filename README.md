# Liaison Cloud CLI

Official command-line interface for [liaison.cloud](https://liaison.cloud), designed
to be **scripted and agent-friendly**.

- JSON output by default — ready for piping into `jq` or parsing by LLM agents
- `--output table` for humans, `--output yaml` when you prefer it
- Credentials from env var (`LIAISON_TOKEN`), config file, or explicit `--token` flag
- Every command has `-h` / `--help` with examples
- Non-interactive by default: destructive operations require `--yes`

## Install

```bash
go install github.com/liaison-cloud/cli/cmd/liaison@latest
```

Or build from source:

```bash
git clone https://github.com/liaison-cloud/cli
cd cli
make build   # produces ./bin/liaison
```

## Authenticate

The CLI accepts a JWT bearer token issued by liaison.cloud. For now the slider-captcha
login flow used by the web UI is not supported headlessly — you need to obtain the
token out of band:

1. Log in to [liaison.cloud](https://liaison.cloud) in your browser.
2. Open DevTools → Application → Local Storage → copy the `authorization` value.
3. Persist it:

   ```bash
   liaison login --token eyJhbGciOi...
   ```

   This writes `~/.liaison/config.yaml` (mode 0600) and verifies the token against
   `/api/v1/iam/profile_json`.

Alternatively, skip the config file entirely and pass the token per-invocation:

```bash
LIAISON_TOKEN=eyJhbGciOi... liaison edge list
# or
liaison --token eyJhbGciOi... edge list
```

Precedence (highest wins): `--token` flag → `LIAISON_TOKEN` env → config file → built-in default.

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

If you are an LLM agent, the simplest way to use this CLI is:

1. Set `LIAISON_TOKEN` in your environment once (via the user's secrets store).
2. Call `liaison <resource> <action> [flags]` — always use the default JSON output and parse it.
3. Discover capabilities with `liaison <resource> --help` if unsure; every flag is documented with examples.
4. Never omit `--yes` for `delete` actions — the CLI will refuse to proceed without it.

## License

Apache 2.0

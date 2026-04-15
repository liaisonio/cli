---
name: liaison-entry
version: 0.1.0
description: "Liaison Cloud entries (a.k.a. proxies): create, list, update, enable/disable, delete the public-facing endpoints that expose a connector's applications to the internet. An entry binds an application to a public port (TCP) or domain (HTTP). Use this skill for intents like 'expose my app via a public URL', 'open port 8080 to the world', 'list my entries', 'take entry 10 offline'. Always read ../liaison-shared/SKILL.md first."
metadata:
  requires:
    bins: ["liaison"]
  cliHelp: "liaison proxy --help"
---

# liaison-entry

**CRITICAL — read [`../liaison-shared/SKILL.md`](../liaison-shared/SKILL.md) first.**

## Concept

An **entry** (internally also called "proxy") is a public-facing endpoint that routes traffic to one **application** behind a connector.

```
internet  →  liaison.cloud:<port>  →  Liaison Cloud routing  →  connector  →  application IP:port
   or
internet  →  https://<domain>/     →  Liaison Cloud routing  →  connector  →  application IP:port
```

Entries come in two flavours, decided by the application's `protocol`:

| Protocol | Public exposure | Configured via |
|----------|-----------------|----------------|
| `http` | Public HTTPS URL like `https://myapp.example.com` | `--domain` |
| `tcp` / `ssh` / `mysql` / `postgresql` / `redis` / `mongodb` / `rdp` | Public TCP port like `liaison.cloud:34567` | `--port` (or omit for auto-allocate) |

An entry has its own `status`:
- `running` — actively accepting traffic
- `stopped` — taken offline without deleting

## Commands

All commands under `liaison proxy`. Aliases: `proxies`, `entry`, `entries`. (The CLI uses "proxy" internally for historical reasons; the user-facing dashboard says "entry".)

### List

```bash
liaison proxy list                      # all entries
liaison proxy list --name my-app        # name substring filter
liaison proxy list --page 2             # pagination
liaison proxy list -o table             # human-readable
```

**Response shape**:
```json
{
  "total": 2,
  "proxies": [
    {
      "id": 10,
      "name": "my-web",
      "protocol": "http",
      "domain": "myapp.example.com",
      "port": 0,
      "status": "running",
      "application_id": 5,
      "edge_id": 100017,
      "created_at": "..."
    }
  ]
}
```

### Get

```bash
liaison proxy get 10
```

### Create

`--name` and `--application-id` are required. Other flags depend on the protocol:

```bash
# HTTP entry → expose via a public domain
liaison proxy create \
  --name my-web \
  --protocol http \
  --domain myapp.example.com \
  --application-id 5

# TCP-style entry → expose on a server-allocated public port
liaison proxy create \
  --name my-ssh \
  --protocol ssh \
  --application-id 6
# server picks an available port and returns it in the response

# TCP-style entry → ask for a specific port
liaison proxy create \
  --name my-mysql \
  --protocol mysql \
  --port 13306 \
  --application-id 7
```

**Picking `--port`:**
- Leave it `0` (the default) for `tcp`-like protocols → server auto-allocates an available port
- Set a specific port only when the user asks for one (e.g. they have a firewall rule pinned to a port)
- For `http`, leave port 0 and use `--domain` instead

**Picking `--domain`:**
- Required for `http` protocol; ignored for everything else
- Must be a domain the user controls and has DNS pointing at Liaison's edge nodes
- Subdomain example: `myapp.example.com`

### Update

Only pass the fields you want to change. Most useful for taking entries on/off:

```bash
liaison proxy update 10 --status stopped       # take offline
liaison proxy update 10 --status running       # bring back
liaison proxy update 10 --port 14000           # change auto-allocated port
liaison proxy update 10 --description "new"    # rename / re-describe
```

`--status` accepts the literal strings `running` and `stopped` (NOT integers — different from connector update which takes numbers/keywords).

### Delete

**Requires `--yes`.**

```bash
liaison proxy delete 10 --yes
```

The associated application is left alone; only the public exposure is removed.

## Intent → command mapping

| User says | Command |
|-----------|---------|
| "Expose my web app at example.com" | First find the application id with `liaison application list`, then `liaison proxy create --name web --protocol http --domain example.com --application-id <id>` |
| "Open SSH access to my home server" | `liaison proxy create --name ssh --protocol ssh --application-id <id>` (port auto-allocated) |
| "What's my SSH port?" | `liaison proxy list -o table` then read the `PORT` column for the ssh entry |
| "Take entry 10 offline temporarily" | `liaison proxy update 10 --status stopped` |
| "Bring entry 10 back" | `liaison proxy update 10 --status running` |
| "Remove the public exposure for X" | `liaison proxy delete <id> --yes` (the application stays) |

## Common mistakes to avoid

- **Creating an entry without an application first.** Applications and entries are separate resources. If `liaison application list --edge-id <connector>` returns nothing, you need to `liaison application create ...` BEFORE creating the entry.
- **Setting both `--port` and `--domain`** on the same entry. Pick one based on protocol — http uses domain, everything else uses port.
- **Confusing entry status with connector status.** They're independent. Stopping an entry takes ONE service offline; stopping a connector takes ALL services on that host offline.
- **Reusing `--port`.** Two entries can't bind the same public port. The server returns a clear error if you try.
- **Sharing the `--domain`** of a domain you don't actually control. The DNS still has to point at Liaison's edge nodes — otherwise the entry exists in the database but no real traffic ever reaches it.

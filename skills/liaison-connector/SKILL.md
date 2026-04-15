---
name: liaison-connector
version: 0.1.0
description: "Liaison Cloud connectors (a.k.a. edges): create, list, inspect, rename, enable/disable (kicks the live tunnel), and delete. A connector is the agent process installed on a user's host that tunnels traffic to the Liaison cloud. Use this skill for intents like 'create a new connector', 'list my connectors', 'take connector X offline', 'disable connector', 'what's connector 100017's status'. Always read ../liaison-shared/SKILL.md first."
metadata:
  requires:
    bins: ["liaison"]
  cliHelp: "liaison edge --help"
---

# liaison-connector

**CRITICAL — read [`../liaison-shared/SKILL.md`](../liaison-shared/SKILL.md) first** for auth, output format, and safety rules.

## Concept

A **connector** (internally also called "edge") is:
- A binary running on a user's machine
- Authenticated by an AccessKey + SecretKey pair issued at creation time
- Tunnelling traffic between that machine and the Liaison cloud
- The parent of one or more **applications** (backend services)
- Either `online` (connected) or `offline`, AND either `running` (status=1, allowed to connect) or `stopped` (status=2, rejected + kicked)

**Two independent state axes — don't confuse them:**

| Axis | Field | Values | Managed by |
|------|-------|--------|-----------|
| Connectivity | `online` | 1 = online, 2 = offline | Server — flips automatically when the tunnel connects/disconnects |
| Authorization | `status` | 1 = running, 2 = stopped | User — `stopped` immediately kicks the live tunnel and blocks reconnection |

When a user says "disable connector X" or "take connector X offline" they almost always mean **set status to stopped**, not wait for the physical tunnel to drop. The CLI models this as `liaison edge update <id> --status stopped`.

## Commands

All commands under `liaison edge`. Aliases: `connector`, `edges`, `connectors` (they all resolve to the same subcommand).

### List

```bash
liaison edge list                           # all connectors, JSON
liaison edge list --online 1                # only currently connected
liaison edge list --online 2                # only disconnected
liaison edge list --name prod               # substring filter on name
liaison edge list --page 2 --page-size 20   # pagination
liaison edge list -o table                  # aligned text for humans
```

**Response shape** (JSON):
```json
{
  "total": 3,
  "edges": [
    {
      "id": 100017,
      "name": "prod-server-01",
      "description": "hosting /etc/...",
      "status": 1,
      "online": 1,
      "application_count": 5,
      "created_at": "2026-04-10 12:34:56",
      "updated_at": "2026-04-14 08:02:11"
    },
    ...
  ]
}
```

### Get

```bash
liaison edge get 100017
```

Returns a single connector object with the same fields as a list entry.

### Create

```bash
liaison edge create --name my-lab-box --description "home lab mini PC"
```

**Response shape** — this is the ONLY time AccessKey/SecretKey are exposed; you cannot retrieve them again later:
```json
{
  "access_key": "MTc3...",
  "secret_key": "20S...",
  "command": "curl -k -sSL https://liaison.cloud/install.sh | bash -s -- --access-key=... --secret-key=... --server-http-addr=liaison.cloud --server-edge-addr=liaison.cloud:30012"
}
```

The `command` field is a one-liner the user runs on the **target host** (not on the machine running this CLI) to download and start the connector agent. Copy the string to the user — do NOT run it yourself unless you are certain the CLI is on the same machine that will host the connector.

### Update

Only pass flags you actually want to change — omitted flags are left alone.

```bash
# Rename
liaison edge update 100017 --name new-name

# Update description
liaison edge update 100017 --description "moved to rack 3"

# Take offline + kick the live tunnel
liaison edge update 100017 --status stopped

# Re-enable
liaison edge update 100017 --status running
```

`--status stopped` has three server-side side effects, in order:
1. `edges.status` flips to 2
2. The server calls frontier `KickEdge(id)` which closes the live tunnel
3. The connector's reconnect attempts are rejected at the auth layer (returns `edge is disabled`)

`--status running` reverses the auth-layer block — the connector will reconnect on its next retry tick (up to ~30 seconds).

### Delete

**Requires `--yes` for non-interactive safety.**

```bash
liaison edge delete 100017 --yes
```

Cascades: deletes the connector, its applications, its entries, and its pending tasks. There is no undo. Always confirm the target id with the user before running.

## Intent → command mapping (for agents)

| User says | Command |
|-----------|---------|
| "Create a connector for my home server" | `liaison edge create --name <name>` |
| "Show me my connectors" | `liaison edge list -o table` |
| "Is connector 100017 online?" | `liaison edge get 100017` then read `.online` (1=yes, 2=no) |
| "Disable / kick / take offline connector X" | `liaison edge update <id> --status stopped` |
| "Re-enable / start / bring back connector X" | `liaison edge update <id> --status running` |
| "Delete connector X" | confirm → `liaison edge delete <id> --yes` |
| "Rename connector X to Y" | `liaison edge update <id> --name Y` |

## What NOT to do

- **Do not invent ids.** Always list or accept an id from the user. IDs are big integers (~100000+) — guessing is not safe.
- **Do not execute the `install_command`** on your own machine. It installs a connector agent bound to a specific network identity; running it locally could register your CLI host as a connector you didn't intend.
- **Do not bulk-delete** without explicit per-id confirmation. Ask "delete connectors A, B, C?" and wait for an explicit yes.
- **Do not chain create + install + app + entry manually** — use the `liaison-quickstart` skill instead, which does all of it in one call and polls for online state.

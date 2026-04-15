---
name: liaison-quickstart
version: 0.1.0
description: "Liaison Cloud one-shot bootstrap: create a connector + register an application + expose it via a public entry, in a single command. Use this skill when a user asks 'set up a new tunnel for my SSH server', 'I want to expose my local web app to the internet', 'create a new connector for my home box', or any other green-field 'go from nothing to a working public endpoint' request. Always read ../liaison-shared/SKILL.md first."
metadata:
  requires:
    bins: ["liaison"]
  cliHelp: "liaison quickstart --help"
---

# liaison-quickstart

**CRITICAL — read [`../liaison-shared/SKILL.md`](../liaison-shared/SKILL.md) first.**

## When to use this skill

Use `liaison quickstart` whenever the user is bootstrapping from zero. It does in one call what would otherwise take 3 separate API calls (connector create → application create → entry create), it polls for the connector to come online, and it returns a single structured JSON result so you can immediately tell the user "your service is now reachable at X".

**Don't** use quickstart when the user already has a connector and just wants to add an application or entry. For those, use the `liaison-application` or `liaison-entry` skills directly — quickstart will create a NEW connector each time it's called, which is almost never what the user wants in incremental flows.

## The five layers of `quickstart`

The command has five stages, each gated on a flag. Skip whichever you don't need.

| Stage | Required flag | Purpose |
|-------|--------------|---------|
| 1. Create connector | always runs | Create an edge record on the server, return ak/sk + install command |
| 2. Run install script locally | `--install` | Shell out to `bash -c` and execute the install one-liner on the **local** machine. Only safe when the CLI is on the same host that should be a connector. Needs sudo. |
| 3. Wait for online | `--wait-online <duration>` | Poll the edge until `online == 1` or timeout. Required if the next stages need a working tunnel. |
| 4. Register application | `--app-name` + `--app-ip` + `--app-port` (+ optional `--app-protocol`) | Create the backend application metadata |
| 5. Expose via entry | `--expose` (+ optional `--entry-name`/`--entry-port`/`--entry-domain`) | Create a public entry pointing at the application |

## Recipes

### Recipe A — just create the connector

User intent: "Create a new connector called mybox." You then hand the install command to the user to run on their host.

```bash
liaison quickstart --name mybox
```

Result you get back:
```json
{
  "connector": { "id": 100042, "name": "mybox", "access_key": "...", "secret_key": "..." },
  "install_command": "curl -k -sSL https://liaison.cloud/install.sh | bash -s -- --access-key=... ...",
  "installed": false,
  "online_waited": false,
  "online_achieved": false,
  "next_steps": [ "Run install_command on the target host (needs curl + bash + sudo)." ]
}
```

**Your job afterwards**: paste `install_command` to the user with the instruction "run this on the machine you want to make a connector".

### Recipe B — connector + application, no public exposure yet

The user wants to register a service but is not ready to publish it.

```bash
liaison quickstart --name mybox \
  --app-name ssh --app-ip 127.0.0.1 --app-port 22 --app-protocol ssh
```

The application is created on the server. Until the user runs the install command on their host, the application metadata exists but has no live connector to forward traffic.

### Recipe C — full bootstrap with public exposure

The user wants to go from zero to "I can SSH from anywhere" / "my web app has a public URL".

```bash
liaison quickstart --name mybox \
  --app-name ssh --app-ip 127.0.0.1 --app-port 22 --app-protocol ssh \
  --expose --wait-online 2m
```

The flow:
1. Create connector → return ak/sk + install command
2. Print "next step: run this command on your host" + WAIT for them to run it
3. Poll for up to 2 minutes until the connector is online
4. Once online, create the application
5. Create the entry pointing at the application
6. Return the entry's allocated port/domain in the result so you can tell the user "ssh -p <port> user@liaison.cloud"

If the user is on the same machine as the CLI, you can collapse steps 2 + 3 by adding `--install`:

```bash
liaison quickstart --name mybox --install \
  --app-name ssh --app-ip 127.0.0.1 --app-port 22 --app-protocol ssh \
  --expose --wait-online 2m
```

### Recipe D — HTTP web app with a custom domain

```bash
liaison quickstart --name mybox \
  --app-name web --app-ip 127.0.0.1 --app-port 8080 --app-protocol http \
  --expose --entry-domain myapp.example.com --wait-online 2m
```

The user's DNS still has to point `myapp.example.com` at Liaison's edge nodes — quickstart does not configure DNS.

## Result shape (full)

```json
{
  "connector": {
    "id": 100042,
    "name": "mybox",
    "access_key": "MTc3NjI2OTc4NjM1MTAx",
    "secret_key": "20S6Yr8EralcxwWGOQyjgAnaqEfmlx0odRJN4x5UHIw",
    "description": "..."
  },
  "install_command": "curl ... | bash -s -- ...",
  "installed": true,                 // true if --install was passed and bash exit was 0
  "online_waited": true,             // true if --wait-online > 0
  "online_achieved": true,           // true if the connector reported online before the deadline
  "application": {                   // only present if --app-name was passed
    "id": 1,
    "name": "ssh",
    "protocol": "ssh",
    "ip": "127.0.0.1",
    "port": 22
  },
  "entry": {                         // only present if --expose was passed
    "id": 10,
    "name": "ssh",
    "protocol": "ssh",
    "port": 34567,
    "domain": ""
  },
  "next_steps": [
    "Connector did not come online within 2m — check the install succeeded on the host."
  ]
}
```

`next_steps` is a hand-curated list of strings — surface them to the user verbatim. They're often the difference between a working bootstrap and a confused user.

## Hard rules for agents

- **Never pass `--install` to a server-side machine that is not the user's intended connector host.** Doing so installs a long-running daemon under a service identity the user didn't intend to create.
- **Always check `online_achieved` before declaring success.** If false, the application/entry exist but no traffic will flow until the user runs the install command.
- **Do not call quickstart twice for the same logical setup.** Each call creates a NEW connector (and new ak/sk). If a previous call created a connector but the user closed the tab, find it with `liaison edge list` instead.
- **Always echo `install_command` to the user** — they need it to actually bring the connector online.
- **Always echo allocated ports/domains** from `entry.port` / `entry.domain` — these are how the user actually reaches their service.

## When NOT to use quickstart

- The user already has a connector and wants to add a second application → use `liaison-application`
- The user already has an application and wants a second public exposure → use `liaison-entry`
- The user wants to LIST or INSPECT existing resources → use `liaison-connector`/`-application`/`-entry`
- The user wants to delete or disable something → use `liaison-connector` or `liaison-entry`

# Hitch Walkthrough

This walkthrough gets Hitch from a fresh checkout to a working Codex proof of concept:

1. install or build `hitch`
2. start the local Hitch server
3. install Codex lifecycle hooks
4. run a no-op observer handler against a sample Codex event
5. inspect the persisted audit record

The proof uses the built-in sample handler:

```sh
hitch handler noop-observer
```

That handler reads the normalized Hitch event envelope from stdin and returns:

```json
{"status":"ok","decision":{"behavior":"none"}}
```

It observes the payload and does not change Codex behavior.

## Prerequisites

- macOS or Linux shell
- `go` on `PATH`
- `git` on `PATH` if installing from source URL
- Codex CLI on `PATH` for automatic Codex hook installation

Check prerequisites:

```sh
go version
git --version
codex --version
```

If `codex --version` fails, you can still run the manual Hitch API example, but `hitch-client install --only codex` will report Codex as unavailable.

## Option A: Install Hitch from latest source

Run the installer:

```sh
curl -fsSL https://raw.githubusercontent.com/sagebynature/hitch/main/install.sh | sh
```

The installer:

- clones the Hitch repository
- builds `./cmd/hitch` and `./cmd/hitch-client`
- installs the binaries to `~/.local/bin/hitch` and `~/.local/bin/hitch-client` by default
- prints `hitch --version` and `hitch-client --version`
- offers to run hook setup when stdin is interactive

If `~/.local/bin` is not first on `PATH`, follow the installer output and restart your shell.

Verify the binary:

```sh
hitch --version
```

Expected output:

```text
hitch 0.1.0
```

## Option B: Build Hitch from a local checkout

From the repository root:

```sh
make build
```

This writes the binary to:

```text
bin/hitch
```

For local walkthrough commands, either call `./bin/hitch` directly or put `bin` first on `PATH`:

```sh
export PATH="$PWD/bin:$PATH"
hitch --version
```

The default sample handler config uses `hitch handler noop-observer`, so `hitch` must be resolvable on `PATH` when the server runs.

## Start the Hitch server

Use the config that matches how you installed Hitch.

If you installed with `install.sh`, use the seeded user config:

```sh
hitch serve --config ~/.config/hitch/config.toml
```

If you are running from a repository checkout, use the development config:

```sh
hitch serve --config config/default.config.toml
```

Use that same config path later with `hitch inspect-event`.

The default server listens on:

```text
http://127.0.0.1:8799
```

The default config stores audit records at:

```text
~/.local/share/hitch/events.sqlite
```

The default config also includes the no-op observer handler:

```toml
[handlers.noop_observer]
command = ["hitch", "handler", "noop-observer"]
events = ["*"]
mode = "sync"
timeout_ms = 1000
on_error = "fail_open"
on_timeout = "fail_open"
```

Keep this server running while you run the remaining commands in another shell.

## Check server health

In another shell:

```sh
hitch status --json
hitch doctor --json
```

You can also call the health endpoint directly:

```sh
curl -sS http://127.0.0.1:8799/v1/health
```

Expected output:

```json
{"status":"ok"}
```

## Install the Codex hook configuration

Preview the install first:

```sh
hitch-client install --only codex --dry-run --json
```

Then install:

```sh
hitch-client install --only codex --yes --json
```

Hitch writes managed Codex command hooks to:

```text
~/.codex/hooks.json
```

The installer backs up an existing `hooks.json` before changing it.

Hitch installs one sync command hook for every supported Codex lifecycle event:

- `SessionStart`
- `SubagentStart`
- `UserPromptSubmit`
- `PreToolUse`
- `PermissionRequest`
- `PostToolUse`
- `PreCompact`
- `PostCompact`
- `SubagentStop`
- `Stop`

Each installed command has this shape, with the event name changed per lifecycle event:

```sh
hitch-client -harness codex -event PreToolUse -sync
```

`hitch-client` reads Codex hook JSON from stdin, forwards it to Hitch, and writes the Codex-native response JSON to stdout.

## Trust hooks in Codex

Codex requires review for non-managed command hooks.

Open Codex and run:

```text
/hooks
```

Review and trust the Hitch hook definitions. Until trusted, Codex may list the hooks but skip running them.

For one-off automation where you already trust the hook file, Codex also supports its own bypass flag:

```sh
codex --dangerously-bypass-hook-trust
```

Use the normal `/hooks` review flow for routine setup.

## Run a manual end-to-end example

This example sends a fake Codex `SessionStart` payload through Hitch. It exercises the same path that an installed Codex hook uses:

```text
Codex hook JSON -> hitch-client -> Hitch API -> normalized event -> noop_observer handler -> Codex-native response -> audit store
```

Run:

```sh
printf '%s\n' '{
  "session_id": "demo-session",
  "transcript_path": null,
  "cwd": "/tmp",
  "hook_event_name": "SessionStart",
  "model": "demo-model",
  "permission_mode": "default",
  "source": "startup"
}' | hitch-client -harness codex -event SessionStart -sync
```

Expected stdout:

```json
{}
```

`{}` means the no-op observer returned no Codex control decision, so Codex should continue normally.

## Run an API example with an inspectable event ID

The adapter prints only the native harness response. To get the Hitch IDs for inspection, call the sync dispatch API directly:

```sh
curl -sS -X POST http://127.0.0.1:8799/v1/dispatch-sync \
  -H 'content-type: application/json' \
  -d '{
    "harness": "codex",
    "source_event_type": "PreToolUse",
    "source_payload": {
      "session_id": "demo-session",
      "transcript_path": null,
      "cwd": "/tmp",
      "hook_event_name": "PreToolUse",
      "model": "demo-model",
      "turn_id": "demo-turn",
      "permission_mode": "default",
      "tool_name": "Bash",
      "tool_use_id": "demo-tool",
      "tool_input": {
        "command": "pwd"
      }
    },
    "hitch_client_version": "manual-curl"
  }'
```

Expected response shape:

```json
{
  "event_id": "evt_...",
  "normalized_event_id": "norm_...",
  "aggregate": {
    "decision": {
      "behavior": "none"
    },
    "handler_results": [
      {
        "status": "ok",
        "decision": {
          "behavior": "none"
        }
      }
    ]
  },
  "native_response": {}
}
```

Copy `normalized_event_id`, then inspect the persisted records with the same config path used by the server.

For an `install.sh` setup:

```sh
hitch inspect-event --config ~/.config/hitch/config.toml norm_...
```

For a repository checkout:

```sh
hitch inspect-event --config config/default.config.toml norm_...
```

Expected inspection includes:

- one inbound Codex event
- one normalized Hitch event
- one `noop_observer` handler invocation
- one native response record

The normalized event should have:

```json
"hitch_event_type": "tool.requested"
```

The handler invocation should have:

```json
"handler_name": "noop_observer"
```

and a persisted decision like:

```json
{"behavior":"none"}
```

## Run the example from Codex itself

After `hitch serve` is running and the hooks are trusted:

1. Start Codex in any workspace.
2. Submit a prompt.
3. Run a simple tool-using request, such as asking Codex to inspect the current directory.
4. Stop the turn normally.

The installed hooks should observe lifecycle events such as:

- `SessionStart`
- `UserPromptSubmit`
- `PreToolUse`
- `PostToolUse`
- `Stop`

Because the default handler is a no-op observer, Codex behavior should not change.

## Troubleshooting

### `hitch-client install --only codex` reports Codex unavailable

Cause: `codex` is not on `PATH`.

Fix:

```sh
codex --version
```

Install Codex or update `PATH`, then rerun:

```sh
hitch-client install --only codex --yes --json
```

### Codex lists hooks but does not run them

Cause: Codex requires hook trust review.

Fix: open Codex and run:

```text
/hooks
```

Trust the Hitch hook entries.

### `hitch-client` prints `{}` even when Hitch is down

For sync hooks, `hitch-client` fails open. If Hitch is unreachable or returns no native response, the client emits a harness-native no-op response.

Check the server:

```sh
curl -sS http://127.0.0.1:8799/v1/health
```

### Handler invocation is missing from inspection

Common causes:

- The server was not running when the hook fired.
- The config used to start the server did not include `handlers.noop_observer`.
- `hitch` was not on `PATH` for the server process.

Check the config passed to `hitch serve` and verify:

```sh
hitch handler noop-observer <<'JSON'
{"hitch_version":"0.1.0","event_id":"evt_demo","received_at":"2026-06-04T00:00:00Z","harness":"codex","source_event_type":"SessionStart","source_payload":{},"hitch_event_type":"session.started","payload":{}}
JSON
```

Expected output:

```json
{"status":"ok","decision":{"behavior":"none"}}
```

### Remove Hitch Codex hooks

Run:

```sh
hitch-client uninstall --only codex --yes --json
```

This removes only Hitch-managed Codex hook commands and leaves user-owned hooks in place.

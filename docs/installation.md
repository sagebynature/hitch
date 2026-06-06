# Installation

Install from latest source:

```sh
curl -fsSL https://raw.githubusercontent.com/sagebynature/hitch/main/scripts/install.sh | sh
```

The source installer checks for `go`, checks for `git` when `HITCH_SOURCE_DIR` is not provided, asks which install mode to use when `/dev/tty` is available, builds the selected binaries from source, installs them to `$HITCH_INSTALL_DIR` or `~/.local/bin`, verifies selected binary versions, and performs the selected setup steps.

Install modes:

| Mode | Behavior |
|---|---|
| `all` | Default. Install `hitch` and `hitch-client`, seed `~/.config/hitch/config.toml` with `hitch config init`, prompt for a Hitch server URL, and offer to run hook setup. |
| `server` | Install only `hitch` and seed server config. Skip `hitch-client`, server URL prompting, and hook setup. |
| `client` | Install only `hitch-client`, prompt for a Hitch server URL, and offer to run hook setup. Skip `hitch` and server config initialization. |

For non-interactive installs, set `HITCH_INSTALL_MODE` explicitly or omit it to keep the default `all` behavior:

```sh
curl -fsSL https://raw.githubusercontent.com/sagebynature/hitch/main/scripts/install.sh | HITCH_INSTALL_MODE=all sh
curl -fsSL https://raw.githubusercontent.com/sagebynature/hitch/main/scripts/install.sh | HITCH_INSTALL_MODE=server sh
curl -fsSL https://raw.githubusercontent.com/sagebynature/hitch/main/scripts/install.sh | HITCH_INSTALL_MODE=client HITCH_URL=http://127.0.0.1:8799 sh
```

Set `HITCH_SKIP_HOOK_INSTALL=1` to install binaries and skip hook setup in `all` or `client` mode.

The current `hitch-client install` command detects Codex, Hermes, Pi, OMP, and OpenCode binaries on `PATH`, installs Hitch command hooks for every supported Codex lifecycle event into `~/.codex/hooks.json`, installs Hitch shell hooks for supported Hermes events into `~/.hermes/config.yaml`, installs a managed Pi extension at `~/.pi/agent/extensions/hitch/index.ts`, installs a managed OMP extension at `~/.omp/agent/extensions/hitch/index.ts`, and installs a managed OpenCode plugin at `~/.config/opencode/plugins/hitch.ts`. Managed shell hook commands execute `hitch-client` directly. `hitch-client uninstall` also recognizes legacy managed hooks that used `hitch adapter`.

Dry run detected supported harnesses:

```sh
hitch-client install --dry-run --json
```

Dry run all harnesses:

```sh
hitch-client install --all --dry-run --json
```

Install selected hooks. Without `--url`, installed hooks resolve the server URL at runtime from `HITCH_URL`, `~/.config/hitch/config.toml`, then `http://127.0.0.1:8799`. Use `--url` to pin a remote or non-default Hitch server directly into generated hooks/extensions:

```sh
hitch-client install --only codex --yes --json
hitch-client install --only hermes --yes --json
hitch-client install --only hermes --url http://127.0.0.1:8797 --yes --json
hitch-client install --only pi --yes --json
hitch-client install --only pi --url http://127.0.0.1:8797 --yes --json
hitch-client install --only omp --yes --json
hitch-client install --only omp --url http://127.0.0.1:8797 --yes --json
hitch-client install --only opencode --yes --json
hitch-client install --only opencode --url http://127.0.0.1:8797 --yes --json
```

Installer behavior verified by tests:

- `hitch config init` creates missing user config at `~/.config/hitch/config.toml` from the embedded default config.
- Existing valid user config keeps user-owned values and receives missing managed default sections, such as the OpenCode event map required by the managed plugin.
- `hitch-client install --dry-run` does not mutate hook configuration.
- Codex hook installation covers `SessionStart`, `SubagentStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `PreCompact`, `PostCompact`, `SubagentStop`, and `Stop`, and is idempotent.
- Hermes hook installation covers `pre_tool_call`, `post_tool_call`, `pre_llm_call`, `post_llm_call`, `on_session_start`, `on_session_end`, `subagent_stop`, `transform_tool_result`, `transform_terminal_output`, `transform_llm_output`, and `pre_gateway_dispatch`, and is idempotent.
- Pi extension installation covers Pi's documented extension event callbacks and is idempotent.
- OMP extension installation covers OMP's current extension lifecycle, tool, session, retry, and user-command events and is idempotent.
- OpenCode plugin installation covers OpenCode typed plugin hooks and SDK lifecycle events and is idempotent.
- Installed clients know about the full source-event catalog where the harness exposes it, but the seeded server config only persists the recommended low-noise subset. Add opt-in `[harness.<name>.event_map]` rows from `docs/events.md` to capture excluded source events.
- Generated Codex and Hermes hook commands omit `-url` by default, resolve the Hitch API URL at runtime, and use `hitch-client` when it is available.
- Generated Pi and OMP extensions route observer callbacks to `/v1/events`, route return-capable control callbacks to `/v1/dispatch-sync`, resolve the Hitch API URL at runtime unless `--url` pins one, promote adapter metadata into Hitch envelope fields when available, and fail open if Hitch is unavailable. The generated OpenCode plugin also resolves the API URL at runtime and fails open.
- Existing Codex, Hermes, Pi, OMP, and OpenCode hook configuration is backed up before Hitch modifies it.
- Unknown harness names are rejected.
- Uninstall removes only Hitch-managed Codex, Hermes, Pi, OMP, or OpenCode hooks and leaves user config in place.

Status and doctor:

```sh
hitch status --json
hitch doctor --json
```

Uninstall selected hooks:

```sh
hitch-client uninstall --only codex --yes --json
hitch-client uninstall --only hermes --yes --json
```

The seeded config includes a sync `noop_observer` handler:

```toml
[handlers.noop_observer]
command = ["hitch", "handler", "noop-observer"]
events = ["*"]
mode = "sync"
timeout_ms = 1000
on_error = "fail_open"
on_timeout = "fail_open"
```

Shell hook shim entrypoints can be used directly by harness hook configuration once installed manually. Use `hitch-client` for hook dispatch:

```sh
hitch-client -harness codex -event SessionStart -sync
hitch-client -harness codex -event PreToolUse -sync
hitch-client -harness codex -event Stop -sync
hitch-client -harness hermes -event pre_tool_call -sync
```

# Installation

Install from latest source:

```sh
curl -fsSL https://raw.githubusercontent.com/sagebynature/hitch/main/install.sh | sh
```

The source installer checks for `git` and `go`, builds `./cmd/hitch` and `./cmd/hitch-client`, installs both binaries to `$HITCH_INSTALL_DIR` or `~/.local/bin`, verifies `hitch --version` and `hitch-client --version`, and then offers to run hook setup.

The current `hitch install` command seeds `~/.config/hitch/config.toml` when it is missing, detects Codex, Hermes, Pi, and OMP binaries on `PATH`, installs Hitch command hooks for every supported Codex lifecycle event into `~/.codex/hooks.json`, and installs Hitch shell hooks for supported Hermes events into `~/.hermes/config.yaml`. Managed hook commands prefer `hitch-client` and fall back to `hitch adapter` when the client binary is not installed beside `hitch`. Pi and OMP are detected and reported, but real hook patching is not implemented for them yet.

Dry run detected supported harnesses:

```sh
hitch install --dry-run --json
```

Dry run all harnesses, including unsupported skip reasons:

```sh
hitch install --all --dry-run --json
```

Install selected hooks. Use `--url` when Hitch is running on a non-default port or config:

```sh
hitch install --only codex --yes --json
hitch install --only hermes --yes --json
hitch install --only hermes --url http://127.0.0.1:8797 --yes --json
```

Installer behavior verified by tests:

- `--dry-run` does not mutate the filesystem.
- Missing user config is created at `~/.config/hitch/config.toml` from the embedded default config.
- Existing user config is left unchanged.
- Codex hook installation covers `SessionStart`, `SubagentStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `PreCompact`, `PostCompact`, `SubagentStop`, and `Stop`, and is idempotent.
- Hermes hook installation covers `pre_tool_call`, `post_tool_call`, `pre_llm_call`, `post_llm_call`, `on_session_start`, `on_session_end`, `subagent_stop`, `transform_tool_result`, `transform_terminal_output`, `transform_llm_output`, and `pre_gateway_dispatch`, and is idempotent.
- Generated Codex and Hermes hook commands embed the resolved Hitch API URL and use `hitch-client` when it is available.
- Existing Codex and Hermes hook configuration is backed up before Hitch modifies it.
- Unsupported available harnesses are reported as skipped.
- Unknown harness names are rejected.
- Uninstall removes only Hitch-managed Codex or Hermes hooks and leaves user config in place.

Status and doctor:

```sh
hitch status --json
hitch doctor --json
```

Uninstall selected hooks:

```sh
hitch uninstall --only codex --yes --json
hitch uninstall --only hermes --yes --json
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

Shell hook shim entrypoints can be used directly by harness hook configuration once installed manually. `hitch-client` is the preferred shim; `hitch adapter` remains a compatibility alias for existing hooks:

```sh
hitch-client -harness codex -event SessionStart -sync
hitch-client -harness codex -event PreToolUse -sync
hitch-client -harness codex -event Stop -sync
hitch-client -harness hermes -event pre_tool_call -sync
hitch adapter -harness hermes -event pre_tool_call -sync
```

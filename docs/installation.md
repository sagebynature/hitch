# Installation

Install from latest source:

```sh
curl -fsSL https://raw.githubusercontent.com/sagebynature/hitch/main/install.sh | sh
```

The source installer checks for `git` and `go`, builds `./cmd/hitch`, installs the binary to `$HITCH_INSTALL_DIR` or `~/.local/bin`, verifies `hitch --version`, and then offers to run hook setup.

The current `hitch install` command seeds `~/.config/hitch/config.toml` when it is missing, detects Codex, Hermes, Pi, and OMP binaries on `PATH`, installs Hitch command hooks for every supported Codex lifecycle event into `~/.codex/hooks.json`, and installs Hitch shell hooks for supported Hermes events into `~/.hermes/config.yaml`. Pi and OMP are detected and reported, but real hook patching is not implemented for them yet.

Dry run detected supported harnesses:

```sh
hitch install --dry-run --json
```

Dry run all harnesses, including unsupported skip reasons:

```sh
hitch install --all --dry-run --json
```

Install selected hooks:

```sh
hitch install --only codex --yes --json
hitch install --only hermes --yes --json
```

Installer behavior verified by tests:

- `--dry-run` does not mutate the filesystem.
- Missing user config is created at `~/.config/hitch/config.toml` from the embedded default config.
- Existing user config is left unchanged.
- Codex hook installation covers `SessionStart`, `SubagentStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `PreCompact`, `PostCompact`, `SubagentStop`, and `Stop`, and is idempotent.
- Hermes hook installation covers `pre_tool_call`, `post_tool_call`, `pre_llm_call`, `post_llm_call`, `on_session_start`, `on_session_end`, `subagent_stop`, `transform_tool_result`, `transform_terminal_output`, `transform_llm_output`, and `pre_gateway_dispatch`, and is idempotent.
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

Shell adapter entrypoints can be used directly by harness hook configuration once installed manually:

```sh
hitch adapter -harness codex -event SessionStart -sync
hitch adapter -harness codex -event PreToolUse -sync
hitch adapter -harness codex -event Stop -sync
hitch adapter -harness hermes -event pre_tool_call -sync
```

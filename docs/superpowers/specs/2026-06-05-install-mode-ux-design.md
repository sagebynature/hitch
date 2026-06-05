# Install Mode UX Design

Date: 2026-06-05

## Goal

Enhance `scripts/install.sh` with an interactive install-mode choice while preserving today's default installer behavior.

## User-facing behavior

The installer supports three modes:

- `all` — default. Install the server CLI and hook client, initialize server config, configure the server URL, and run hook setup.
- `server` — install only the `hitch` server CLI and initialize server config. Do not install `hitch-client`, configure hook URL, or run hook setup.
- `client` — install only `hitch-client`, configure the server URL, and run hook setup. Do not install the `hitch` server CLI or initialize server config.

When `/dev/tty` is available, `install.sh` asks:

```text
Install Hitch mode [all/server/client] (all):
```

An empty answer selects `all`. Invalid answers are rejected with a clear message and the prompt repeats.

When no TTY is available, the installer uses `HITCH_INSTALL_MODE` if set; otherwise it defaults to `all` for backward compatibility.

`HITCH_INSTALL_MODE` accepts exactly `all`, `server`, or `client`. Any other value fails early before cloning, building, or writing files.

## Mode matrix

| Mode | Build/install `hitch` | Build/install `hitch-client` | Run `hitch config init --json` | Prompt/configure `HITCH_URL` | Run `hitch-client install` |
|---|---:|---:|---:|---:|---:|
| `all` | yes | yes | yes | yes | yes |
| `server` | yes | no | yes | no | no |
| `client` | no | yes | no | yes | yes |

`HITCH_SKIP_HOOK_INSTALL=1` remains supported and disables hook setup in `all` and `client` modes.

## Implementation shape

`install.sh` remains POSIX `sh`.

Add:

- `HITCH_INSTALL_MODE=${HITCH_INSTALL_MODE:-}` near the existing environment defaults.
- `normalize_install_mode` to validate a mode string.
- `select_install_mode` to prompt on `/dev/tty` when interactive, otherwise default to `all`.
- small predicates or `case` branches in `main` to decide which binaries to build, copy, chmod, version-check, and configure.

The current source acquisition remains shared: if either binary must be built, the installer still resolves `HITCH_SOURCE_DIR` or clones `HITCH_REPO_URL` at `HITCH_REF` once.

Build/install steps should be mode-aware:

- `all`: current behavior, except gated by the selected mode.
- `server`: run only `go build -o "$tmp_dir/hitch" ./cmd/hitch`, copy only `hitch`, chmod/version-check only `hitch`, then run `hitch config init --json`.
- `client`: run only `go build -o "$tmp_dir/hitch-client" ./cmd/hitch-client`, copy only `hitch-client`, chmod/version-check only `hitch-client`, then configure URL and run hook setup unless skipped.

The existing `maybe_update_path` behavior should continue after any binary install. Its message can stay generic enough for both binaries, or be updated to avoid saying only `hitch` was installed when in client mode.

## Error handling

- Missing `go` still fails before build in every mode.
- Missing `git` fails only when `HITCH_SOURCE_DIR` is empty, as today.
- Invalid install mode fails before clone/build/copy.
- In `client` mode, hook setup requires the newly installed `hitch-client`; if no TTY is available, the installer prints the existing manual hook setup command.

## Testing

Add or update installer tests around `scripts/install.sh` if the current test harness supports shell installer execution. At minimum cover the script behavior through existing install tests or a new shell-level test that verifies:

- default mode installs both binaries and runs config init/hook path as before;
- `HITCH_INSTALL_MODE=server` skips `hitch-client` and hook setup;
- `HITCH_INSTALL_MODE=client` skips `hitch` and config init but runs URL/hook flow;
- invalid `HITCH_INSTALL_MODE` exits non-zero before build output appears.

Update `docs/installation.md` to document the prompt and non-interactive `HITCH_INSTALL_MODE` examples.

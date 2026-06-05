# Hitch Installer Design

## Goal

Create an installer similar in spirit to `https://pi.dev/install.sh`: build Hitch from the latest source, install the `hitch` binary into the user's PATH, create the Hitch user config, detect agent harnesses available on the machine, let the user choose which harnesses to wire up, and install Hitch hooks safely.

## Current implementation state

Implemented:

- Root-level `install.sh` builds from latest source, installs to `$HITCH_INSTALL_DIR` or `~/.local/bin`, verifies `hitch --version`, and prints PATH guidance.
- `hitch install` requires `--yes` unless `--dry-run` is used or an interactive TTY confirms the operation.
- `hitch install` plans a `seed_config` operation before harness hook operations.
- Missing `~/.config/hitch/config.toml` is created from the embedded default config.
- Existing `~/.config/hitch/config.toml` is left unchanged.
- Codex, Hermes, Pi, and OMP are detected from `PATH`.
- Codex hook installation writes a command hook to `~/.codex/hooks.json`.
- Existing Codex hook config is backed up before Hitch modifies it.
- `hitch uninstall --only codex` removes only the Hitch-managed Codex hook.
- Hermes, Pi, and OMP are reported as skipped until their real patchers are implemented.

Not implemented yet:

- Real Hermes config patching.
- Real Pi extension hook installation.
- Real OMP extension hook installation.
- Full interactive multi-select toggling; current interactive flow confirms the default supported selection.

## Recommended approach

Use a two-layer installer:

1. `install.sh` installs the Hitch binary from latest source.
2. `hitch install` seeds Hitch config, detects harnesses, and installs hooks.

This keeps curl-pipe-shell small and keeps harness-specific config mutation in Go, where it can be tested.

## Alternatives considered

### Shell script does everything

`install.sh` would build Hitch and directly edit harness configs.

Trade-off: fastest to write, but brittle. POSIX shell config patching is hard to test and unsafe as harness formats evolve.

### Shell script installs binary; Hitch installs hooks

`install.sh` handles prerequisites, checkout, build, binary placement, and PATH guidance. `hitch install` handles Hitch config seeding, harness detection, prompting, backups, dry-run, JSON output, and config patching.

Trade-off: slightly more code, but safer and maintainable. This is the recommended design.

### Package-manager first

Ship Homebrew/npm/Go install flows first and leave hook wiring to docs.

Trade-off: good later, but it does not satisfy the goal of building latest source and installing hooks in one flow.

## `install.sh` behavior

Add a root-level `install.sh` that supports:

```sh
curl -fsSL https://raw.githubusercontent.com/sagebynature/hitch/main/install.sh | sh
```

Behavior:

- Require `git` and `go`.
- Clone or update `github.com/sagebynature/hitch` from latest `main` into a temp/cache directory.
- Run:

```sh
go build -o hitch ./cmd/hitch
```

- Install to the first safe writable destination:
  1. `$HITCH_INSTALL_DIR`, if set.
  2. `~/.local/bin`.
  3. Fail with clear guidance if no destination is writable or creatable.
- Verify the installed binary with `hitch --version`, or with the absolute installed path if PATH is not updated yet.
- If the install directory is not first on PATH, offer to append the right PATH line to the user's shell profile.
- After binary installation, call the installed binary with `install` for hook setup when a TTY is available; otherwise print the exact hook setup command.
- For scripted usage, support flows such as:

```sh
HITCH_INSTALL_DIR="$HOME/.local/bin" sh install.sh
hitch install --only codex,hermes --yes
```

The shell script must not edit harness config files directly.

## `hitch install` behavior

Continue upgrading the installer from Codex support to full multi-harness support.

### Config seeding

This part is implemented and should remain part of every non-uninstall install plan:

- Create `~/.config/hitch/config.toml` from the embedded default config when missing.
- Leave an existing config file unchanged.
- Include config creation in `--dry-run --json` planned operations.
- Do not remove user config during `hitch uninstall`.

### Harness detection

Detection should produce structured records similar to:

```json
{
  "harness": "codex",
  "available": true,
  "reason": "codex found on PATH",
  "binary_path": "/opt/homebrew/bin/codex",
  "config_path": "~/.codex/config.toml",
  "installed": false,
  "supported": true
}
```

Initial detection rules:

- Codex: `command -v codex`, plus known config path.
- Hermes: `command -v hermes`, plus known config path.
- Pi: `command -v pi`, plus known config path or adapter extension path.
- OMP: `command -v omp`, plus known config path.

Codex patching is implemented through `~/.codex/hooks.json`. For Hermes, Pi, and OMP, keep marking config patching as unsupported until their config or extension contracts are verified.

### Selection

Interactive flow should show only available harnesses by default:

```text
Hitch found these harnesses:

  [x] Codex   found at /opt/homebrew/bin/codex
  [x] Pi      found at /Users/sage/.local/bin/pi
  [ ] Hermes  not found
  [ ] OMP     not found

Install Hitch hooks for selected harnesses? [Y/n]
```

Non-interactive behavior:

- `hitch install --all --yes`: install every detected and supported harness.
- `hitch install --only codex,pi --yes`: install selected harnesses if available and supported.
- `hitch install --dry-run --json`: print planned file edits only.
- No `--yes` and no TTY: fail safely.

The current CLI requires `--yes` for non-dry-run installs unless stdin is an interactive TTY that confirms the operation.

## Safety rules

- Never overwrite a user-owned config file without backing it up.
- Real harness config backups should go under:

```text
~/.config/hitch/backups/<harness>/<timestamp>-<filename>
```

- Current placeholder integration backups are flat files under:

```text
~/.config/hitch/backups/<harness>.txt.bak
```

- Patch idempotently with managed blocks where a text config supports them:

```text
# BEGIN HITCH MANAGED
...
# END HITCH MANAGED
```

- If a config format does not support managed text blocks, use format-aware patching and preserve unrelated fields.
- If an existing Hitch block differs, replace it only after backup.
- If an unknown conflicting hook is present, fail with exact manual instructions instead of clobbering it.
- Uninstall must remove only Hitch-managed hook content.

## Hook command

Installed hooks should use the absolute installed binary path, not `hitch` from PATH:

```sh
/Users/sage/.local/bin/hitch-client -harness codex -event PreToolUse -sync
```

This avoids PATH resolution bugs inside harness processes.

## Test plan

Go-side tests:

- config seeding operation appears before harness operations
- missing user config is created from the embedded default config
- existing user config is not overwritten
- uninstall leaves user config in place
- harness detection with fake PATH binaries
- dry-run output for detected harnesses
- idempotent hook install
- backup creation before hook config replacement
- conflict handling
- uninstall removes only Hitch-managed hook content
- non-interactive install requires `--yes`
- interactive install can proceed after TTY confirmation
- generated adapter command uses an absolute binary path

Shell script tests:

- fails when `git` is missing
- fails when `go` is missing
- honors `HITCH_INSTALL_DIR`
- installs binary into a temp dir
- verifies installed binary with `--version`
- prints PATH guidance when install dir is not on PATH
- invokes the installed `hitch install`

## Next implementation milestones

1. Add full interactive multi-select instead of yes/no confirmation for the default supported selection.
2. Implement Hermes config patching after verifying its `~/.hermes/config.yaml` shell hook schema.
3. Implement Pi extension installation after packaging the Pi adapter as a real extension.
4. Implement OMP extension installation after packaging the OMP adapter as a real extension.
5. Add timestamped real-config backups and conflict detection for every real patcher.
6. Add uninstall support for every Hitch-managed hook or extension.
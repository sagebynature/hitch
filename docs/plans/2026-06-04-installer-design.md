# Hitch Installer Design

## Goal

Create an installer similar in spirit to `https://pi.dev/install.sh`: build Hitch from the latest source, install the `hitch` binary into the user's PATH, detect agent harnesses available on the machine, let the user choose which harnesses to wire up, and install Hitch hooks safely.

## Recommended approach

Use a two-layer installer:

1. `install.sh` installs the Hitch binary.
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

The root-level `install.sh` should support:

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
- After binary installation, call `hitch install` for interactive hook selection.
- For scripted usage, support flows such as:

```sh
HITCH_INSTALL_DIR="$HOME/.local/bin" sh install.sh
hitch install --only codex,hermes --yes
```

## `hitch install` behavior

Upgrade the existing placeholder installer into a real harness installer.

Before harness detection, `hitch install` should ensure user config exists:

- Create `~/.config/hitch/config.toml` from the checked-in default config when missing.
- Leave an existing config file unchanged.
- Include config creation in `--dry-run --json` planned operations.
- Do not remove user config during `hitch uninstall`.

Detection should produce structured records similar to:

```json
{
  "harness": "codex",
  "available": true,
  "reason": "codex found on PATH",
  "config_path": "~/.codex/config.toml",
  "installed": false
}
```

Initial detection rules:

- Codex: `command -v codex`, plus known config path.
- Hermes: `command -v hermes`, plus known config path.
- Pi: `command -v pi`, plus known config path or adapter extension path.
- OMP: `command -v omp`, plus known config path.

If config locations are not yet certain, implement detection first and make unsupported patch targets explicit instead of guessing.

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

- `hitch install --all --yes`: install every detected supported harness.
- `hitch install --only codex,pi --yes`: install selected harnesses if available.
- `hitch install --dry-run --json`: print planned file edits only.
- No `--yes` and no TTY: fail safely.

## Safety rules

- Never overwrite a config file without backing it up.
- Backups go under:

```text
~/.config/hitch/backups/<harness>/<timestamp>-<filename>
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

## Hook command

Installed hooks should use the absolute installed binary path, not `hitch` from PATH:

```sh
/Users/sage/.local/bin/hitch adapter -harness codex -event PreToolUse -sync
```

This avoids PATH resolution bugs inside harness processes.

## Tests

Focus tests on Go installer logic, not curl-pipe-shell behavior.

Go-side tests:

- harness detection with fake PATH binaries
- dry-run output
- idempotent install
- backup creation
- conflict handling
- uninstall removes only Hitch-managed blocks
- non-interactive install requires `--yes`
- generated adapter command uses an absolute binary path

Shell script tests:

- fails when `go` is missing
- honors `HITCH_INSTALL_DIR`
- installs binary into a temp dir
- prints PATH guidance when install dir is not on PATH

## Open decision

Whether interactive confirmation may edit configs by default. Recommended policy:

- Interactive confirmation may edit configs.
- Non-interactive mode requires `--yes`.

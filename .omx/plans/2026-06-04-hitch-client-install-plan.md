# Hitch Client Install Migration Plan

## Requirements Summary

Migrate harness hook installation ownership from `hitch` to `hitch-client`.

Current evidence:

- `cmd/hitch/main.go:40-45` dispatches `hitch install` and `hitch uninstall` to installer code.
- `cmd/hitch/main.go:66` advertises `hitch install` in daemon CLI usage.
- `cmd/hitch/install.go:84-121` implements the installer command surface.
- `cmd/hitch-client/main.go:21-35` currently only parses hook dispatch flags and calls `clientshim.Run`.
- Docs and scripts still refer to `hitch install`: `README.md:130`, `install.sh:114`, `Makefile:49`, `docs/installation.md:16-30`, `docs/walkthrough.md:165-171`, `docs/handler-development.md:278`, `docs/docs/latest/index.html:78-84`, `docs/index.html:59,131-133`.

Approved design: `docs/plans/2026-06-04-hitch-client-install-design.md`.

## Acceptance Criteria

1. `hitch-client install` accepts the current installer flags: `--only`, `--all`, `--dry-run`, `--yes`, `--json`, and `--url`.
2. `hitch-client uninstall` accepts the same selection/output flags and removes managed harness hooks.
3. `hitch install` and `hitch uninstall` are no longer daemon subcommands and return usage/non-zero instead of mutating files.
4. New managed Codex and Hermes hook commands use the running `hitch-client` path, not `hitch adapter`.
5. `hitch-client uninstall` removes both new `hitch-client ...` managed hooks and legacy `hitch adapter ...` managed hooks.
6. `hitch-client` hook dispatch mode still works with no subcommand and preserves current stdout behavior.
7. Documentation, installer shell guidance, Makefile dry-run target, and latest HTML docs use `hitch-client install` / `hitch-client uninstall`.
8. Verification commands pass:
   - focused Go tests for installer/client behavior
   - `go test ./...`
   - `bun test adapters/**/*.test.ts`
   - example test drives
   - binary install dry-runs proving hook commands contain `bin/hitch-client`

## Implementation Steps

### 1. Extract installer package

Files:

- `cmd/hitch/install.go`
- new `internal/install/install.go`

Move installer code from `cmd/hitch/install.go` into `internal/install` with an exported entrypoint:

```go
func Run(args []string, uninstall bool) error
```

Keep the existing installer semantics. Replace `fatal`/`writeCLI` dependencies with package-local error returns and writer-aware output if needed.

Acceptance:

- `cmd/hitch` no longer needs installer implementation code.
- Installer tests can call `internal/install` or `hitch-client` command wrappers.

### 2. Add `hitch-client install` and `uninstall`

File:

- `cmd/hitch-client/main.go`

Change `run` to dispatch subcommands before flag parsing:

- `install` -> `install.Run(args, false)`
- `uninstall` -> `install.Run(args, true)`
- `--version` remains supported
- no subcommand keeps existing hook-dispatch flag behavior

Acceptance:

- `go run ./cmd/hitch-client install --dry-run --json` emits installer JSON.
- `go run ./cmd/hitch-client uninstall --only codex --dry-run --json` emits uninstall JSON.

### 3. Remove daemon install commands

File:

- `cmd/hitch/main.go`

Delete `case "install"` and `case "uninstall"` from the main command switch. Remove install from usage.

Acceptance:

- `go run ./cmd/hitch install --dry-run --json` exits non-zero and prints usage.
- No installer package import exists in `cmd/hitch`.

### 4. Retarget installer binary resolution

File:

- `internal/install/install.go`

Use the running executable as the `hitch-client` path for generated hook commands. Remove the normal install path that prefers sibling `hitch-client` but falls back to `hitch adapter` for newly generated hooks.

Keep legacy uninstall matching:

- match `hitch-client ... -harness <h> -event <e>`
- match `hitch adapter ... -harness <h> -event <e>`

Acceptance:

- `plannedOps(... install ...)` reasons contain `hitch-client` command base.
- no newly generated hook command contains ` adapter `.

### 5. Move and update tests

Files:

- `cmd/hitch/main_test.go`
- `cmd/hitch-client/main_test.go`
- new or updated `internal/install/*_test.go`

Move installer-specific tests from `cmd/hitch` to the installer package or retarget them through `cmd/hitch-client`. Keep daemon tests only for daemon CLI behavior.

Add tests for:

- `hitch-client install` dry-run planning.
- `hitch-client uninstall` legacy hook cleanup.
- `hitch install` rejected as unknown command.
- default hook dispatch still succeeds.

### 6. Update scripts and docs

Files:

- `Makefile`
- `install.sh`
- `README.md`
- `docs/installation.md`
- `docs/handler-development.md`
- `docs/walkthrough.md`
- `docs/docs/latest/index.html`
- `docs/index.html`
- any plan/doc references that describe current command behavior, excluding historical design docs unless they need a migration note.

Replace operational instructions with `hitch-client install` / `hitch-client uninstall`. Keep narrow compatibility notes for `hitch adapter` as a hook shim only.

### 7. Verify and clean up

Run:

```sh
go test ./cmd/hitch ./cmd/hitch-client ./internal/install ./internal/clientshim -run 'Test.*Install|Test.*Uninstall|TestRun|TestDefaultURL|TestPlannedOps|TestUnknown'
go test ./...
bun test adapters/**/*.test.ts
python3 examples/test_drive.py
HITCH_PAYLOAD_LOGGER_PORT=8796 python3 examples/test_payload_logger.py
make build
./bin/hitch-client --version
./bin/hitch-client install --only codex --url http://127.0.0.1:8797 --dry-run --json
./bin/hitch-client install --only hermes --url http://127.0.0.1:8797 --dry-run --json
```

Also verify `./bin/hitch install --dry-run --json` fails non-zero.

## Risks and Mitigations

- Risk: moving tests loses coverage. Mitigation: preserve existing installer tests and add command-level tests for both binaries.
- Risk: package extraction accidentally changes JSON output. Mitigation: reuse existing response shape and assert dry-run JSON fields in tests.
- Risk: old hooks become hard to remove. Mitigation: keep legacy uninstall matching for `hitch adapter` managed commands.
- Risk: main worktree contains unrelated user edits. Mitigation: implement only in `.worktrees/hitch-client-install` and merge when clean.

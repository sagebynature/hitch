# Hitch Client Install Migration Design

## Status

Approved on 2026-06-04.

## Problem

Harness hook installation currently lives behind `hitch-client install` and `hitch-client uninstall`. That puts agent-harness config mutation on the daemon CLI even though the hook execution boundary now belongs to `hitch-client`.

The ownership boundary should be sharper:

- `hitch` runs and inspects the local Hitch control plane.
- `hitch-client` owns agent-harness integration: hook installation, hook uninstallation, native stdin dispatch, and native stdout responses.

## Decision

Make a clean cutover:

- Remove `install` and `uninstall` subcommands from `hitch`.
- Add `install` and `uninstall` subcommands to `hitch-client`.
- Do not keep a `hitch-client install` compatibility alias.
- Keep legacy uninstall matching for old managed hooks that invoke `hitch adapter`, so existing installations can be removed by `hitch-client uninstall`.

## Command Surface

`hitch-client install` and `hitch-client uninstall` keep the existing installer flags and output contract:

```sh
hitch-client install --dry-run --json
hitch-client install --all --dry-run --json
hitch-client install --only codex --yes --json
hitch-client install --only hermes --url http://127.0.0.1:8797 --yes --json
hitch-client uninstall --only codex --yes --json
hitch-client uninstall --only hermes --yes --json
```

The native hook shim behavior remains the default non-subcommand mode:

```sh
hitch-client -harness codex -event PreToolUse -sync
```

`hitch` keeps daemon and inspection commands only:

```sh
hitch serve
hitch adapter
hitch handler noop-observer
hitch status
hitch doctor
hitch inspect-event
hitch replay
```

`hitch adapter` remains only as a compatibility hook shim for old managed hook commands and manual debugging. It is not the installer entrypoint.

## Architecture

Move installer orchestration out of `cmd/hitch` into a reusable internal package, expected shape:

```text
internal/install
  Run(args []string, uninstall bool) error
  PlannedOps(...)
  ApplyOps(...)
  DetectHarnesses(...)
```

`cmd/hitch-client` calls this package for `install` and `uninstall` subcommands. `cmd/hitch` no longer imports or calls installer code.

The installer package should resolve the running `hitch-client` executable and use it as the managed hook command target. Installed Codex and Hermes hooks should have this shape:

```sh
'/path/to/hitch-client' -url 'http://127.0.0.1:8797' -harness codex -event PreToolUse -sync
```

It should not generate new hooks that invoke `hitch adapter`.

## Migration Behavior

Uninstall remains migration-aware:

- Remove managed hooks that invoke `hitch-client ... -harness <name> -event <event>`.
- Remove legacy managed hooks that invoke `hitch adapter ... -harness <name> -event <event>`.
- Preserve unrelated user hooks.

This makes the clean cutover safe for users who installed hooks before the command migration.

## Error Handling

Keep current installer safety rules:

- Mutating installs/uninstalls require `--yes` unless stdin is an interactive TTY and the user confirms.
- `--dry-run` never mutates files.
- Unknown harness names fail.
- Unsupported detected harnesses are reported as skipped.
- Existing config files are backed up before mutation.
- Missing Hitch config is seeded from the embedded default config.

## Documentation Updates

Update references from `hitch-client install` to `hitch-client install` in:

- README
- `install.sh` post-install guidance
- installation docs
- handler development docs
- walkthrough
- generated/latest docs HTML
- Makefile dry-run target

Where relevant, keep explicit notes that `hitch adapter` remains a compatibility/debug shim, not the installer entrypoint.

## Tests

Required automated coverage:

- `hitch-client install --dry-run --json` plans Codex and Hermes hook operations.
- Planned managed hook commands use `hitch-client`, not `hitch adapter`.
- `hitch-client uninstall` removes both new and legacy managed hooks.
- `hitch-client install` and `hitch-client uninstall` are no longer accepted daemon subcommands.
- Existing stdin hook dispatch tests for `hitch-client` still pass.
- Full `go test ./...`, adapter tests, example drives, and binary dry runs pass.

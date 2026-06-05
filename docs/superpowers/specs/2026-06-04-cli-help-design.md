# CLI Help Design

## Status

Approved on 2026-06-04.

## Problem

`hitch --help` currently exposes only the root Go `flag` usage and hides the real subcommands. `hitch-client --help` explains hook dispatch flags but does not mention `install` or `uninstall`. Subcommand help is sparse and inconsistent. `hitch adapter` is being removed because `hitch-client` is now the dedicated hook dispatch CLI.

The CLI should be useful to a human trying to install, run, inspect, or debug Hitch without reading source code.

## Decision

Implement human-first help while keeping the current `flag` package and command structure.

Use a small internal usage renderer instead of introducing a CLI framework. Each binary and subcommand should have explicit usage text with:

- one-line purpose
- usage syntax
- commands or flags
- examples
- relevant notes

Support both conventional help forms:

- `hitch --help`
- `hitch help <command>`
- `hitch <command> --help`
- `hitch-client --help`
- `hitch-client help install`
- `hitch-client install --help`

## Target UX

### `hitch --help`

Should explain the product and command surface:

```text
Hitch routes source harness events to local policy/observer handlers.

Usage:
  hitch <command> [options]

Commands:
  serve           Run the local Hitch API server
  handler         Run a bundled handler
  status          Print runtime/config status
  doctor          Run basic diagnostics
  inspect-event   Inspect an audited event
  replay          Replay handlers for an audited event

Global:
  hitch --version
  hitch --help

Examples:
  hitch serve --config config/default.config.toml
  hitch status --json
  hitch inspect-event norm_...
```

### `hitch <command> --help`

Every subcommand should show at least one example and its flags. For `handler`, the help should list bundled handlers such as `noop-observer`.

### `hitch-client --help`

Should explain both hook dispatch and installation modes:

```text
hitch-client dispatches source hook payloads to Hitch.

Usage:
  hitch-client [options] < payload.json
  hitch-client install [options]
  hitch-client uninstall [options]

Hook flags:
  -harness string   source harness: codex, hermes, pi, omp
  -event string     source event type, e.g. PreToolUse
  -sync             wait for native response on stdout
  -url string       Hitch API URL

Examples:
  hitch-client -harness codex -event PreToolUse -sync < payload.json
  hitch-client install codex
```

## Implementation Notes

- Keep command parsing boring and local to `cmd/hitch` and `cmd/hitch-client`.
- Avoid new dependencies.
- Prefer explicit string constants/functions for help text over dynamically introspecting flags; this keeps examples and notes readable.
- Set each `flag.FlagSet` output to the caller-provided writer where tests need to capture help.
- Use `flag.ContinueOnError` in testable command paths where practical so help can return without `os.Exit`.
- Preserve existing command behavior for non-help paths.
- Remove `hitch adapter` from the daemon CLI and route users to `hitch-client` for dispatch.

## Acceptance Criteria

- `hitch --help` prints command summaries and examples.
- `hitch help serve` and `hitch serve --help` print serve-specific help.
- `hitch help handler` lists bundled handlers.
- `hitch --help` does not list `adapter`.
- `hitch-client --help` mentions dispatch, install, and uninstall modes.
- `hitch-client help install` and `hitch-client install --help` print installer help.
- Help output exits successfully and does not require config files, servers, stdin payloads, or network access.
- Existing command behavior remains unchanged outside help paths.
- Tests assert representative help content for both binaries.

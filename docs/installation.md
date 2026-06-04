# Installation

The current installer manages Hitch integration placeholder files under `~/.config/hitch/integrations`. It does not yet patch real Codex, Hermes, Pi, or OMP user configuration files.

Dry run all harnesses:

```sh
hitch install --all --dry-run --json
```

Install selected managed integration files:

```sh
hitch install --only codex,hermes --yes --json
```

Installer behavior verified by tests:

- `--dry-run` does not mutate the filesystem.
- Unknown harness names are rejected.
- Re-running install for identical managed content is idempotent.
- Existing different integration-file content is backed up under `~/.config/hitch/backups` before overwrite.
- Uninstall removes the selected managed integration file.

Status and doctor:

```sh
hitch status --json
hitch doctor --json
```

Uninstall selected managed files:

```sh
hitch uninstall --only codex --yes --json
```

Shell adapter entrypoints can be used directly by harness hook configuration once installed manually:

```sh
hitch adapter -harness codex -event PreToolUse -sync
hitch adapter -harness hermes -event pre_tool_call -sync
```

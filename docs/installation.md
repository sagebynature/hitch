# Installation

Current implementation provides managed integration placeholders and adapter entrypoints.

Dry run:

```sh
hitch install --only codex,hermes,pi,omp --dry-run --json
```

Install managed integration files:

```sh
hitch install --only codex --yes
```

Status and doctor:

```sh
hitch status --json
hitch doctor --json
```

Uninstall managed files:

```sh
hitch uninstall --only codex --yes
```

Installer behavior is intentionally idempotent and scoped to Hitch-managed files under `~/.config/hitch/integrations` in this first implementation.

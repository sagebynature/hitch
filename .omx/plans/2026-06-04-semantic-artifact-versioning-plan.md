# Semantic Artifact Versioning Plan

## Requirements Summary

Hitch needs SemVer-managed build artifacts without conflating product release version, protocol compatibility, store schema compatibility, and exact rebuild identity.

Current evidence:

- `cmd/hitch/main.go:31-69` has a CLI-only `version = "0.1.0"` constant used by `hitch --version`.
- `internal/protocol/protocol.go:10` has a separate `protocol.Version = "0.1.0"` used by envelopes and event mapping.
- `internal/api/client.go:51-52` stamps API client requests with `SourceAdapterVersion: protocol.Version`.
- `internal/api/server.go:120-123` stores `source_adapter_version` on inbound events and stores normalized events with `MappingVersion: protocol.Version`.
- `internal/store/store.go:13-18` defines `schemaVersion = 1` and a `schema_meta(version)` table.
- `internal/store/store.go:152-176` writes `schema_version` into every persisted journal row.
- `Makefile:22-24` and `install.sh:93-100` build with plain `go build`, so artifact version metadata is not currently injected from release state.

## Design Principles

1. Product SemVer identifies the released Hitch artifact, not every internal contract.
2. Protocol, adapter contract, and store schema versions remain separate compatibility axes.
3. Exact incremental rebuild decisions use artifact manifests and hashes, not SemVer alone.
4. The default developer build remains boring: `make build` continues to produce `bin/hitch`.
5. Version information is observable from the CLI, API status/health paths where appropriate, and artifact manifests.

## Recommended Architecture

### Version domains

Introduce explicit version domains:

- Product version: SemVer for the `hitch` binary and release artifacts, injected at build time.
- Commit/date/dirty metadata: build provenance, injected at build time.
- Protocol version: compatibility version for normalized event envelopes and handler input.
- Adapter contract version: compatibility version for request/response semantics shared with native adapters.
- Store schema version: integer schema version for SQLite/journal rows.
- Artifact digest: SHA256 over each emitted binary/archive for exact identity.

### Package layout

Add a small build metadata package:

```text
internal/buildinfo
  info.go
```

Target shape:

```go
package buildinfo

var Version = "0.0.0-dev"
var Commit = "unknown"
var Date = "unknown"
var Dirty = "unknown"

func Info() Info
```

Keep compatibility versions out of `buildinfo`:

```text
internal/protocol.Version              // protocol compatibility, existing
internal/protocol.AdapterContractVersion
internal/store.SchemaVersion or Version()
```

This removes the duplicate CLI product constant from `cmd/hitch/main.go` and leaves `internal/protocol.Version` focused on event compatibility.

## Implementation Steps

### 1. Add build metadata package

Files:

- Add `internal/buildinfo/info.go`.
- Update `cmd/hitch/main.go`.

Changes:

- Define injected variables: `Version`, `Commit`, `Date`, `Dirty`.
- Define a stable struct for JSON output:

```go
type Info struct {
  Version string `json:"version"`
  Commit  string `json:"commit"`
  Date    string `json:"date"`
  Dirty   string `json:"dirty"`
}
```

- Replace `cmd/hitch/main.go:31` CLI `version` constant with `buildinfo.Version`.
- Keep text output compatible enough for humans: `hitch 0.2.0`.
- Add `hitch --version --json` or `hitch version --json`; prefer `hitch version` only if the CLI parser is cleaned up at the same time. The lower-risk first step is `--version --json`.

Acceptance criteria:

- `go run ./cmd/hitch --version` prints `hitch 0.0.0-dev` in an untagged dev build.
- `go run ./cmd/hitch --version --json` prints JSON with product build metadata.
- Existing status output uses the same product version source.

### 2. Separate protocol and adapter contract versions

Files:

- Update `internal/protocol/protocol.go`.
- Update `internal/api/client.go`.
- Update `internal/api/server.go` only if request/response JSON needs a new field.
- Update `schemas/hitch-event-envelope.schema.json` if contract fields are exposed in envelopes.

Changes:

- Keep `protocol.Version` as event envelope/mapping compatibility.
- Add `protocol.AdapterContractVersion = "0.1.0"`.
- Update `internal/api/client.go:51-52` so `SourceAdapterVersion` uses the adapter contract version when the client represents the local adapter contract, not the event protocol version.
- Do not rename stored column `source_adapter_version`; clarify its value in code comments/tests.

Acceptance criteria:

- Stored inbound events record adapter contract version.
- Stored normalized events record protocol/mapping version.
- Handler input `hitch_version` remains protocol version unless a later ADR changes envelope semantics.

### 3. Expose schema version safely

Files:

- Update `internal/store/store.go`.
- Update `internal/store/store_test.go`.

Changes:

- Export or expose the schema version without exposing mutable state, e.g.:

```go
const SchemaVersion = 1
```

or:

```go
func SchemaVersion() int { return schemaVersion }
```

Prefer `const SchemaVersion = 1` if external package references are acceptable; otherwise use a function.

- Add a store test that verifies `schema_meta.version` is seeded to the expected schema version.
- Leave real migration upgrade mechanics out of this plan unless a migration is actually needed.

Acceptance criteria:

- CLI JSON can report schema version.
- Store tests prove DB metadata and row metadata are still written.

### 4. Add build metadata injection to Makefile

Files:

- Update `Makefile`.

Changes:

- Add variables:

```make
VERSION ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo 0.0.0-dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
DIRTY   ?= $(shell test -z "$$(git status --porcelain 2>/dev/null)" && echo false || echo true)
LDFLAGS := -X github.com/sagebynature/hitch/internal/buildinfo.Version=$(VERSION) ...
```

- Use `go build -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/hitch`.
- Normalize product version from `git describe` so a tag `v0.2.0` reports `0.2.0` or decide to keep the leading `v`. Recommendation: CLI JSON uses `0.2.0`; Git tags use `v0.2.0`.

Acceptance criteria:

- `make build VERSION=0.2.0 COMMIT=test DATE=2026-06-04T00:00:00Z DIRTY=false` produces a binary whose version JSON reports those exact values.
- Default `make build` works outside a Git checkout and falls back to dev metadata.

### 5. Update installer build path

Files:

- Update `install.sh`.

Changes:

- Replace direct `go build` at `install.sh:93-94` with either:
  - `make build BINARY="$tmp_dir/hitch"`, reusing Makefile metadata logic; or
  - equivalent shell-computed `-ldflags` if avoiding a make dependency is more important.

Recommendation: use `make build` if `make` is already an accepted prerequisite; otherwise keep installer self-contained with shell ldflags. Current install docs mention `git` and `go`, not `make`, so the safer default is shell ldflags in `install.sh`.

Acceptance criteria:

- Source install still works with only `git` and `go` prerequisites.
- Installed `hitch --version --json` reports source ref provenance when available.

### 6. Generate artifact manifests

Files:

- Add `scripts/build_artifact_manifest.go` or keep it inside `cmd/hitch` as a subcommand.
- Update `Makefile`.
- Add `schemas/artifact-manifest.schema.json`.

Recommended manifest shape:

```json
{
  "artifact_schema_version": 1,
  "name": "hitch",
  "version": "0.2.0",
  "commit": "abc1234",
  "date": "2026-06-04T00:00:00Z",
  "dirty": false,
  "goos": "darwin",
  "goarch": "arm64",
  "protocol_version": "0.1.0",
  "adapter_contract_version": "0.1.0",
  "store_schema_version": 1,
  "sha256": "...",
  "files": [
    { "path": "hitch", "sha256": "...", "size": 12345678 }
  ]
}
```

Changes:

- Add `make artifact` that builds `build/hitch_<version>_<goos>_<goarch>/hitch` and writes adjacent manifest JSON.
- Avoid reading the whole binary into long-lived memory in Go if implementing in Go; stream into `sha256.New()`.
- Validate manifest schema in tests or with a small Go test if no JSON schema runner exists.

Acceptance criteria:

- `make artifact VERSION=0.2.0 ...` writes binary plus manifest.
- Manifest digest matches the built binary.
- Manifest includes every compatibility version needed for incremental decisions.

### 7. Define incremental compatibility policy

Files:

- Add or update docs in `docs/replay.md` or a new plan/ADR if this becomes architectural policy.
- Update tests once behavior exists.

Policy:

- If product SemVer differs but protocol/schema/adapter versions are unchanged, artifacts may be different but existing normalized events remain replay-compatible.
- If protocol version changes, normalized event replay should be treated as requiring compatibility review or remapping.
- If adapter contract version changes, native adapter artifacts must be rebuilt/updated together.
- If store schema version changes, DB migration logic must prove forward migration before writing new rows.
- If manifest hash differs, exact artifact cache is stale regardless of SemVer.

Acceptance criteria:

- The policy is documented in one place.
- Any future rebuild checker has an unambiguous decision table.

### 8. Tests and verification

Add or update tests:

- CLI version text output.
- CLI version JSON output.
- Status JSON uses `buildinfo.Version`.
- API request helper uses `AdapterContractVersion` for `source_adapter_version`.
- Ingest path persists `source_adapter_version` and `mapping_version` distinctly.
- Store schema metadata is seeded.
- Manifest generation produces matching SHA256 and required compatibility fields.

Verification commands:

```sh
go test ./...
make build VERSION=0.2.0 COMMIT=test DATE=2026-06-04T00:00:00Z DIRTY=false
./bin/hitch --version
./bin/hitch --version --json
make artifact VERSION=0.2.0 COMMIT=test DATE=2026-06-04T00:00:00Z DIRTY=false
```

## Risks and Mitigations

- Risk: Product version and protocol version are confused again.
  - Mitigation: package boundaries and tests must assert different fields in status/version/ingest paths.

- Risk: Git-dependent Makefile logic breaks source tarball builds.
  - Mitigation: every metadata field has an explicit override and a non-Git fallback.

- Risk: Installer gains an implicit dependency on `make`.
  - Mitigation: keep installer shell-based or explicitly update installation docs if using Makefile.

- Risk: SemVer is misused as an exact cache key.
  - Mitigation: manifests include SHA256; documentation states SemVer is compatibility, hash is identity.

- Risk: Dirty builds are accidentally treated as release artifacts.
  - Mitigation: release target should fail on `DIRTY=true`; dev `make build` should not fail.

## ADR Summary

Decision: Adopt Git tag SemVer plus ldflags-injected product build metadata, separate compatibility versions, and artifact manifests with hashes.

Drivers:

- Hitch is a Go local daemon where ldflags injection is the standard lightweight way to stamp binaries.
- Existing code already records protocol, adapter/source, mapping, and schema versions, but not cleanly separated.
- Incremental build correctness requires exact artifact identity, not only semantic compatibility.

Alternatives considered:

1. Keep hardcoded constants only.
   - Rejected: cannot identify release provenance or artifact staleness.
2. Use SemVer only, no manifests.
   - Rejected: SemVer cannot prove byte identity or input freshness.
3. Introduce a full release tool immediately.
   - Deferred: useful later, but current repo can get the core contract with Makefile/install changes first.

Consequences:

- Version output becomes more verbose and machine-readable.
- Release builds need explicit metadata discipline.
- Future replay/remapping can make compatibility decisions from recorded data.

Follow-ups:

- Add GoReleaser or equivalent once cross-platform packaging is needed.
- Add DB migration tests before increasing store schema version.
- Add adapter package SemVer if TypeScript adapters are published independently.

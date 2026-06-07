# Golang Repo Structure Improvements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Improve Hitch’s Go repository structure so CI actually verifies the code, package boundaries are clearer, harness registration is centralized, large files are easier to maintain, and declared configuration/tooling matches implemented behavior.

**Architecture:** Keep the current single Go module, `cmd/` entrypoints, and `internal/` package boundary. Make incremental, behavior-preserving changes: first fix verification, then extract small seams around harness registration and service orchestration, then split oversized files without changing public behavior. Avoid framework additions unless they directly enforce repository quality.

**Tech Stack:** Go 1.26.4, standard `net/http`, `database/sql`, `log/slog`, `modernc.org/sqlite`, `github.com/BurntSushi/toml`, GitHub Actions, Makefile.

---

## Current evidence

- `go.mod` declares module `github.com/sagebynature/hitch` and `go 1.26.4`.
- `Makefile` defines `test`, but not `test-go`; CI and release workflows call `make test-go`.
- `internal/api/server.go` imports every harness adapter and builds the adapter map directly.
- `internal/clientshim/clientshim.go` imports every harness adapter again for sync no-op translation.
- `internal/install/install.go` is a very large multi-responsibility file covering CLI flags, harness catalogs, detection, planning, mutation, generated TS/plugin content, YAML/JSON editing, backup, and shell quoting.
- `cmd/hitch/main.go` mixes CLI parsing, server wiring, store setup, signal handling, replay dispatch, and output formatting.
- `internal/config/config.go` validates `audit.backend = jsonl` and `log.otlp`, but observed server/logging wiring only implements SQLite persistence plus stdout/file log sinks.
- `go test ./...` passes and `go vet ./...` is clean before this plan.

---

## Target file structure

### Modify

- `Makefile` — add missing `test-go`, `vet`, and optionally `check` targets.
- `.github/workflows/ci.yml` — make CI call explicit verification targets.
- `.github/workflows/release.yml` — match CI verification before release packaging.
- `cmd/hitch/main.go` — thin the command entrypoint after internal command package exists.
- `cmd/hitch/main_test.go` — keep CLI behavior coverage; adjust calls only if command functions move.
- `cmd/hitch-client/main.go` — keep thin; only adjust imports if installer/client command APIs move.
- `internal/api/server.go` — depend on a harness registry instead of concrete harness adapter packages.
- `internal/api/server_test.go` — add/adjust tests for injected harness registry and server behavior.
- `internal/clientshim/clientshim.go` — use shared harness registry for `NativeNoop`.
- `internal/clientshim/clientshim_test.go` — add coverage that no-op translation still works via registry.
- `internal/config/config.go` — align validation with implemented backends, or introduce explicit backend helpers.
- `internal/config/config_test.go` — pin accepted/rejected config combinations.
- `internal/install/install.go` — shrink by moving related code into same-package files.
- `internal/install/install_test.go` — should remain passing after file split.
- `internal/logging/logging.go` — either implement OTLP sink or reject unsupported OTLP config for now.
- `internal/logging/logging_test.go` — cover selected logging decision.
- `README.md` and `docs/configuration.md` — align docs with actual supported backends/tooling.

### Create

- `internal/harness/registry.go` — shared harness registration API and default registry.
- `internal/app/app.go` — server application wiring that currently lives in `cmd/hitch serve`.
- `internal/app/replay.go` — replay orchestration currently embedded in `cmd/hitch replay`.
- `internal/app/app_test.go` — unit tests for service wiring decisions that do not require OS signals.
- `internal/install/catalog.go` — event lists and harness specs.
- `internal/install/planning.go` — `plannedOps`, harness selection, and operation descriptions.
- `internal/install/detect.go` — detection helpers.
- `internal/install/apply.go` — `applyOps`, backup, install/uninstall file mutation coordination.
- `internal/install/codex.go` — Codex hook JSON helpers.
- `internal/install/hermes.go` — Hermes YAML helpers.
- `internal/install/pi.go` — Pi/OMP extension helpers if shared code remains small.
- `internal/install/opencode.go` — OpenCode plugin helpers.
- `internal/install/render.go` — JSON/string literal helpers and generated TypeScript/plugin content renderers.
- `.golangci.yml` — optional after vet passes; minimal lint set only.

---

# Implementation tasks

### Task 1: Fix verification entrypoints

**Files:**
- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml`
- Modify: `.github/workflows/release.yml`

- [ ] **Step 1: Add explicit Makefile targets**

Change the first line and add targets after `test`:

```make
.PHONY: help build test test-go vet check run serve status doctor install-dry-run clean preview-pages
```

```make
test:
	go test $(GO_PACKAGES)

test-go: test

vet:
	go vet $(GO_PACKAGES)

check: vet test-go build
```

- [ ] **Step 2: Verify the missing-target bug is fixed**

Run:

```sh
make -n test-go
```

Expected output includes:

```text
go test ./...
```

- [ ] **Step 3: Make CI use the aggregate check**

In `.github/workflows/ci.yml`, replace the separate Go test/build steps:

```yaml
      - name: Run Go checks
        run: make check
```

Keep the install dry-run and installer shell syntax steps after `make check`.

- [ ] **Step 4: Make release use the same check**

In `.github/workflows/release.yml`, replace the separate Go test/build steps:

```yaml
      - name: Run Go checks
        run: make check
```

Keep release packaging after checks.

- [ ] **Step 5: Run verification**

Run:

```sh
make check
```

Expected: `go vet ./...`, `go test ./...`, and both CLI builds complete successfully.

- [ ] **Step 6: Commit**

```sh
git add Makefile .github/workflows/ci.yml .github/workflows/release.yml
git commit -m "ci: restore go verification target"
```

---

### Task 2: Centralize harness registration

**Files:**
- Create: `internal/harness/registry.go`
- Modify: `internal/api/server.go`
- Modify: `internal/clientshim/clientshim.go`
- Modify: `internal/api/server_test.go`
- Modify: `internal/clientshim/clientshim_test.go`

- [ ] **Step 1: Add registry tests first**

Create or add to `internal/harness/registry_test.go`:

```go
package harness

import (
	"testing"

	"github.com/sagebynature/hitch/internal/protocol"
)

func TestDefaultRegistryContainsSupportedHarnesses(t *testing.T) {
	registry := DefaultRegistry()
	for _, h := range []protocol.Harness{
		protocol.HarnessCodex,
		protocol.HarnessHermes,
		protocol.HarnessPi,
		protocol.HarnessOMP,
		protocol.HarnessOpenCode,
	} {
		if _, ok := registry.Lookup(h); !ok {
			t.Fatalf("missing harness %s", h)
		}
	}
}

func TestRegistryRejectsUnknownHarness(t *testing.T) {
	registry := DefaultRegistry()
	if _, ok := registry.Lookup(protocol.Harness("unknown")); ok {
		t.Fatal("unknown harness was registered")
	}
}
```

- [ ] **Step 2: Run the new test and verify it fails**

Run:

```sh
go test ./internal/harness -run 'TestDefaultRegistry|TestRegistryRejects' -count=1
```

Expected: FAIL because `DefaultRegistry` is undefined.

- [ ] **Step 3: Implement the registry**

Create `internal/harness/registry.go`:

```go
package harness

import (
	"github.com/sagebynature/hitch/internal/harness/codex"
	"github.com/sagebynature/hitch/internal/harness/hermes"
	"github.com/sagebynature/hitch/internal/harness/omp"
	"github.com/sagebynature/hitch/internal/harness/opencode"
	"github.com/sagebynature/hitch/internal/harness/pi"
	"github.com/sagebynature/hitch/internal/protocol"
)

type Registry struct {
	normalizers map[protocol.Harness]Normalizer
}

func NewRegistry(normalizers map[protocol.Harness]Normalizer) Registry {
	out := make(map[protocol.Harness]Normalizer, len(normalizers))
	for h, n := range normalizers {
		out[h] = n
	}
	return Registry{normalizers: out}
}

func DefaultRegistry() Registry {
	return NewRegistry(map[protocol.Harness]Normalizer{
		protocol.HarnessCodex:    codex.Mapper{},
		protocol.HarnessHermes:   hermes.Mapper{},
		protocol.HarnessPi:       pi.Mapper{},
		protocol.HarnessOMP:      omp.Mapper{},
		protocol.HarnessOpenCode: opencode.Mapper{},
	})
}

func (r Registry) Lookup(h protocol.Harness) (Normalizer, bool) {
	n, ok := r.normalizers[h]
	return n, ok
}
```

- [ ] **Step 4: Run registry tests**

Run:

```sh
go test ./internal/harness -run 'TestDefaultRegistry|TestRegistryRejects' -count=1
```

Expected: PASS.

- [ ] **Step 5: Update API server construction**

In `internal/api/server.go`, remove direct imports of harness adapter packages and change `Server` construction to use `harness.DefaultRegistry()`.

Replace `buildHarnessRuntimes` with:

```go
func buildHarnessRuntimes(cfg config.Config, registry harness.Registry) map[protocol.Harness]harnessRuntime {
	return map[protocol.Harness]harnessRuntime{
		protocol.HarnessCodex:    buildHarnessRuntime(registry, protocol.HarnessCodex, cfg.Harness.Codex.EventMap),
		protocol.HarnessHermes:   buildHarnessRuntime(registry, protocol.HarnessHermes, cfg.Harness.Hermes.EventMap),
		protocol.HarnessPi:       buildHarnessRuntime(registry, protocol.HarnessPi, cfg.Harness.Pi.EventMap),
		protocol.HarnessOMP:      buildHarnessRuntime(registry, protocol.HarnessOMP, cfg.Harness.OMP.EventMap),
		protocol.HarnessOpenCode: buildHarnessRuntime(registry, protocol.HarnessOpenCode, cfg.Harness.OpenCode.EventMap),
	}
}

func buildHarnessRuntime(registry harness.Registry, h protocol.Harness, eventMap map[string]config.EventTypes) harnessRuntime {
	normalizer, _ := registry.Lookup(h)
	return harnessRuntime{normalizer: normalizer, eventMap: eventMap}
}
```

Update `New`:

```go
func New(cfg config.Config, log *slog.Logger, st *store.Store) *Server {
	return NewWithHarnessRegistry(cfg, log, st, harness.DefaultRegistry())
}

func NewWithHarnessRegistry(cfg config.Config, log *slog.Logger, st *store.Store, registry harness.Registry) *Server {
	s := &Server{cfg: cfg, log: log, store: st, runner: dispatch.NewRunnerWithLogger(cfg.Handlers, log), harnesses: buildHarnessRuntimes(cfg, registry)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("POST /v1/events", s.handleEvent)
	mux.HandleFunc("GET /v1/events/", s.handleGetEvent)
	s.mux = mux
	return s
}
```

- [ ] **Step 6: Update client no-op translation**

In `internal/clientshim/clientshim.go`, remove direct adapter imports and rewrite `NativeNoop`:

```go
func NativeNoop(harnessName, sourceEventType string) protocol.RawJSON {
	normalizer, ok := harness.DefaultRegistry().Lookup(protocol.Harness(harnessName))
	if !ok {
		return nil
	}
	native, _ := normalizer.Translate(sourceEventType, protocol.AggregateDecision{Decision: protocol.Decision{Behavior: protocol.BehaviorNone}})
	return native
}
```

- [ ] **Step 7: Run affected tests**

Run:

```sh
go test ./internal/harness ./internal/api ./internal/clientshim -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```sh
git add internal/harness internal/api/server.go internal/api/server_test.go internal/clientshim/clientshim.go internal/clientshim/clientshim_test.go
git commit -m "refactor: centralize harness registration"
```

---

### Task 3: Move daemon orchestration out of `cmd/hitch`

**Files:**
- Create: `internal/app/app.go`
- Create: `internal/app/replay.go`
- Create: `internal/app/app_test.go`
- Modify: `cmd/hitch/main.go`
- Modify: `cmd/hitch/main_test.go`

- [ ] **Step 1: Write application wiring tests**

Create `internal/app/app_test.go`:

```go
package app

import (
	"context"
	"testing"
	"time"
)

func TestServerAddr(t *testing.T) {
	got := ServerAddr("127.0.0.1", 8799)
	if got != "127.0.0.1:8799" {
		t.Fatalf("got %q", got)
	}
}

func TestShutdownTimeoutIsFiveSeconds(t *testing.T) {
	ctx, cancel := ShutdownContext(context.Background())
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("shutdown context has no deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 4*time.Second || remaining > 5*time.Second {
		t.Fatalf("unexpected shutdown timeout %s", remaining)
	}
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```sh
go test ./internal/app -count=1
```

Expected: FAIL because package/functions are undefined.

- [ ] **Step 3: Create app helpers**

Create `internal/app/app.go`:

```go
package app

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/sagebynature/hitch/internal/api"
	"github.com/sagebynature/hitch/internal/config"
	"github.com/sagebynature/hitch/internal/logging"
	"github.com/sagebynature/hitch/internal/store"
)

func ServerAddr(host string, port int) string {
	return fmt.Sprintf("%s:%d", host, port)
}

func ShutdownContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 5*time.Second)
}

type ServeOptions struct {
	ConfigPath string
}

type ServerBundle struct {
	Server *http.Server
	Close  func() error
}

func NewServerBundle(ctx context.Context, opts ServeOptions) (*ServerBundle, error) {
	configPath := opts.ConfigPath
	if configPath == "" {
		configPath = config.DefaultPath
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	logger, logCloser, err := logging.New(cfg.Log)
	if err != nil {
		return nil, err
	}
	dbPath := config.ExpandHome(cfg.Audit.SQLite.Path)
	if err := ensureDir(filepath.Dir(dbPath)); err != nil {
		_ = logCloser.Close()
		return nil, err
	}
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		_ = logCloser.Close()
		return nil, err
	}
	srv := &http.Server{
		Addr:              ServerAddr(cfg.Server.Host, cfg.Server.Port),
		Handler:           api.New(cfg, logger, st).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return &ServerBundle{
		Server: srv,
		Close: func() error {
			storeErr := st.Close()
			logErr := logCloser.Close()
			if storeErr != nil {
				return storeErr
			}
			return logErr
		},
	}, nil
}
```

Create an OS wrapper in the same file or a separate small file:

```go
func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
```

Add `os` to imports when using `ensureDir`.

- [ ] **Step 4: Move replay orchestration**

Create `internal/app/replay.go`:

```go
package app

import (
	"context"
	"time"

	"github.com/sagebynature/hitch/internal/config"
	"github.com/sagebynature/hitch/internal/dispatch"
	"github.com/sagebynature/hitch/internal/harness"
	"github.com/sagebynature/hitch/internal/protocol"
	"github.com/sagebynature/hitch/internal/store"
)

type ReplayOptions struct {
	ConfigPath string
	EventID    string
	DryRun     bool
}

type ReplayResult struct {
	DryRun    bool                       `json:"dry_run"`
	Event     protocol.EventEnvelope     `json:"event"`
	Aggregate protocol.AggregateDecision `json:"aggregate,omitempty"`
}

func Replay(ctx context.Context, opts ReplayOptions) (ReplayResult, error) {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return ReplayResult{}, err
	}
	st, err := store.Open(ctx, config.ExpandHome(cfg.Audit.SQLite.Path))
	if err != nil {
		return ReplayResult{}, err
	}
	defer st.Close()
	env, err := st.GetEvent(ctx, opts.EventID)
	if err != nil {
		return ReplayResult{}, err
	}
	if opts.DryRun {
		return ReplayResult{DryRun: true, Event: env}, nil
	}
	result := dispatch.NewRunner(cfg.Handlers).Dispatch(ctx, env, "control", 2*time.Second)
	for _, inv := range result.Invocations {
		if err := st.InsertHandlerInvocation(ctx, store.HandlerInvocation{ID: harness.NewID("hinv"), NormalizedEventID: opts.EventID, HandlerName: inv.HandlerName, Kind: inv.Kind, StartedAt: inv.StartedAt, CompletedAt: inv.CompletedAt, Status: inv.Status, Stdout: inv.Stdout, Stderr: inv.Stderr, Output: inv.Output, Decision: inv.Decision, Error: inv.Error, ReplaySourceID: opts.EventID}); err != nil {
			return ReplayResult{}, err
		}
	}
	return ReplayResult{DryRun: false, Event: env, Aggregate: result.Aggregate}, nil
}
```

- [ ] **Step 5: Run app tests**

Run:

```sh
go test ./internal/app -count=1
```

Expected: PASS.

- [ ] **Step 6: Thin `cmd/hitch`**

In `cmd/hitch/main.go`, change `serve` to call `app.NewServerBundle`, start `bundle.Server.ListenAndServe`, then call `app.ShutdownContext` for shutdown. Change `replay` to call `app.Replay` and pass the returned `ReplayResult` to `writeCLI`.

The command should keep CLI output and exit behavior unchanged.

- [ ] **Step 7: Run CLI tests**

Run:

```sh
go test ./cmd/hitch ./internal/app -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```sh
git add cmd/hitch/main.go cmd/hitch/main_test.go internal/app
git commit -m "refactor: move daemon orchestration into app package"
```

---

### Task 4: Introduce consumer-owned API interfaces

**Files:**
- Modify: `internal/api/server.go`
- Modify: `internal/api/server_test.go`

- [ ] **Step 1: Add interface compile checks in tests**

Add to `internal/api/server_test.go` near test helpers:

```go
var _ eventStore = (*store.Store)(nil)
```

Expected initially: FAIL if `eventStore` does not exist.

- [ ] **Step 2: Define the narrow interface in `internal/api`**

In `internal/api/server.go`, add below `Server` or near types:

```go
type eventStore interface {
	InsertInbound(context.Context, store.InboundEvent) error
	InsertNormalized(context.Context, store.NormalizedEvent) error
	InsertHandlerInvocation(context.Context, store.HandlerInvocation) error
	InsertNativeResponse(context.Context, store.NativeResponse) error
	GetEvent(context.Context, string) (protocol.EventEnvelope, error)
	InspectEvent(context.Context, string) (store.EventInspection, error)
}
```

Change `Server.store` from `*store.Store` to `eventStore` while keeping `New(cfg config.Config, log *slog.Logger, st *store.Store) *Server` unchanged for callers.

- [ ] **Step 3: Run API tests**

Run:

```sh
go test ./internal/api -count=1
```

Expected: PASS.

- [ ] **Step 4: Evaluate dispatch interface only if tests need it**

If API tests still need heavy real handler execution setup, add:

```go
type dispatcher interface {
	Dispatch(context.Context, protocol.EventEnvelope, string, time.Duration) dispatch.Result
}
```

Then change `Server.runner` to `dispatcher`. If this does not reduce test setup, skip this sub-step and keep the concrete `dispatch.Runner`.

- [ ] **Step 5: Run affected tests**

Run:

```sh
go test ./internal/api ./internal/store ./internal/dispatch -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```sh
git add internal/api/server.go internal/api/server_test.go
git commit -m "refactor: narrow api persistence boundary"
```

---

### Task 5: Split installer package without behavior changes

**Files:**
- Modify: `internal/install/install.go`
- Create: `internal/install/catalog.go`
- Create: `internal/install/planning.go`
- Create: `internal/install/detect.go`
- Create: `internal/install/apply.go`
- Create: `internal/install/codex.go`
- Create: `internal/install/hermes.go`
- Create: `internal/install/pi.go`
- Create: `internal/install/opencode.go`
- Create: `internal/install/render.go`
- Existing tests: `internal/install/install_test.go`, `internal/install/install_script_test.go`

- [ ] **Step 1: Record current behavior**

Run:

```sh
go test ./internal/install -count=1
```

Expected: PASS before moving code.

- [ ] **Step 2: Move catalogs and specs**

Move these declarations from `install.go` to `catalog.go` without changing names or values:

```go
var codexLifecycleEvents = []string{...}
var hermesHookEvents = []string{...}
var hermesHookSyncEvents = map[string]struct{}{...}
var piExtensionEvents = []string{...}
var piExtensionSyncEvents = []string{...}
var ompExtensionEvents = []string{...}
var ompExtensionSyncEvents = []string{...}
var opencodeHookEvents = []string{...}
const piManagedExtensionMarker = "Managed by Hitch"
type harnessSpec struct { ... }
type harnessDetection struct { ... }
type installOperation struct { ... }
func knownHarnessSpecs() []harnessSpec { ... }
func knownHarnessNames() []string { ... }
```

Keep package name `install`. Do not export these identifiers.

- [ ] **Step 3: Run installer tests**

Run:

```sh
go test ./internal/install -count=1
```

Expected: PASS.

- [ ] **Step 4: Move detection helpers**

Move to `detect.go`:

```go
func detectHarnesses() []harnessDetection { ... }
func detectHarness(spec harnessSpec) harnessDetection { ... }
func defaultInteractiveSelection(detections []harnessDetection) []string { ... }
func stdinIsTerminal() bool { ... }
func confirmInstall(detections []harnessDetection, selected []string, uninstall bool) bool { ... }
func titleForHarness(name string) string { ... }
```

Run `go test ./internal/install -count=1`. Expected: PASS.

- [ ] **Step 5: Move planning helpers**

Move to `planning.go`:

```go
func plannedOps(harnesses []string, uninstall bool, apiURL string, pinURL bool) ([]installOperation, error) { ... }
func timestampedBackupPath(harnessName, filename string) string { ... }
func backupPath(harnessName string) string { ... }
func installedBinaryPath() (string, error) { ... }
func adapterCommandBase(clientPath, apiURL string) string { ... }
func extensionURLReason(apiURL string) string { ... }
func flagProvided(args []string, name string) bool { ... }
```

Run `go test ./internal/install -count=1`. Expected: PASS.

- [ ] **Step 6: Move apply/backup helpers**

Move to `apply.go`:

```go
func applyOps(ops []installOperation, uninstall bool) error { ... }
func applyPlaceholderOp(op installOperation, uninstall bool) error { ... }
func installExtensionContent(path, backup string, content []byte) error { ... }
func uninstallPiExtension(path, backup string) error { ... }
func uninstallOpenCodePlugin(path, backup string) error { ... }
func backupFile(path, backup string) error { ... }
```

Run `go test ./internal/install -count=1`. Expected: PASS.

- [ ] **Step 7: Move harness-specific helpers**

Move Codex helpers to `codex.go`:

```go
func codexHookInstalled(path string) bool { ... }
func installCodexHook(path, backup, binaryPath string) error { ... }
func uninstallCodexHook(path, backup string) error { ... }
func readCodexHooks(path string) (map[string]any, bool, error) { ... }
func codexAdapterCommand(commandBase, event string) string { ... }
func upsertCodexHook(doc map[string]any, event, command string) bool { ... }
func removeCodexHook(doc map[string]any, event string) bool { ... }
func codexEventHasManagedHook(doc map[string]any, event string) bool { ... }
func codexManagedHookNeedle(event string) string { ... }
func codexEventGroups(doc map[string]any, event string) []any { ... }
func setCodexEventGroups(doc map[string]any, event string, groups []any) { ... }
func hookCommandContains(hook map[string]any, needle string) bool { ... }
func writeJSONFile(path string, doc map[string]any) error { ... }
```

Move Hermes helpers to `hermes.go`:

```go
func hermesHookInstalled(path string) bool { ... }
func installHermesHooks(path, backup, binaryPath string) error { ... }
func uninstallHermesHooks(path, backup string) error { ... }
func readHermesConfig(path string) (*yaml.Node, bool, error) { ... }
func emptyYAMLDocument() *yaml.Node { ... }
func ensureDocumentMapping(doc *yaml.Node) *yaml.Node { ... }
func hermesAdapterCommand(commandBase, event string) string { ... }
func upsertHermesHook(doc *yaml.Node, event, command string) bool { ... }
func removeHermesHook(doc *yaml.Node, event string) bool { ... }
func hermesEventHasManagedHook(doc *yaml.Node, event string) bool { ... }
func hermesManagedHookNeedle(event string) string { ... }
func hermesHookNode(command string) *yaml.Node { ... }
func ensureYAMLMapping(parent *yaml.Node, key string) *yaml.Node { ... }
func ensureYAMLSequence(parent *yaml.Node, key string) *yaml.Node { ... }
func yamlMappingValue(mapping *yaml.Node, key string) *yaml.Node { ... }
func setYAMLMappingValue(mapping *yaml.Node, key string, value *yaml.Node) { ... }
func deleteYAMLMappingKey(mapping *yaml.Node, key string) { ... }
func yamlHookCommandContains(entry *yaml.Node, needle string) bool { ... }
func yamlIntScalar(value string) *yaml.Node { ... }
func yamlScalarValue(mapping *yaml.Node, key string) string { ... }
func yamlScalar(value string) *yaml.Node { ... }
func writeYAMLFile(path string, doc *yaml.Node) error { ... }
```

Move Pi/OMP helpers to `pi.go`:

```go
func piExtensionInstalled(path string) bool { ... }
func installPiExtension(path, backup, apiURL string) error { ... }
func installOMPExtension(path, backup, apiURL string) error { ... }
func piExtensionContent(apiURL string) ([]byte, error) { ... }
func ompExtensionContent(apiURL string) ([]byte, error) { ... }
func extensionContent(harnessName, clientVersion string, sourceEvents, syncEvents []string, apiURL string) ([]byte, error) { ... }
```

Move OpenCode helpers to `opencode.go`:

```go
func opencodePluginInstalled(path string) bool { ... }
func installOpenCodePlugin(path, backup, apiURL string) error { ... }
func opencodePluginContent(apiURL string) ([]byte, error) { ... }
func openCodePluginContent(harnessName, clientVersion string, sourceEvents []string, apiURL string) ([]byte, error) { ... }
```

Move render helpers to `render.go`:

```go
func jsonArrayLiteral(values []string) (string, error) { ... }
func jsonStringLiteral(value string) (string, error) { ... }
func shellQuote(s string) string { ... }
```

Run `go test ./internal/install -count=1` after each move. Expected: PASS after each move.

- [ ] **Step 8: Keep `install.go` focused on CLI entrypoint**

After moves, `install.go` should contain:

```go
func Run(args []string, uninstall bool) error { ... }
func flagSet(name string) *flag.FlagSet { ... }
func writeCLI(jsonOut bool, v interface{}) error { ... }
```

- [ ] **Step 9: Run all tests**

Run:

```sh
go test ./...
```

Expected: PASS.

- [ ] **Step 10: Commit**

```sh
git add internal/install
git commit -m "refactor: split installer responsibilities"
```

---

### Task 6: Align config validation with implemented backends

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/logging/logging.go`
- Modify: `internal/logging/logging_test.go`
- Modify: `README.md`
- Modify: `docs/configuration.md`

- [ ] **Step 1: Decide conservative behavior**

Use the conservative option: reject configured but unimplemented backends for now. This keeps runtime behavior honest and avoids accepting config that the daemon ignores.

- [ ] **Step 2: Add config tests for unsupported backends**

Add to `internal/config/config_test.go`:

```go
func TestRejectsUnimplementedAuditJSONLBackend(t *testing.T) {
	cfg := strings.Replace(baseConfig, `backend = "sqlite"`, `backend = "jsonl"`, 1)
	_, err := Parse([]byte(cfg))
	if err == nil || !strings.Contains(err.Error(), `audit.backend "jsonl" is not implemented`) {
		t.Fatalf("expected jsonl implementation error, got %v", err)
	}
}

func TestRejectsUnimplementedOTLPLogging(t *testing.T) {
	cfg := strings.Replace(baseConfig, `enabled = false
endpoint = "http://127.0.0.1:4318"`, `enabled = true
endpoint = "http://127.0.0.1:4318"`, 1)
	_, err := Parse([]byte(cfg))
	if err == nil || !strings.Contains(err.Error(), `log.otlp is not implemented`) {
		t.Fatalf("expected otlp implementation error, got %v", err)
	}
}
```

If `config_test.go` does not already import `strings`, add it.

- [ ] **Step 3: Verify tests fail**

Run:

```sh
go test ./internal/config -run 'TestRejectsUnimplemented' -count=1
```

Expected: FAIL because validation currently accepts these configs.

- [ ] **Step 4: Update validation**

In `internal/config/config.go`, change `Validate` audit backend handling:

```go
case "sqlite":
	if c.Audit.SQLite.Path == "" {
		return errors.New("audit.sqlite.path is required")
	}
case "jsonl":
	return errors.New(`audit.backend "jsonl" is not implemented`)
```

Change OTLP validation:

```go
if c.Log.OTLP.Enabled {
	return errors.New("log.otlp is not implemented")
}
```

- [ ] **Step 5: Run config tests**

Run:

```sh
go test ./internal/config -count=1
```

Expected: PASS.

- [ ] **Step 6: Update docs**

In `README.md`, keep the existing statement that SQLite is current verified backend. Remove or revise any text implying JSONL or OTLP are usable today.

In `docs/configuration.md`, document:

```markdown
Supported today:

- audit backend: `sqlite`
- log sinks: stdout and rolling file

Rejected until implemented:

- `audit.backend = "jsonl"`
- `[log.otlp].enabled = true`
```

- [ ] **Step 7: Run docs-adjacent and config tests**

Run:

```sh
go test ./internal/config ./internal/logging ./cmd/hitch -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```sh
git add internal/config/config.go internal/config/config_test.go internal/logging/logging.go internal/logging/logging_test.go README.md docs/configuration.md
git commit -m "fix: reject unimplemented config backends"
```

---

### Task 7: Add minimal static analysis configuration

**Files:**
- Create: `.golangci.yml`
- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add minimal lint config**

Create `.golangci.yml`:

```yaml
version: "2"

run:
  timeout: 5m

golinters-settings: {}

linters:
  enable:
    - govet
    - ineffassign
    - staticcheck
    - unused
```

If the installed golangci-lint version expects v1 config syntax, use this instead:

```yaml
run:
  timeout: 5m

linters:
  enable:
    - govet
    - ineffassign
    - staticcheck
    - unused
```

- [ ] **Step 2: Add Makefile target that gracefully requires the tool**

Add:

```make
lint:
	golangci-lint run
```

Update `.PHONY`:

```make
.PHONY: help build test test-go vet lint check run serve status doctor install-dry-run clean preview-pages
```

Do not include `lint` in `check` until the GitHub Action installs `golangci-lint`.

- [ ] **Step 3: Add CI lint installation and run**

In `.github/workflows/ci.yml`, add after Go setup and before `make check`:

```yaml
      - name: Run golangci-lint
        uses: golangci/golangci-lint-action@v8
        with:
          version: latest
```

Keep `make check` after lint.

- [ ] **Step 4: Run local verification if tool is installed**

Run:

```sh
golangci-lint run
```

Expected: PASS. If the binary is not installed locally, do not vendor or script-install it in the repo; rely on the GitHub Action and run `go vet ./...` locally.

- [ ] **Step 5: Run existing verification**

Run:

```sh
make check
```

Expected: PASS.

- [ ] **Step 6: Commit**

```sh
git add .golangci.yml Makefile .github/workflows/ci.yml
git commit -m "ci: add minimal go linting"
```

---

### Task 8: Decide API client package intent

**Files:**
- Modify: `internal/api/client.go`
- Modify: `cmd/hitch/main_test.go`
- Optional create: `internal/apitest/client.go`

- [ ] **Step 1: Verify current uses**

Run:

```sh
go test ./cmd/hitch ./internal/api -count=1
```

Expected: PASS before restructuring.

- [ ] **Step 2: Choose conservative action**

Keep `internal/api/client.go` if it is intentionally an internal integration-test helper. Add a package comment to make that explicit:

```go
// Client is an internal test and tool helper for exercising the Hitch HTTP API.
// It is not a public SDK because this module keeps implementation packages under internal/.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}
```

If maintainers want an external Go SDK later, create a separate `client` package in a dedicated task after the HTTP contract is stable.

- [ ] **Step 3: Run tests**

Run:

```sh
go test ./cmd/hitch ./internal/api -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```sh
git add internal/api/client.go
git commit -m "docs: clarify internal api client scope"
```

---

### Task 9: Final full verification and documentation pass

**Files:**
- Modify if needed: `README.md`
- Modify if needed: `docs/configuration.md`
- Modify if needed: `docs/handler-development.md`

- [ ] **Step 1: Run full checks**

Run:

```sh
make check
```

Expected: PASS.

- [ ] **Step 2: Run install smoke**

Run:

```sh
make install-dry-run
```

Expected: exits 0 and prints the dry-run install plan as before.

- [ ] **Step 3: Run focused packages after refactors**

Run:

```sh
go test ./internal/api ./internal/clientshim ./internal/harness/... ./internal/install ./cmd/hitch ./cmd/hitch-client -count=1
```

Expected: PASS.

- [ ] **Step 4: Confirm docs match actual commands**

Check that `README.md` common commands include working targets:

```markdown
make test
make test-go
make vet
make check
make build
make install-dry-run
```

Check that docs do not claim JSONL audit or OTLP logging are supported until implementation exists.

- [ ] **Step 5: Commit final docs if changed**

```sh
git add README.md docs/configuration.md docs/handler-development.md
git commit -m "docs: align go project structure guidance"
```

Skip this commit if no docs changed in Task 9.

---

## Execution order and risk controls

1. Task 1 first because CI currently has a likely false-positive gap.
2. Task 2 before API/app refactors because harness registration is duplicated and centralizing it reduces later churn.
3. Task 3 after Task 2 because app wiring should use the cleaner API construction.
4. Task 4 after Task 3 because narrower interfaces are easier once orchestration is out of `cmd/`.
5. Task 5 can run independently after Task 1; it is a pure same-package file split if no identifiers are renamed.
6. Task 6 must happen before docs finalization because it changes accepted config behavior.
7. Task 7 should happen after the codebase is stable to avoid lint churn during refactors.
8. Task 8 is documentation/scope clarification only.
9. Task 9 is the final integration gate.

## Success criteria

- `make -n test-go` prints `go test ./...`.
- CI and release workflows run real Go verification.
- `internal/api` no longer imports concrete harness adapter packages.
- `internal/clientshim` no longer duplicates harness adapter switching logic.
- `cmd/hitch/main.go` is mostly command parsing and process control, with service/replay logic in `internal/app`.
- `internal/install/install.go` is reduced to the install CLI entrypoint and small helpers; harness-specific code lives in focused files.
- Config validation rejects unimplemented JSONL audit and OTLP logging, or those features are implemented with tests. This plan chooses rejection.
- `go test ./...`, `go vet ./...`, and `make check` pass.
- Docs describe only supported behavior.

## Self-review

- Spec coverage: Covers every assessment finding: CI target drift, `api` adapter coupling, installer size, `cmd` orchestration, narrow interfaces, config/backend drift, linting, and API client scope.
- Placeholder scan: No task uses TBD/TODO/implement later. Code-moving steps name exact identifiers to move and require tests after each move.
- Type consistency: New names are stable: `harness.Registry`, `harness.DefaultRegistry`, `NewWithHarnessRegistry`, `app.ServerAddr`, `app.ShutdownContext`, `app.NewServerBundle`, `app.Replay`, `ReplayOptions`, `ReplayResult`.

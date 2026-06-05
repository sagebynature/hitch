# Install Mode UX Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an interactive `all`/`server`/`client` install-mode choice to `scripts/install.sh` while preserving current default behavior.

**Architecture:** Keep the installer as POSIX `sh` and add one early mode-selection function plus mode-aware build/install/configure branches in `main`. Cover behavior with a Go test that executes `scripts/install.sh` through fake `go` binaries so the script logic is tested without doing real builds.

**Tech Stack:** POSIX `sh`, Go `testing`, `os/exec`, existing Markdown docs.

---

## File Structure

- Modify `scripts/install.sh`: add `HITCH_INSTALL_MODE`, prompt/validation helpers, mode-aware binary build/copy/version/config/hook flow, and a PATH message that works for `hitch-client`-only installs.
- Create `internal/install/install_script_test.go`: shell-level installer tests using fake `go` output binaries and temporary `HOME`/install directories.
- Modify `docs/installation.md`: document the prompt, mode matrix, and non-interactive `HITCH_INSTALL_MODE` examples.

---

### Task 1: Add shell-level installer tests

**Files:**
- Create: `internal/install/install_script_test.go`

- [ ] **Step 1: Create failing tests for install modes**

Create `internal/install/install_script_test.go` with this content:

```go
package install

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type installScriptResult struct {
	Output     string
	Log        string
	InstallDir string
	Err        error
}

func TestSourceInstallerInstallModes(t *testing.T) {
	cases := []struct {
		name       string
		env        map[string]string
		wantFiles  []string
		denyFiles  []string
		wantLog    []string
		denyLog    []string
		wantOutput []string
	}{
		{
			name: "default all installs server and client",
			env: map[string]string{
				"HITCH_SKIP_HOOK_INSTALL": "1",
			},
			wantFiles: []string{"hitch", "hitch-client"},
			wantLog: []string{
				"hitch --version",
				"hitch-client --version",
				"hitch config init --json",
			},
			wantOutput: []string{"Installed Hitch to"},
		},
		{
			name: "server installs only server binary and config",
			env: map[string]string{
				"HITCH_INSTALL_MODE":      "server",
				"HITCH_SKIP_HOOK_INSTALL": "1",
			},
			wantFiles: []string{"hitch"},
			denyFiles: []string{"hitch-client"},
			wantLog: []string{
				"hitch --version",
				"hitch config init --json",
			},
			denyLog: []string{
				"hitch-client --version",
				"hitch-client install",
			},
			wantOutput: []string{"Installed Hitch server to"},
		},
		{
			name: "client installs only client binary and prints manual hook setup without tty",
			env: map[string]string{
				"HITCH_INSTALL_MODE": "client",
				"HITCH_URL":          "http://127.0.0.1:9876",
			},
			wantFiles: []string{"hitch-client"},
			denyFiles: []string{"hitch"},
			wantLog: []string{
				"hitch-client --version",
			},
			denyLog: []string{
				"hitch --version",
				"hitch config init --json",
				"hitch-client install",
			},
			wantOutput: []string{
				"Installed Hitch client to",
				"Run hook setup with:",
				"hitch-client install",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := runSourceInstaller(t, tc.env)
			if result.Err != nil {
				t.Fatalf("installer failed: %v\n%s", result.Err, result.Output)
			}
			for _, name := range tc.wantFiles {
				if _, err := os.Stat(filepath.Join(result.InstallDir, name)); err != nil {
					t.Fatalf("expected installed %s: %v\noutput:\n%s", name, err, result.Output)
				}
			}
			for _, name := range tc.denyFiles {
				if _, err := os.Stat(filepath.Join(result.InstallDir, name)); !os.IsNotExist(err) {
					t.Fatalf("did not expect installed %s: %v\noutput:\n%s", name, err, result.Output)
				}
			}
			for _, want := range tc.wantLog {
				if !strings.Contains(result.Log, want) {
					t.Fatalf("installer log missing %q\nlog:\n%s\noutput:\n%s", want, result.Log, result.Output)
				}
			}
			for _, deny := range tc.denyLog {
				if strings.Contains(result.Log, deny) {
					t.Fatalf("installer log unexpectedly contains %q\nlog:\n%s\noutput:\n%s", deny, result.Log, result.Output)
				}
			}
			for _, want := range tc.wantOutput {
				if !strings.Contains(result.Output, want) {
					t.Fatalf("installer output missing %q\noutput:\n%s", want, result.Output)
				}
			}
		})
	}
}

func TestSourceInstallerRejectsInvalidInstallModeBeforeBuild(t *testing.T) {
	result := runSourceInstaller(t, map[string]string{"HITCH_INSTALL_MODE": "bogus"})
	if result.Err == nil {
		t.Fatalf("expected invalid mode failure\noutput:\n%s", result.Output)
	}
	if !strings.Contains(result.Output, "Invalid HITCH_INSTALL_MODE") {
		t.Fatalf("expected invalid mode message\noutput:\n%s", result.Output)
	}
	if strings.Contains(result.Log, "go build") || strings.Contains(result.Output, "Building Hitch") {
		t.Fatalf("invalid mode should fail before build\nlog:\n%s\noutput:\n%s", result.Log, result.Output)
	}
}

func runSourceInstaller(t *testing.T, env map[string]string) installScriptResult {
	t.Helper()

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	installDir := t.TempDir()
	homeDir := t.TempDir()
	fakeBin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "installer.log")

	writeFakeGo(t, filepath.Join(fakeBin, "go"))

	cmd := exec.Command("sh", filepath.Join(repoRoot, "scripts", "install.sh"))
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HITCH_INSTALL_MODE=",
		"HITCH_SKIP_HOOK_INSTALL=",
		"HITCH_URL=",
		"HOME="+homeDir,
		"HITCH_SOURCE_DIR="+repoRoot,
		"HITCH_INSTALL_DIR="+installDir,
		"HITCH_TEST_LOG="+logPath,
	)
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err = cmd.Run()

	logBytes, readErr := os.ReadFile(logPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	return installScriptResult{Output: output.String(), Log: string(logBytes), InstallDir: installDir, Err: err}
}

func writeFakeGo(t *testing.T, path string) {
	t.Helper()
	const fakeGo = `#!/bin/sh
set -eu
printf 'go %s\n' "$*" >> "$HITCH_TEST_LOG"
out=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o)
      shift
      out=$1
      ;;
  esac
  shift || break
done
if [ -z "$out" ]; then
  printf 'fake go: missing -o\n' >&2
  exit 2
fi
mkdir -p "$(dirname "$out")"
cat > "$out" <<'EOS'
#!/bin/sh
name=${0##*/}
printf '%s %s\n' "$name" "$*" >> "$HITCH_TEST_LOG"
case "$name:$1:$2" in
  hitch:--version:*) printf 'hitch test-version\n' ;;
  hitch:config:init) printf '{"created":true}\n' ;;
  hitch-client:--version:*) printf 'hitch-client test-version\n' ;;
  hitch-client:install:*) printf '{"operations":[]}\n' ;;
esac
EOS
chmod 755 "$out"
`
	if err := os.WriteFile(path, []byte(fakeGo), 0o755); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run the focused test and verify it fails**

Run:

```bash
go test ./internal/install -run 'TestSourceInstaller(InstallModes|RejectsInvalidInstallModeBeforeBuild)' -count=1
```

Expected now: FAIL. Failures should show that `HITCH_INSTALL_MODE=server` still installs `hitch-client`, `HITCH_INSTALL_MODE=client` still installs `hitch`, and invalid mode does not produce the expected validation error.

- [ ] **Step 3: Commit the failing tests**

```bash
git add internal/install/install_script_test.go
git commit -m "test: cover source installer modes"
```

---

### Task 2: Implement install modes in `install.sh`

**Files:**
- Modify: `scripts/install.sh:4-177`

- [ ] **Step 1: Add mode environment variable and selection helpers**

In `scripts/install.sh`, add this environment default after `HITCH_URL=${HITCH_URL:-}`:

```sh
HITCH_INSTALL_MODE=${HITCH_INSTALL_MODE:-}
```

Add these functions after `prompt_tty()`:

```sh
normalize_install_mode() {
  case "$1" in
    "") printf 'all' ;;
    all|server|client) printf '%s' "$1" ;;
    *) return 1 ;;
  esac
}

select_install_mode() {
  if [ -n "$HITCH_INSTALL_MODE" ]; then
    if selected_mode=$(normalize_install_mode "$HITCH_INSTALL_MODE"); then
      HITCH_INSTALL_MODE=$selected_mode
      return 0
    fi
    printf 'Invalid HITCH_INSTALL_MODE: %s (expected all, server, or client).\n' "$HITCH_INSTALL_MODE" >&2
    exit 1
  fi

  if tty_available; then
    while :; do
      prompt_tty "Install Hitch mode [all/server/client] (all): " || break
      if selected_mode=$(normalize_install_mode "$answer"); then
        HITCH_INSTALL_MODE=$selected_mode
        return 0
      fi
      printf 'Please enter all, server, or client.\n' > /dev/tty
    done
  fi

  HITCH_INSTALL_MODE=all
}
```

- [ ] **Step 2: Make PATH detection binary-aware**

Replace `path_is_first()` with:

```sh
path_is_first() {
  cmd_name=$1
  cmd_path=$(command -v "$cmd_name" 2>/dev/null || true)
  [ "$cmd_path" = "$HITCH_INSTALL_DIR/$cmd_name" ]
}
```

Replace `maybe_update_path()` with this mode-neutral version:

```sh
maybe_update_path() {
  path_binary=$1
  installed_names=$2
  if path_is_first "$path_binary"; then
    return 0
  fi

  command_text=$(path_command)
  config_file=$(shell_config_file)
  printf '\nInstalled %s into %s, but that directory is not first on PATH.\n' "$installed_names" "$HITCH_INSTALL_DIR"
  if tty_available && prompt_tty "Add it to $config_file now? [Y/n] "; then
    case "$answer" in
      n|N|no|NO) ;;
      *)
        mkdir -p "$(dirname "$config_file")"
        touch "$config_file"
        if ! grep -Fxq "$command_text" "$config_file" 2>/dev/null; then
          printf '\n# Hitch\n%s\n' "$command_text" >> "$config_file"
          printf 'Added PATH update to %s.\n' "$config_file"
        fi
        ;;
    esac
  fi
  printf 'Restart your shell or run:\n\n  %s\n\n' "$command_text"
}
```

- [ ] **Step 3: Make `main` mode-aware**

At the start of `main()`, before `require_command go`, select and translate the mode:

```sh
  select_install_mode
  install_hitch=0
  install_client=0
  init_config=0
  configure_url=0
  setup_hooks=0
  path_binary=hitch
  installed_names='Hitch'
  case "$HITCH_INSTALL_MODE" in
    all)
      install_hitch=1
      install_client=1
      init_config=1
      configure_url=1
      setup_hooks=1
      path_binary=hitch
      installed_names='Hitch server and client'
      ;;
    server)
      install_hitch=1
      init_config=1
      path_binary=hitch
      installed_names='Hitch server'
      ;;
    client)
      install_client=1
      configure_url=1
      setup_hooks=1
      path_binary=hitch-client
      installed_names='Hitch client'
      ;;
  esac
```

Then replace the current unconditional build/copy/version/config block with mode-aware branches:

```sh
  printf 'Building Hitch from %s...\n' "$src_dir"
  if [ "$install_hitch" = 1 ]; then
    (cd "$src_dir" && go build -o "$tmp_dir/hitch" ./cmd/hitch)
  fi
  if [ "$install_client" = 1 ]; then
    (cd "$src_dir" && go build -o "$tmp_dir/hitch-client" ./cmd/hitch-client)
  fi

  mkdir -p "$HITCH_INSTALL_DIR"
  if [ "$install_hitch" = 1 ]; then
    cp "$tmp_dir/hitch" "$HITCH_INSTALL_DIR/hitch"
    chmod 755 "$HITCH_INSTALL_DIR/hitch"
    "$HITCH_INSTALL_DIR/hitch" --version
  fi
  if [ "$install_client" = 1 ]; then
    cp "$tmp_dir/hitch-client" "$HITCH_INSTALL_DIR/hitch-client"
    chmod 755 "$HITCH_INSTALL_DIR/hitch-client"
    "$HITCH_INSTALL_DIR/hitch-client" --version
  fi
  if [ "$init_config" = 1 ]; then
    "$HITCH_INSTALL_DIR/hitch" config init --json
  fi

  case "$HITCH_INSTALL_MODE" in
    all) printf 'Installed Hitch to %s/hitch and %s/hitch-client.\n' "$HITCH_INSTALL_DIR" "$HITCH_INSTALL_DIR" ;;
    server) printf 'Installed Hitch server to %s/hitch.\n' "$HITCH_INSTALL_DIR" ;;
    client) printf 'Installed Hitch client to %s/hitch-client.\n' "$HITCH_INSTALL_DIR" ;;
  esac
  maybe_update_path "$path_binary" "$installed_names"

  if [ "$setup_hooks" != 1 ] || [ "$HITCH_SKIP_HOOK_INSTALL" = 1 ]; then
    return 0
  fi

  if [ "$configure_url" = 1 ]; then
    configure_server_url
  fi

  if tty_available; then
    "$HITCH_INSTALL_DIR/hitch-client" install < /dev/tty
  else
    printf 'Run hook setup with:\n\n  %s/hitch-client install\n\n' "$HITCH_INSTALL_DIR"
  fi
```

Keep the existing `require_command go`, optional `require_command git`, temp directory, source directory, and clone logic around this block.

- [ ] **Step 4: Run syntax check**

Run:

```bash
sh -n scripts/install.sh
```

Expected: no output and exit code 0.

- [ ] **Step 5: Run focused installer tests and verify they pass**

Run:

```bash
go test ./internal/install -run 'TestSourceInstaller(InstallModes|RejectsInvalidInstallModeBeforeBuild)' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit implementation**

```bash
git add scripts/install.sh internal/install/install_script_test.go
git commit -m "feat: add source installer modes"
```

---

### Task 3: Document install modes

**Files:**
- Modify: `docs/installation.md:3-31`

- [ ] **Step 1: Update installation docs**

Replace the opening installer description in `docs/installation.md` lines 3-10 with:

````md
Install from latest source:

```sh
curl -fsSL https://raw.githubusercontent.com/sagebynature/hitch/main/scripts/install.sh | sh
```

The source installer checks for `git` and `go`, asks which install mode to use when `/dev/tty` is available, builds the selected binaries from source, installs them to `$HITCH_INSTALL_DIR` or `~/.local/bin`, verifies selected binary versions, and performs the selected setup steps.

Install modes:

| Mode | Behavior |
|---|---|
| `all` | Default. Install `hitch` and `hitch-client`, seed `~/.config/hitch/config.toml` with `hitch config init`, prompt for a Hitch server URL, and offer to run hook setup. |
| `server` | Install only `hitch` and seed server config. Skip `hitch-client`, server URL prompting, and hook setup. |
| `client` | Install only `hitch-client`, prompt for a Hitch server URL, and offer to run hook setup. Skip `hitch` and server config initialization. |

For non-interactive installs, set `HITCH_INSTALL_MODE` explicitly or omit it to keep the default `all` behavior:

```sh
HITCH_INSTALL_MODE=all sh scripts/install.sh
HITCH_INSTALL_MODE=server sh scripts/install.sh
HITCH_INSTALL_MODE=client HITCH_URL=http://127.0.0.1:8799 sh scripts/install.sh
```

Set `HITCH_SKIP_HOOK_INSTALL=1` to install binaries and skip hook setup in `all` or `client` mode.
````

- [ ] **Step 2: Run docs-targeted checks**

Run:

```bash
go test ./internal/install -run 'TestSourceInstaller(InstallModes|RejectsInvalidInstallModeBeforeBuild)' -count=1
```

Expected: PASS. This confirms docs did not accompany a broken installer state.

- [ ] **Step 3: Run installer shell syntax check again**

Run:

```bash
sh -n scripts/install.sh
```

Expected: no output and exit code 0.

- [ ] **Step 4: Commit docs**

```bash
git add docs/installation.md
git commit -m "docs: describe source installer modes"
```

---

### Task 4: Final focused verification

**Files:**
- Verify: `scripts/install.sh`
- Verify: `internal/install/install_script_test.go`
- Verify: `docs/installation.md`

- [ ] **Step 1: Run install package tests**

Run:

```bash
go test ./internal/install -count=1
```

Expected: PASS.

- [ ] **Step 2: Run full Go test suite**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Run shell syntax check**

Run:

```bash
sh -n scripts/install.sh
```

Expected: no output and exit code 0.

- [ ] **Step 4: Commit any final fixups**

If Task 4 required no file changes, do not create a commit. If it required a fix, commit only the changed files:

```bash
git add scripts/install.sh internal/install/install_script_test.go docs/installation.md
git commit -m "fix: stabilize source installer modes"
```

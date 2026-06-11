//go:build !windows
// +build !windows

package install

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

type buildTarget struct {
	Binary  string
	Package string
}

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
		wantBuild  []buildTarget
		denyBuild  []buildTarget
		wantLog    []string
		denyLog    []string
		wantOutput []string
		denyOutput []string
	}{
		{
			name: "default all installs server and client",
			env: map[string]string{
				"HITCH_SKIP_HOOK_INSTALL": "1",
			},
			wantFiles: []string{"hitch", "hitch-client"},
			wantBuild: []buildTarget{
				{Binary: "hitch", Package: "./cmd/hitch"},
				{Binary: "hitch-client", Package: "./cmd/hitch-client"},
			},
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
				"HITCH_INSTALL_MODE": "server",
			},
			wantFiles: []string{"hitch"},
			denyFiles: []string{"hitch-client"},
			wantBuild: []buildTarget{
				{Binary: "hitch", Package: "./cmd/hitch"},
			},
			denyBuild: []buildTarget{
				{Binary: "hitch-client", Package: "./cmd/hitch-client"},
			},
			wantLog: []string{
				"hitch --version",
				"hitch config init --json",
			},
			denyLog: []string{
				"hitch-client --version",
				"hitch-client install",
			},
			wantOutput: []string{"Installed Hitch server to"},
			denyOutput: []string{"Run hook setup with:"},
		},
		{
			name: "client installs only client binary and prints manual hook setup without tty",
			env: map[string]string{
				"HITCH_INSTALL_MODE": "client",
				"HITCH_URL":          "http://127.0.0.1:9876/a'b?x=1&y=2#frag",
			},
			wantFiles: []string{"hitch-client"},
			denyFiles: []string{"hitch"},
			wantBuild: []buildTarget{
				{Binary: "hitch-client", Package: "./cmd/hitch-client"},
			},
			denyBuild: []buildTarget{
				{Binary: "hitch", Package: "./cmd/hitch"},
			},
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
				"--url 'http://127.0.0.1:9876/a'\\''b?x=1&y=2#frag'",
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
			for _, want := range tc.wantBuild {
				if !hasBuildTarget(result.Log, want) {
					t.Fatalf("installer log missing build target %+v\nlog:\n%s\noutput:\n%s", want, result.Log, result.Output)
				}
			}
			for _, deny := range tc.denyBuild {
				if hasBuildTarget(result.Log, deny) {
					t.Fatalf("installer log unexpectedly contains build target %+v\nlog:\n%s\noutput:\n%s", deny, result.Log, result.Output)
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
			for _, deny := range tc.denyOutput {
				if strings.Contains(result.Output, deny) {
					t.Fatalf("installer output unexpectedly contains %q\noutput:\n%s", deny, result.Output)
				}
			}
		})
	}
}

func TestSourceInstallerInjectsRequestedVersion(t *testing.T) {
	result := runSourceInstaller(t, map[string]string{
		"HITCH_INSTALL_MODE": "client",
		"HITCH_VERSION":      "v9.8.7",
	})
	if result.Err != nil {
		t.Fatalf("installer failed: %v\n%s", result.Err, result.Output)
	}
	if !strings.Contains(result.Output, "Building Hitch 9.8.7 from") {
		t.Fatalf("installer output missing derived version\noutput:\n%s", result.Output)
	}
	if !strings.Contains(result.Log, "go build -ldflags -s -w -X main.version=9.8.7") {
		t.Fatalf("installer did not inject version ldflags\nlog:\n%s", result.Log)
	}
}
func hasBuildTarget(log string, target buildTarget) bool {
	want := "go build output=" + target.Binary + " package=" + target.Package
	for _, line := range strings.Split(log, "\n") {
		if line == want {
			return true
		}
	}
	return false
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
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
pkg=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o)
      shift
      out=$1
      ;;
    -*)
      ;;
    *)
      pkg=$1
      ;;
  esac
  shift || break
done
printf 'go build output=%s package=%s\n' "${out##*/}" "$pkg" >> "$HITCH_TEST_LOG"
if [ -z "$out" ]; then
  printf 'fake go: missing -o\n' >&2
  exit 2
fi
mkdir -p "$(dirname "$out")"
cat > "$out" <<'EOS'
#!/bin/sh
name=${0##*/}
printf '%s %s\n' "$name" "$*" >> "$HITCH_TEST_LOG"
case "$name:${1-}:${2-}" in
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

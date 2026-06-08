package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func TestResolveConfigPathDefaultsOnlyWhenFlagNotProvided(t *testing.T) {
	if got := resolveConfigPath("", false); got == "" {
		t.Fatal("empty unprovided config path did not default")
	}
	if got := resolveConfigPath("", true); got != "" {
		t.Fatalf("explicit empty config path was defaulted to %q", got)
	}
	if got := resolveConfigPath("/tmp/hitch.toml", true); got != "/tmp/hitch.toml" {
		t.Fatalf("explicit config path changed to %q", got)
	}
}

func TestNewServerBundleLogsExtensions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create extension dir
	extDir := filepath.Join(home, ".config", "hitch", "extensions", "test_ext")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	extTOML := `
name = "test_ext"
entrypoint = "main:handle"
kind = "observer"
hitch_events = ["*"]
timeout_ms = 1000
`
	if err := os.WriteFile(filepath.Join(extDir, "config.toml"), []byte(extTOML), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create main config
	configDir := filepath.Join(home, ".config", "hitch")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "events.sqlite")
	configTOML := `
[server]
host = "127.0.0.1"
port = 8790
max_request_bytes = 1048576

[log]
level = "info"
format = "json"

[log.stdout]
enabled = true

[audit]
enabled = true
backend = "sqlite"

[audit.sqlite]
path = "` + filepath.ToSlash(dbPath) + `"

[handlers.my_native]
type = "native"
extension = "test_ext"
`
	configPath := filepath.Join(configDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(configTOML), 0o644); err != nil {
		t.Fatal(err)
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	bundle, err := NewServerBundle(context.Background(), ServeOptions{ConfigPath: configPath, ConfigPathProvided: true})
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(out), "active extensions loaded") {
		t.Fatalf("expected log 'active extensions loaded' in stdout, got:\n%s", out)
	}
	if !strings.Contains(string(out), "test_ext") {
		t.Fatalf("expected log to contain extension name 'test_ext', got:\n%s", out)
	}
}


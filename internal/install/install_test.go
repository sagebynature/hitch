package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sagebynature/hitch/internal/config"
	"gopkg.in/yaml.v3"
)

func addFakeCommand(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return path
}

func prepareInstallerTest(t *testing.T, harness string) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HITCH_CLIENT_BINARY_PATH", filepath.Join(t.TempDir(), "hitch-client"))
	return addFakeCommand(t, harness)
}

func TestInstallDryRunPlansWithoutMutatingFilesystem(t *testing.T) {
	prepareInstallerTest(t, "codex")

	ops, err := plannedOps([]string{"codex"}, false, "")
	if err != nil {
		t.Fatal(err)
	}

	if len(ops) != 2 || ops[0].Action != "seed_config" || ops[1].Action != "install_codex_hook" {
		t.Fatalf("unexpected operations: %#v", ops)
	}
	if _, err := os.Stat(ops[0].Path); !os.IsNotExist(err) {
		t.Fatalf("dry-run planning should not create config file: %v", err)
	}
	if _, err := os.Stat(ops[1].Path); !os.IsNotExist(err) {
		t.Fatalf("dry-run planning should not create Codex hooks file: %v", err)
	}
}

func TestApplyOpsSeedsUserConfigWithoutOverwritingExistingFile(t *testing.T) {
	prepareInstallerTest(t, "codex")

	ops, err := plannedOps([]string{"codex"}, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	cfgPath := config.ExpandHome(config.DefaultPath)
	seeded, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(seeded) != config.DefaultConfigTOML {
		t.Fatalf("seeded config does not match default config")
	}
	if _, err := config.Parse(seeded); err != nil {
		t.Fatalf("seeded config is invalid: %v", err)
	}

	const existing = "user-owned config\n"
	if err := os.WriteFile(cfgPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != existing {
		t.Fatalf("installer overwrote existing user config: %q", current)
	}
}

func TestApplyOpsInstallsCodexHookIdempotentlyAndBacksUpExistingFile(t *testing.T) {
	prepareInstallerTest(t, "codex")

	ops, err := plannedOps([]string{"codex"}, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	hookOp := ops[1]
	first, err := os.ReadFile(hookOp.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range codexLifecycleEvents {
		needle := "-harness codex -event " + event + " -sync"
		if !strings.Contains(string(first), needle) {
			t.Fatalf("codex %s hook was not installed: %s", event, first)
		}
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hookOp.BackupPath); !os.IsNotExist(err) {
		t.Fatalf("idempotent install should not create backup: %v", err)
	}

	existing := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"existing"}]}]}}` + "\n"
	if err := os.WriteFile(hookOp.Path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(hookOp.BackupPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != existing {
		t.Fatalf("backup did not preserve previous content: %q", backup)
	}
	current, err := os.ReadFile(hookOp.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(current), "existing") || !strings.Contains(string(current), "-harness codex") {
		t.Fatalf("installed content did not preserve existing hook and add Hitch hook: %s", current)
	}
}

func TestApplyOpsUninstallRemovesOnlyManagedCodexHook(t *testing.T) {
	prepareInstallerTest(t, "codex")

	ops, err := plannedOps([]string{"codex"}, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	removeOps, err := plannedOps([]string{"codex"}, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(removeOps, true); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(ops[1].Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(current), "-harness codex") {
		t.Fatalf("uninstall should remove managed Codex hook: %s", current)
	}

}

func TestApplyOpsInstallsHermesHooksIdempotentlyAndBacksUpExistingFile(t *testing.T) {
	prepareInstallerTest(t, "hermes")

	ops, err := plannedOps([]string{"hermes"}, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 || ops[0].Action != "seed_config" || ops[1].Action != "install_hermes_hook" {
		t.Fatalf("unexpected operations: %#v", ops)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	hookOp := ops[1]
	first, err := os.ReadFile(hookOp.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range hermesHookEvents {
		needle := "-harness hermes -event " + event + " -sync"
		if !strings.Contains(string(first), needle) {
			t.Fatalf("hermes %s hook was not installed: %s", event, first)
		}
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hookOp.BackupPath); !os.IsNotExist(err) {
		t.Fatalf("idempotent Hermes install should not create backup: %v", err)
	}

	existing := "model: test\nhooks:\n  pre_tool_call:\n    - matcher: terminal\n      command: existing\nhooks_auto_accept: false\n"
	if err := os.WriteFile(hookOp.Path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(hookOp.BackupPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != existing {
		t.Fatalf("backup did not preserve previous Hermes config: %q", backup)
	}
	current, err := os.ReadFile(hookOp.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(current), "command: existing") || !strings.Contains(string(current), "-harness hermes") || !strings.Contains(string(current), "hooks_auto_accept: false") {
		t.Fatalf("Hermes install did not preserve existing config and add Hitch hooks: %s", current)
	}
}

func TestPlannedOpsEmbedsAdapterURLInHermesHookCommand(t *testing.T) {
	prepareInstallerTest(t, "hermes")

	ops, err := plannedOps([]string{"hermes"}, false, "http://127.0.0.1:8797")
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(ops[1].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(current), "-url ''http://127.0.0.1:8797'' -harness hermes") {
		t.Fatalf("Hermes hook command did not embed configured Hitch URL: %s", current)
	}
}

func TestPlannedOpsUsesHitchClientWhenAvailable(t *testing.T) {
	for _, harnessName := range []string{"codex", "hermes"} {
		t.Run(harnessName, func(t *testing.T) {
			prepareInstallerTest(t, harnessName)
			dir := t.TempDir()
			clientPath := filepath.Join(dir, "hitch-client")
			if err := os.WriteFile(clientPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HITCH_CLIENT_BINARY_PATH", clientPath)

			ops, err := plannedOps([]string{harnessName}, false, "http://127.0.0.1:8797")
			if err != nil {
				t.Fatal(err)
			}
			command := ops[1].Reason
			if !strings.Contains(command, shellQuote(clientPath)+" -url 'http://127.0.0.1:8797'") {
				t.Fatalf("hook command should prefer hitch-client, got %q", command)
			}
			if strings.Contains(command, " adapter ") {
				t.Fatalf("hitch-client command should not include adapter subcommand: %q", command)
			}
		})
	}
}

func TestUninstallMatchesLegacyAndClientManagedHooks(t *testing.T) {
	codexDoc := map[string]any{"hooks": map[string]any{"PreToolUse": []any{
		map[string]any{"matcher": "*", "hooks": []any{
			map[string]any{"type": "command", "command": "'/bin/hitch' adapter -url 'http://127.0.0.1:8797' -harness codex -event PreToolUse -sync"},
			map[string]any{"type": "command", "command": "'/bin/hitch-client' -url 'http://127.0.0.1:8797' -harness codex -event PreToolUse -sync"},
			map[string]any{"type": "command", "command": "existing"},
		}},
	}}}
	if !removeCodexHook(codexDoc, "PreToolUse") {
		t.Fatal("expected Codex managed hooks to be removed")
	}
	codexRaw, _ := json.Marshal(codexDoc)
	if strings.Contains(string(codexRaw), "-harness codex -event PreToolUse") {
		t.Fatalf("Codex uninstall kept a managed hook: %s", codexRaw)
	}
	if !strings.Contains(string(codexRaw), "existing") {
		t.Fatalf("Codex uninstall removed user hook: %s", codexRaw)
	}

	hermesDoc := emptyYAMLDocument()
	root := ensureDocumentMapping(hermesDoc)
	hooks := ensureYAMLMapping(root, "hooks")
	entries := ensureYAMLSequence(hooks, "pre_tool_call")
	entries.Content = append(entries.Content,
		hermesHookNode("'/bin/hitch' adapter -url 'http://127.0.0.1:8797' -harness hermes -event pre_tool_call -sync"),
		hermesHookNode("'/bin/hitch-client' -url 'http://127.0.0.1:8797' -harness hermes -event pre_tool_call -sync"),
		hermesHookNode("existing"),
	)
	if !removeHermesHook(hermesDoc, "pre_tool_call") {
		t.Fatal("expected Hermes managed hooks to be removed")
	}
	hermesRaw, _ := yaml.Marshal(hermesDoc)
	if strings.Contains(string(hermesRaw), "-harness hermes -event pre_tool_call") {
		t.Fatalf("Hermes uninstall kept a managed hook: %s", hermesRaw)
	}
	if !strings.Contains(string(hermesRaw), "existing") {
		t.Fatalf("Hermes uninstall removed user hook: %s", hermesRaw)
	}
}

func TestApplyOpsUninstallRemovesOnlyManagedHermesHooks(t *testing.T) {
	prepareInstallerTest(t, "hermes")

	ops, err := plannedOps([]string{"hermes"}, false, "")
	if err != nil {
		t.Fatal(err)
	}
	hookOp := ops[1]
	existing := "hooks:\n  pre_tool_call:\n    - matcher: terminal\n      command: existing\n"
	if err := os.MkdirAll(filepath.Dir(hookOp.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hookOp.Path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	removeOps, err := plannedOps([]string{"hermes"}, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(removeOps, true); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(hookOp.Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(current), "-harness hermes") {
		t.Fatalf("uninstall should remove managed Hermes hooks: %s", current)
	}
	if !strings.Contains(string(current), "command: existing") {
		t.Fatalf("uninstall should preserve user Hermes hooks: %s", current)
	}
}

func TestDetectHarnessesReportsAvailabilityAndSupport(t *testing.T) {
	path := prepareInstallerTest(t, "codex")

	detections := detectHarnesses()
	var codex harnessDetection
	for _, d := range detections {
		if d.Harness == "codex" {
			codex = d
			break
		}
	}
	if !codex.Available || codex.BinaryPath != path || !codex.Supported {
		t.Fatalf("unexpected codex detection: %#v", codex)
	}
}

func TestPlannedOpsSkipsUnsupportedAvailableHarness(t *testing.T) {
	prepareInstallerTest(t, "pi")

	ops, err := plannedOps([]string{"pi"}, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 || ops[1].Action != "skip" || ops[1].Status != "skipped" {
		t.Fatalf("unexpected unsupported harness plan: %#v", ops)
	}
}

func TestPlannedOpsRejectsUnknownHarness(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, err := plannedOps([]string{"unknown"}, false, ""); err == nil {
		t.Fatal("expected unsupported harness error")
	}
}

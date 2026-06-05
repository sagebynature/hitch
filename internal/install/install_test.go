package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	ops, err := plannedOps([]string{"codex"}, false, "", false)
	if err != nil {
		t.Fatal(err)
	}

	if len(ops) != 1 || ops[0].Action != "install_codex_hook" {
		t.Fatalf("unexpected operations: %#v", ops)
	}
	if _, err := os.Stat(ops[0].Path); !os.IsNotExist(err) {
		t.Fatalf("dry-run planning should not create Codex hooks file: %v", err)
	}
}

func TestApplyOpsDoesNotSeedServerConfig(t *testing.T) {
	prepareInstallerTest(t, "codex")

	ops, err := plannedOps([]string{"codex"}, false, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(os.Getenv("HOME"), ".config", "hitch", "config.toml")
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Fatalf("hook installer should not seed server config: %v", err)
	}
}

func TestApplyOpsInstallsCodexHookIdempotentlyAndBacksUpExistingFile(t *testing.T) {
	prepareInstallerTest(t, "codex")

	ops, err := plannedOps([]string{"codex"}, false, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	hookOp := ops[0]
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

	ops, err := plannedOps([]string{"codex"}, false, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	removeOps, err := plannedOps([]string{"codex"}, true, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(removeOps, true); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(ops[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(current), "-harness codex") {
		t.Fatalf("uninstall should remove managed Codex hook: %s", current)
	}

}

func TestApplyOpsInstallsHermesHooksIdempotentlyAndBacksUpExistingFile(t *testing.T) {
	prepareInstallerTest(t, "hermes")

	ops, err := plannedOps([]string{"hermes"}, false, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Action != "install_hermes_hook" {
		t.Fatalf("unexpected operations: %#v", ops)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	hookOp := ops[0]
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

	ops, err := plannedOps([]string{"hermes"}, false, "http://127.0.0.1:8797", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(ops[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(current), "-url ''http://127.0.0.1:8797'' -harness hermes") {
		t.Fatalf("Hermes hook command did not embed configured Hitch URL: %s", current)
	}
}

func TestPlannedOpsOmitsAdapterURLWhenNotPinned(t *testing.T) {
	for _, harnessName := range []string{"codex", "hermes"} {
		t.Run(harnessName, func(t *testing.T) {
			prepareInstallerTest(t, harnessName)

			ops, err := plannedOps([]string{harnessName}, false, "", false)
			if err != nil {
				t.Fatal(err)
			}
			command := ops[0].Reason
			if strings.Contains(command, "-url") {
				t.Fatalf("unpinned hook command should resolve URL at runtime, got %q", command)
			}
			if !strings.HasSuffix(command, "hitch-client'") {
				t.Fatalf("hook command should be hitch-client only before harness flags, got %q", command)
			}
		})
	}
}

func TestExtensionContentEmbedsQuotedValuesAsJSONLiterals(t *testing.T) {
	wantURL := "http://127.0.0.1:8797/path?name=\"quoted\"&slash=\\"
	wantEvents := []string{"turn\"end", "path\\name", "line\nbreak"}
	contentBytes, err := extensionContent("pi\"harness", "client\"version\\", wantEvents, wantURL)
	if err != nil {
		t.Fatal(err)
	}
	content := string(contentBytes)

	var gotURL string
	if err := json.Unmarshal([]byte(jsValueBetween(t, content, `const HITCH_PINNED_API_URL = `, ";\n")), &gotURL); err != nil {
		t.Fatal(err)
	}
	if gotURL != wantURL {
		t.Fatalf("API URL literal decoded to %q, want %q", gotURL, wantURL)
	}

	var gotEvents []string
	if err := json.Unmarshal([]byte(jsValueBetween(t, content, `const HITCH_EVENTS = `, ";\n")), &gotEvents); err != nil {
		t.Fatal(err)
	}
	if len(gotEvents) != len(wantEvents) {
		t.Fatalf("events literal decoded to %#v, want %#v", gotEvents, wantEvents)
	}
	for i := range wantEvents {
		if gotEvents[i] != wantEvents[i] {
			t.Fatalf("events literal decoded to %#v, want %#v", gotEvents, wantEvents)
		}
	}

	var gotHarness string
	if err := json.Unmarshal([]byte(jsValueBetween(t, content, `        harness: `, ",\n        source_event_type")), &gotHarness); err != nil {
		t.Fatal(err)
	}
	if gotHarness != `pi"harness` {
		t.Fatalf("harness literal decoded to %q", gotHarness)
	}

	var gotVersion string
	if err := json.Unmarshal([]byte(jsValueBetween(t, content, `        hitch_client_version: `, "\n      })")), &gotVersion); err != nil {
		t.Fatal(err)
	}
	if gotVersion != `client"version\` {
		t.Fatalf("client version literal decoded to %q", gotVersion)
	}
}

func jsValueBetween(t *testing.T, content, prefix, suffix string) string {
	t.Helper()
	_, after, ok := strings.Cut(content, prefix)
	if !ok {
		t.Fatalf("missing prefix %q in generated extension:\n%s", prefix, content)
	}
	value, _, ok := strings.Cut(after, suffix)
	if !ok {
		t.Fatalf("missing suffix %q after prefix %q in generated extension:\n%s", suffix, prefix, content)
	}
	return value
}

func TestApplyOpsInstallsPiExtensionIdempotentlyAndBacksUpExistingFile(t *testing.T) {
	prepareInstallerTest(t, "pi")

	ops, err := plannedOps([]string{"pi"}, false, "http://127.0.0.1:8797", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Action != "install_pi_extension" {
		t.Fatalf("unexpected operations: %#v", ops)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	hookOp := ops[0]
	first, err := os.ReadFile(hookOp.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), `const HITCH_PINNED_API_URL = "http://127.0.0.1:8797";`) || !strings.Contains(string(first), `function hitchAPIURL()`) || !strings.Contains(string(first), `envString("HITCH_URL")`) || !strings.Contains(string(first), `pi.on(sourceEventType, async (event, ctx)`) || !strings.Contains(string(first), `metadata: collectMetadata(event, ctx)`) || !strings.Contains(string(first), `"tool_call"`) || !strings.Contains(string(first), `source_event_type: sourceEventType`) {
		t.Fatalf("Pi extension did not embed expected adapter logic: %s", first)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hookOp.BackupPath); !os.IsNotExist(err) {
		t.Fatalf("idempotent Pi install should not create backup: %v", err)
	}

	existing := "// user-owned extension\nexport default function(pi) {}\n"
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
		t.Fatalf("backup did not preserve previous Pi extension: %q", backup)
	}
}

func TestApplyOpsInstallsOMPExtensionIdempotentlyAndBacksUpExistingFile(t *testing.T) {
	prepareInstallerTest(t, "omp")

	ops, err := plannedOps([]string{"omp"}, false, "http://127.0.0.1:8797", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Action != "install_omp_extension" {
		t.Fatalf("unexpected operations: %#v", ops)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	hookOp := ops[0]
	first, err := os.ReadFile(hookOp.Path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(first)
	for _, needle := range []string{
		`const HITCH_PINNED_API_URL = "http://127.0.0.1:8797";`,
		`pi.on(sourceEventType, async (event, ctx)`,
		`metadata: collectMetadata(event, ctx)`,
		`"session_before_branch"`,
		`"session.compacting"`,
		`harness: "omp"`,
		`hitch_client_version: "hitch-omp-extension"`,
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("OMP extension missing %q: %s", needle, content)
		}
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hookOp.BackupPath); !os.IsNotExist(err) {
		t.Fatalf("idempotent OMP install should not create backup: %v", err)
	}

	existing := "// user-owned extension\nexport default function(pi) {}\n"
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
		t.Fatalf("backup did not preserve previous OMP extension: %q", backup)
	}
}

func TestApplyOpsInstallsOpenCodePluginIdempotentlyAndBacksUpExistingFile(t *testing.T) {
	prepareInstallerTest(t, "opencode")

	ops, err := plannedOps([]string{"opencode"}, false, "http://127.0.0.1:9000", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Action != "install_opencode_plugin" {
		t.Fatalf("unexpected operations: %#v", ops)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	hookOp := ops[0]
	first, err := os.ReadFile(hookOp.Path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(first)
	for _, want := range []string{
		"Managed by Hitch",
		`const HITCH_PINNED_API_URL = "http://127.0.0.1:9000";`,
		"export const HitchPlugin",
		`"tool.execute.before"`,
		`"permission.ask"`,
		"/v1/dispatch-sync",
		"/v1/events",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("OpenCode plugin missing %q:\n%s", want, content)
		}
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hookOp.BackupPath); !os.IsNotExist(err) {
		t.Fatalf("idempotent OpenCode install should not create backup: %v", err)
	}

	existing := "export const Existing = async () => ({})\n"
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
		t.Fatalf("backup did not preserve previous OpenCode plugin: %q", backup)
	}
}

func TestApplyOpsUninstallRemovesOnlyManagedOpenCodePlugin(t *testing.T) {
	prepareInstallerTest(t, "opencode")

	ops, err := plannedOps([]string{"opencode"}, false, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	removeOps, err := plannedOps([]string{"opencode"}, true, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(removeOps, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ops[0].Path); !os.IsNotExist(err) {
		t.Fatalf("managed OpenCode plugin should be removed: %v", err)
	}

	userOwned := "// Managed by Hitch, but not generated by this installer\nexport const Existing = async () => ({ dispatchToHitch: true })\n"
	if err := os.MkdirAll(filepath.Dir(ops[0].Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ops[0].Path, []byte(userOwned), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyOps(removeOps, true); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(ops[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != userOwned {
		t.Fatalf("OpenCode uninstall removed or changed user-owned plugin: %q", current)
	}
}

func TestOpenCodePluginContentAppliesThrowSetAppendAndInjectContext(t *testing.T) {
	contentBytes, err := opencodePluginContent("")
	if err != nil {
		t.Fatal(err)
	}
	content := string(contentBytes)
	for _, want := range []string{"function applyAdapterResponse", "async function postToHitch", "async function observeWithHitch", `hitchAPIURL() + "/v1/events"`, `postToHitch("/v1/dispatch-sync"`, `adapter_action === "throw"`, `adapter_action === "set"`, `adapter_action === "append"`, `adapter_action === "inject_context"`, "client.session.prompt", "HITCH_DEFAULT_API_URL"} {
		if !strings.Contains(content, want) {
			t.Fatalf("generated plugin missing %q:\n%s", want, content)
		}
	}
}

func TestApplyOpsUninstallRemovesOnlyManagedPiExtension(t *testing.T) {
	prepareInstallerTest(t, "pi")

	ops, err := plannedOps([]string{"pi"}, false, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(ops, false); err != nil {
		t.Fatal(err)
	}
	removeOps, err := plannedOps([]string{"pi"}, true, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyOps(removeOps, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ops[0].Path); !os.IsNotExist(err) {
		t.Fatalf("managed Pi extension should be removed: %v", err)
	}

	userOwned := "// user extension\n"
	if err := os.MkdirAll(filepath.Dir(ops[0].Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ops[0].Path, []byte(userOwned), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyOps(removeOps, true); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(ops[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != userOwned {
		t.Fatalf("Pi uninstall removed or changed user-owned extension: %q", current)
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

			ops, err := plannedOps([]string{harnessName}, false, "http://127.0.0.1:8797", true)
			if err != nil {
				t.Fatal(err)
			}
			command := ops[0].Reason
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

	ops, err := plannedOps([]string{"hermes"}, false, "", false)
	if err != nil {
		t.Fatal(err)
	}
	hookOp := ops[0]
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
	removeOps, err := plannedOps([]string{"hermes"}, true, "", false)
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

func TestPlannedOpsInstallsAvailableOMPHarness(t *testing.T) {
	prepareInstallerTest(t, "omp")

	ops, err := plannedOps([]string{"omp"}, false, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0].Action != "install_omp_extension" || ops[0].Status != "planned" {
		t.Fatalf("unexpected OMP harness plan: %#v", ops)
	}
	if !strings.HasSuffix(ops[0].Path, filepath.Join(".omp", "agent", "extensions", "hitch", "index.ts")) {
		t.Fatalf("unexpected OMP extension path: %q", ops[0].Path)
	}
}

func TestPlannedOpsRejectsUnknownHarness(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, err := plannedOps([]string{"unknown"}, false, "", false); err == nil {
		t.Fatal("expected unsupported harness error")
	}
}

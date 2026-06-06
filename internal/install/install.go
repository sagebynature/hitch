package install

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sagebynature/hitch/internal/config"
	"gopkg.in/yaml.v3"
)

var codexLifecycleEvents = []string{
	"SessionStart",
	"SubagentStart",
	"UserPromptSubmit",
	"PreToolUse",
	"PermissionRequest",
	"PostToolUse",
	"PreCompact",
	"PostCompact",
	"SubagentStop",
	"Stop",
}

var hermesHookEvents = []string{
	"pre_tool_call",
	"post_tool_call",
	"pre_llm_call",
	"post_llm_call",
	"on_session_start",
	"on_session_end",
	"subagent_stop",
	"transform_tool_result",
	"transform_terminal_output",
	"transform_llm_output",
	"pre_gateway_dispatch",
}

var piExtensionEvents = []string{
	"input",
	"before_agent_start",
	"agent_start",
	"turn_start",
	"context",
	"before_provider_request",
	"tool_call",
	"tool_result",
	"turn_end",
	"agent_end",
	"session_start",
	"session_shutdown",
	"session_before_switch",
	"session_before_fork",
	"session_before_compact",
	"session_compact",
	"user_bash",
}

var piExtensionSyncEvents = []string{
	"input",
	"context",
	"before_provider_request",
	"tool_call",
	"tool_result",
	"session_before_switch",
	"session_before_fork",
	"session_before_compact",
}

var ompExtensionEvents = []string{
	"input",
	"before_agent_start",
	"agent_start",
	"agent_end",
	"turn_start",
	"turn_end",
	"before_provider_request",
	"after_provider_response",
	"context",
	"message_start",
	// "message_update",
	"message_end",
	"tool_call",
	"tool_result",
	"tool_execution_start",
	"tool_execution_update",
	"tool_execution_end",
	"session_start",
	"session_before_switch",
	"session_switch",
	"session_before_branch",
	"session_branch",
	"session_before_compact",
	"session.compacting",
	"session_compact",
	"session_before_tree",
	"session_tree",
	"session_shutdown",
	"auto_compaction_start",
	"auto_compaction_end",
	"auto_retry_start",
	"auto_retry_end",
	"ttsr_triggered",
	"todo_reminder",
	"goal_updated",
	"credential_disabled",
	"user_bash",
	"user_python",
}
var ompExtensionSyncEvents = []string{
	"input",
	"context",
	"before_provider_request",
	"tool_call",
	"tool_result",
	"session_before_switch",
	"session_before_branch",
	"session_before_compact",
	"session.compacting",
	"session_before_tree",
}

var opencodeHookEvents = []string{
	"chat.message",
	"chat.params",
	"chat.headers",
	"command.execute.before",
	"command.executed",
	"permission.ask",
	"permission.asked",
	"permission.updated",
	"permission.replied",
	"tool.execute.before",
	"tool.execute.after",
	"tool.definition",
	"shell.env",
	"experimental.session.compacting",
	"experimental.compaction.autocontinue",
	"experimental.text.complete",
	"session.created",
	"session.updated",
	"session.deleted",
	"session.diff",
	"session.error",
	"session.idle",
	"session.status",
	"session.compacted",
	"message.updated",
	"message.removed",
	"message.part.updated",
	"message.part.removed",
	"file.edited",
	"file.watcher.updated",
	"todo.updated",
	"server.connected",
	"server.instance.disposed",
	"installation.updated",
	"installation.update-available",
	"lsp.client.diagnostics",
	"lsp.updated",
	"tui.prompt.append",
	"tui.command.execute",
	"tui.toast.show",
	"pty.created",
	"pty.updated",
	"pty.exited",
	"pty.deleted",
	"vcs.branch.updated",
}

const piManagedExtensionMarker = "Managed by Hitch"

type harnessSpec struct {
	Name       string
	Title      string
	Command    string
	ConfigPath string
	Supported  bool
	Reason     string
}

type harnessDetection struct {
	Harness    string `json:"harness"`
	Available  bool   `json:"available"`
	Reason     string `json:"reason"`
	BinaryPath string `json:"binary_path,omitempty"`
	ConfigPath string `json:"config_path,omitempty"`
	Installed  bool   `json:"installed"`
	Supported  bool   `json:"supported"`
}

type installOperation struct {
	Harness    string `json:"harness"`
	Action     string `json:"action"`
	Path       string `json:"path,omitempty"`
	BackupPath string `json:"backup_path,omitempty"`
	Status     string `json:"status,omitempty"`
	Reason     string `json:"reason,omitempty"`
	AdapterURL string `json:"-"`
}

func knownHarnessSpecs() []harnessSpec {
	return []harnessSpec{
		{Name: "codex", Title: "Codex", Command: "codex", ConfigPath: "~/.codex/hooks.json", Supported: true},
		{Name: "hermes", Title: "Hermes", Command: "hermes", ConfigPath: "~/.hermes/config.yaml", Supported: true},
		{Name: "pi", Title: "Pi", Command: "pi", ConfigPath: "~/.pi/agent/extensions/hitch/index.ts", Supported: true},
		{Name: "omp", Title: "OMP", Command: "omp", ConfigPath: "~/.omp/agent/extensions/hitch/index.ts", Supported: true},
		{Name: "opencode", Title: "OpenCode", Command: "opencode", ConfigPath: "~/.config/opencode/plugins/hitch.ts", Supported: true},
	}
}

func Run(args []string, uninstall bool) error {
	commandName := "install"
	if uninstall {
		commandName = "uninstall"
	}
	fs := flagSet(commandName)
	only := fs.String("only", "codex,hermes,pi,omp,opencode", "comma-separated harness list")
	dryRun := fs.Bool("dry-run", false, "show changes without writing")
	yes := fs.Bool("yes", false, "confirm filesystem changes")
	jsonOut := fs.Bool("json", false, "emit JSON")
	all := fs.Bool("all", false, "select all harnesses")
	apiURL := fs.String("url", "", "hitch API URL pinned into installed hook commands/extensions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pinURL := flagProvided(args, "url")

	detections := detectHarnesses()
	selected := strings.Split(*only, ",")
	onlyProvided := flagProvided(args, "only")
	if *all {
		selected = knownHarnessNames()
	} else if !onlyProvided {
		selected = defaultInteractiveSelection(detections)
	}
	if !*dryRun && !*yes {
		if !stdinIsTerminal() {
			return fmt.Errorf("--yes is required unless --dry-run is used")
		}
		if !confirmInstall(detections, selected, uninstall) {
			return writeCLI(*jsonOut, map[string]interface{}{"dry_run": *dryRun, "uninstall": uninstall, "harnesses": detections, "operations": []installOperation{}})
		}
	}

	ops, err := plannedOps(selected, uninstall, *apiURL, pinURL)
	if err != nil {
		return err
	}
	if !*dryRun {
		if err := applyOps(ops, uninstall); err != nil {
			return err
		}
	}
	return writeCLI(*jsonOut, map[string]interface{}{"dry_run": *dryRun, "uninstall": uninstall, "harnesses": detections, "operations": ops})
}

func flagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func writeCLI(jsonOut bool, v interface{}) error {
	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(v)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, string(b))
	return err
}

func knownHarnessNames() []string {
	specs := knownHarnessSpecs()
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}
	return names
}

func flagProvided(args []string, name string) bool {
	long := "--" + name
	short := "-" + name
	for _, arg := range args {
		if arg == long || arg == short || strings.HasPrefix(arg, long+"=") || strings.HasPrefix(arg, short+"=") {
			return true
		}
	}
	return false
}

func detectHarnesses() []harnessDetection {
	specs := knownHarnessSpecs()
	out := make([]harnessDetection, 0, len(specs))
	for _, spec := range specs {
		out = append(out, detectHarness(spec))
	}
	return out
}

func detectHarness(spec harnessSpec) harnessDetection {
	binary, err := exec.LookPath(spec.Command)
	d := harnessDetection{Harness: spec.Name, ConfigPath: config.ExpandHome(spec.ConfigPath), Supported: spec.Supported}
	if err == nil {
		d.Available = true
		d.BinaryPath = binary
		d.Reason = spec.Command + " found on PATH"
	} else {
		d.Reason = spec.Command + " not found on PATH"
	}
	if spec.Reason != "" && !spec.Supported && d.Available {
		d.Reason = spec.Reason
	}
	switch spec.Name {
	case "codex":
		d.Installed = codexHookInstalled(d.ConfigPath)
	case "hermes":
		d.Installed = hermesHookInstalled(d.ConfigPath)
	case "pi", "omp":
		d.Installed = piExtensionInstalled(d.ConfigPath)
	case "opencode":
		d.Installed = opencodePluginInstalled(d.ConfigPath)
	}
	return d
}

func defaultInteractiveSelection(detections []harnessDetection) []string {
	selected := make([]string, 0, len(detections))
	for _, d := range detections {
		if d.Available && d.Supported {
			selected = append(selected, d.Harness)
		}
	}
	return selected
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func confirmInstall(detections []harnessDetection, selected []string, uninstall bool) bool {
	selectedSet := map[string]struct{}{}
	for _, h := range selected {
		selectedSet[strings.TrimSpace(h)] = struct{}{}
	}
	verb := "Install Hitch hooks"
	if uninstall {
		verb = "Uninstall Hitch hooks"
	}
	fmt.Fprintln(os.Stderr, "Hitch found these harnesses:")
	fmt.Fprintln(os.Stderr)
	for _, d := range detections {
		mark := " "
		if _, ok := selectedSet[d.Harness]; ok && d.Available && d.Supported {
			mark = "x"
		}
		status := d.Reason
		if d.BinaryPath != "" {
			status = "found at " + d.BinaryPath
		}
		if !d.Supported {
			status = d.Reason
		}
		fmt.Fprintf(os.Stderr, "  [%s] %-7s %s\n", mark, titleForHarness(d.Harness), status)
	}
	fmt.Fprintf(os.Stderr, "\n%s for selected harnesses? [Y/n] ", verb)
	var answer string
	_, _ = fmt.Fscanln(os.Stdin, &answer)
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "", "y", "yes":
		return true
	default:
		return false
	}
}

func titleForHarness(name string) string {
	for _, spec := range knownHarnessSpecs() {
		if spec.Name == name {
			return spec.Title
		}
	}
	return name
}

func plannedOps(harnesses []string, uninstall bool, apiURL string, pinURL bool) ([]installOperation, error) {
	ops := []installOperation{}
	known := map[string]harnessSpec{}
	for _, spec := range knownHarnessSpecs() {
		known[spec.Name] = spec
	}
	binaryPath, err := installedBinaryPath()
	if err != nil && !uninstall {
		return nil, err
	}
	if !pinURL {
		apiURL = ""
	}
	for _, h := range harnesses {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		spec, ok := known[h]
		if !ok {
			return nil, fmt.Errorf("unsupported harness %q", h)
		}
		detection := detectHarness(spec)
		if !detection.Available {
			ops = append(ops, installOperation{Harness: h, Action: "skip", Status: "skipped", Reason: detection.Reason})
			continue
		}
		if !detection.Supported {
			ops = append(ops, installOperation{Harness: h, Action: "skip", Status: "skipped", Reason: detection.Reason, Path: detection.ConfigPath})
			continue
		}
		switch h {
		case "codex":
			action := "install_codex_hook"
			if uninstall {
				action = "uninstall_codex_hook"
			}
			ops = append(ops, installOperation{Harness: h, Action: action, Path: detection.ConfigPath, BackupPath: timestampedBackupPath(h, filepath.Base(detection.ConfigPath)), Status: "planned", Reason: adapterCommandBase(binaryPath, apiURL)})
		case "hermes":
			action := "install_hermes_hook"
			if uninstall {
				action = "uninstall_hermes_hook"
			}
			ops = append(ops, installOperation{Harness: h, Action: action, Path: detection.ConfigPath, BackupPath: timestampedBackupPath(h, filepath.Base(detection.ConfigPath)), Status: "planned", Reason: adapterCommandBase(binaryPath, apiURL)})
		case "pi":
			action := "install_pi_extension"
			if uninstall {
				action = "uninstall_pi_extension"
			}
			ops = append(ops, installOperation{Harness: h, Action: action, Path: detection.ConfigPath, BackupPath: timestampedBackupPath(h, filepath.Base(detection.ConfigPath)), Status: "planned", Reason: extensionURLReason(apiURL), AdapterURL: apiURL})
		case "omp":
			action := "install_omp_extension"
			if uninstall {
				action = "uninstall_omp_extension"
			}
			ops = append(ops, installOperation{Harness: h, Action: action, Path: detection.ConfigPath, BackupPath: timestampedBackupPath(h, filepath.Base(detection.ConfigPath)), Status: "planned", Reason: extensionURLReason(apiURL), AdapterURL: apiURL})
		case "opencode":
			action := "install_opencode_plugin"
			if uninstall {
				action = "uninstall_opencode_plugin"
			}
			ops = append(ops, installOperation{Harness: h, Action: action, Path: detection.ConfigPath, BackupPath: timestampedBackupPath(h, filepath.Base(detection.ConfigPath)), Status: "planned", Reason: extensionURLReason(apiURL), AdapterURL: apiURL})
		}
	}
	return ops, nil
}

func timestampedBackupPath(harnessName, filename string) string {
	stamp := time.Now().UTC().Format("20060102T150405Z")
	return filepath.Join(config.ExpandHome("~/.config/hitch/backups"), harnessName, stamp+"-"+filename)
}

func backupPath(harnessName string) string {
	return filepath.Join(config.ExpandHome("~/.config/hitch/backups"), harnessName+".txt.bak")
}

func installedBinaryPath() (string, error) {
	if override := os.Getenv("HITCH_CLIENT_BINARY_PATH"); override != "" {
		return filepath.Abs(override)
	}
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(p)
}

func adapterCommandBase(clientPath, apiURL string) string {
	if apiURL == "" {
		return shellQuote(clientPath)
	}
	return shellQuote(clientPath) + " -url " + shellQuote(apiURL)
}

func extensionURLReason(apiURL string) string {
	if apiURL == "" {
		return "runtime URL resolver"
	}
	return apiURL
}

func applyOps(ops []installOperation, uninstall bool) error {
	for _, op := range ops {
		switch op.Action {
		case "install_codex_hook":
			if err := installCodexHook(op.Path, op.BackupPath, op.Reason); err != nil {
				return err
			}
		case "uninstall_codex_hook":
			if err := uninstallCodexHook(op.Path, op.BackupPath); err != nil {
				return err
			}
		case "install_hermes_hook":
			if err := installHermesHooks(op.Path, op.BackupPath, op.Reason); err != nil {
				return err
			}
		case "uninstall_hermes_hook":
			if err := uninstallHermesHooks(op.Path, op.BackupPath); err != nil {
				return err
			}
		case "install_pi_extension":
			if err := installPiExtension(op.Path, op.BackupPath, op.AdapterURL); err != nil {
				return err
			}
		case "uninstall_pi_extension":
			if err := uninstallPiExtension(op.Path, op.BackupPath); err != nil {
				return err
			}
		case "install_omp_extension":
			if err := installOMPExtension(op.Path, op.BackupPath, op.AdapterURL); err != nil {
				return err
			}
		case "uninstall_omp_extension":
			if err := uninstallPiExtension(op.Path, op.BackupPath); err != nil {
				return err
			}
		case "install_opencode_plugin":
			if err := installOpenCodePlugin(op.Path, op.BackupPath, op.AdapterURL); err != nil {
				return err
			}
		case "uninstall_opencode_plugin":
			if err := uninstallOpenCodePlugin(op.Path, op.BackupPath); err != nil {
				return err
			}
		case "skip":
			continue
		case "install", "uninstall":
			if err := applyPlaceholderOp(op, uninstall); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported install action %q", op.Action)
		}
	}
	return nil
}

func applyPlaceholderOp(op installOperation, uninstall bool) error {
	p := op.Path
	if uninstall {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	content := []byte(fmt.Sprintf("# Managed by Hitch\nharness=%s\n", op.Harness))
	existing, err := os.ReadFile(p)
	if err == nil {
		if string(existing) == string(content) {
			return nil
		}
		backup := op.BackupPath
		if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(backup, existing, 0o600); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, content, 0o644)
}

func codexHookInstalled(path string) bool {
	doc, _, err := readCodexHooks(path)
	if err != nil {
		return false
	}
	for _, event := range codexLifecycleEvents {
		if !codexEventHasManagedHook(doc, event) {
			return false
		}
	}
	return true
}

func installCodexHook(path, backup, binaryPath string) error {
	doc, existed, err := readCodexHooks(path)
	if err != nil {
		return err
	}
	changed := false
	for _, event := range codexLifecycleEvents {
		command := codexAdapterCommand(binaryPath, event)
		if upsertCodexHook(doc, event, command) {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if existed {
		if err := backupFile(path, backup); err != nil {
			return err
		}
	}
	return writeJSONFile(path, doc)
}

func uninstallCodexHook(path, backup string) error {
	doc, existed, err := readCodexHooks(path)
	if err != nil {
		return err
	}
	if !existed {
		return nil
	}
	changed := false
	for _, event := range codexLifecycleEvents {
		if removeCodexHook(doc, event) {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if err := backupFile(path, backup); err != nil {
		return err
	}
	return writeJSONFile(path, doc)
}

func hermesHookInstalled(path string) bool {
	doc, _, err := readHermesConfig(path)
	if err != nil {
		return false
	}
	for _, event := range hermesHookEvents {
		if !hermesEventHasManagedHook(doc, event) {
			return false
		}
	}
	return true
}

func installHermesHooks(path, backup, binaryPath string) error {
	doc, existed, err := readHermesConfig(path)
	if err != nil {
		return err
	}
	changed := false
	for _, event := range hermesHookEvents {
		command := hermesAdapterCommand(binaryPath, event)
		if upsertHermesHook(doc, event, command) {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if existed {
		if err := backupFile(path, backup); err != nil {
			return err
		}
	}
	return writeYAMLFile(path, doc)
}

func uninstallHermesHooks(path, backup string) error {
	doc, existed, err := readHermesConfig(path)
	if err != nil {
		return err
	}
	if !existed {
		return nil
	}
	changed := false
	for _, event := range hermesHookEvents {
		if removeHermesHook(doc, event) {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if err := backupFile(path, backup); err != nil {
		return err
	}
	return writeYAMLFile(path, doc)
}

func piExtensionInstalled(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	content := string(b)
	return strings.Contains(content, piManagedExtensionMarker) && strings.Contains(content, "dispatchToHitch")
}

func opencodePluginInstalled(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	content := string(b)
	return strings.Contains(content, piManagedExtensionMarker) && strings.Contains(content, "HitchPlugin") && strings.Contains(content, "dispatchToHitch")
}

func installPiExtension(path, backup, apiURL string) error {
	content, err := piExtensionContent(apiURL)
	if err != nil {
		return err
	}
	return installExtensionContent(path, backup, content)
}

func installOMPExtension(path, backup, apiURL string) error {
	content, err := ompExtensionContent(apiURL)
	if err != nil {
		return err
	}
	return installExtensionContent(path, backup, content)
}

func installOpenCodePlugin(path, backup, apiURL string) error {
	content, err := opencodePluginContent(apiURL)
	if err != nil {
		return err
	}
	return installExtensionContent(path, backup, content)
}

func installExtensionContent(path, backup string, content []byte) error {
	existing, err := os.ReadFile(path)
	if err == nil {
		if string(existing) == string(content) {
			return nil
		}
		if err := backupFile(path, backup); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func uninstallPiExtension(path, backup string) error {
	existing, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !strings.Contains(string(existing), piManagedExtensionMarker) {
		return nil
	}
	if err := backupFile(path, backup); err != nil {
		return err
	}
	return os.Remove(path)
}

func uninstallOpenCodePlugin(path, backup string) error {
	existing, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	content := string(existing)
	if !strings.Contains(content, piManagedExtensionMarker) || !strings.Contains(content, "HitchPlugin") || !strings.Contains(content, "dispatchToHitch") {
		return nil
	}
	if err := backupFile(path, backup); err != nil {
		return err
	}
	return os.Remove(path)
}

func piExtensionContent(apiURL string) ([]byte, error) {
	return extensionContent("pi", "hitch-pi-extension", piExtensionEvents, piExtensionSyncEvents, apiURL)
}

func ompExtensionContent(apiURL string) ([]byte, error) {
	return extensionContent("omp", "hitch-omp-extension", ompExtensionEvents, ompExtensionSyncEvents, apiURL)
}

func opencodePluginContent(apiURL string) ([]byte, error) {
	return openCodePluginContent("opencode", "hitch-opencode-plugin", opencodeHookEvents, apiURL)
}

func extensionContent(harnessName, clientVersion string, sourceEvents, syncEvents []string, apiURL string) ([]byte, error) {
	urlLiteral, err := jsonStringLiteral(apiURL)
	if err != nil {
		return nil, err
	}
	eventsLiteral, err := jsonArrayLiteral(sourceEvents)
	if err != nil {
		return nil, err
	}
	syncEventsLiteral, err := jsonArrayLiteral(syncEvents)
	if err != nil {
		return nil, err
	}
	harnessLiteral, err := jsonStringLiteral(harnessName)
	if err != nil {
		return nil, err
	}
	versionLiteral, err := jsonStringLiteral(clientVersion)
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	b.Grow(2700 + len(piManagedExtensionMarker) + len(urlLiteral) + len(eventsLiteral) + len(syncEventsLiteral) + len(harnessLiteral) + len(versionLiteral))
	b.WriteString("// ")
	b.WriteString(piManagedExtensionMarker)
	b.WriteString(`.
// Generated by hitch-client install. Do not edit by hand.

const HITCH_PINNED_API_URL = `)
	b.WriteString(urlLiteral)
	b.WriteString(`;
const HITCH_DEFAULT_API_URL = "http://127.0.0.1:8799";
const HITCH_EVENTS = `)
	b.WriteString(eventsLiteral)
	b.WriteString(`;
const HITCH_SYNC_EVENTS = `)
	b.WriteString(syncEventsLiteral)
	b.WriteString(`;
const HITCH_SYNC_EVENT_SET = new Set(HITCH_SYNC_EVENTS);

function envString(name) {
  const processLike = typeof globalThis === "object" ? Object(globalThis).process : undefined;
  const env = processLike && typeof processLike === "object" ? Object(processLike).env : undefined;
  const value = env && typeof env === "object" ? Object(env)[name] : undefined;
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function hitchAPIURL() {
  return firstString(HITCH_PINNED_API_URL, envString("HITCH_URL"), HITCH_DEFAULT_API_URL);
}

function setPath(target, path, value) {
  if (!target || !Array.isArray(path) || path.length === 0) return;
  let cursor = target;
  for (const key of path.slice(0, -1)) {
    if (!cursor[key] || typeof cursor[key] !== "object") cursor[key] = {};
    cursor = cursor[key];
  }
  cursor[path[path.length - 1]] = value;
}

function applyAdapterResponse(adapterResponse, event) {
  if (!adapterResponse || adapterResponse.adapter_action === "noop") return undefined;
  if (adapterResponse.adapter_action === "return") return adapterResponse.return_value;
  if (adapterResponse.adapter_action === "mutate_and_return") {
    for (const mutation of adapterResponse.mutations || []) {
      setPath(event, mutation.path, mutation.value);
    }
    if (Object.prototype.hasOwnProperty.call(adapterResponse, "return_value")) {
      return adapterResponse.return_value;
    }
  }
  return undefined;
}

function firstString(...values) {
  for (const value of values) {
    if (typeof value === "string" && value.length > 0) return value;
    if (typeof value === "number" && Number.isFinite(value)) return String(value);
  }
  return undefined;
}

function sessionFileFrom(ctx) {
  const manager = ctx?.sessionManager;
  if (!manager) return undefined;
  try {
    if (typeof manager.getSessionFile === "function") return manager.getSessionFile();
  } catch {}
  return firstString(manager.sessionFile, manager.file, manager.path);
}

function sessionIDFrom(event, ctx, transcriptPath) {
  const direct = firstString(event?.session_id, event?.sessionId, ctx?.session_id, ctx?.sessionId);
  if (direct) return direct;
  const manager = ctx?.sessionManager;
  try {
    const id = manager && typeof manager.getSessionId === "function" ? manager.getSessionId() : undefined;
    if (id) return String(id);
  } catch {}
  const fromManager = firstString(manager?.session_id, manager?.sessionId, manager?.id);
  if (fromManager) return fromManager;
  if (typeof transcriptPath === "string" && transcriptPath.length > 0) {
    const name = transcriptPath.split(/[\\/]/).pop() || transcriptPath;
    return name.replace(/\.(jsonl|json)$/i, "");
  }
  return undefined;
}

function modelString(model) {
  if (typeof model === "string") return model;
  if (!model || typeof model !== "object") return undefined;
  const provider = firstString(model.provider, model.providerId, model.vendor);
  const id = firstString(model.id, model.model, model.name);
  if (provider && id) return provider + "/" + id;
  return id || provider;
}

function collectMetadata(event, ctx) {
  const transcriptPath = firstString(event?.transcript_path, event?.transcriptPath, sessionFileFrom(ctx));
  return {
    session_id: sessionIDFrom(event, ctx, transcriptPath),
    turn_id: firstString(event?.turn_id, event?.turnId, event?.turnIndex),
    cwd: firstString(event?.cwd, ctx?.cwd),
    model: firstString(event?.model) || modelString(event?.model) || modelString(ctx?.model),
    transcript_path: transcriptPath
  };
}


async function dispatchToHitch(sourceEventType, event, ctx) {
  let sourcePayload;
  try {
    sourcePayload = {
      event: JSON.parse(JSON.stringify(event ?? {})),
      metadata: collectMetadata(event, ctx)
    };
  } catch {
    return undefined;
  }

  const endpoint = HITCH_SYNC_EVENT_SET.has(sourceEventType) ? "/v1/dispatch-sync" : "/v1/events";
  try {
    const response = await fetch(hitchAPIURL() + endpoint, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        harness: `)
	b.WriteString(harnessLiteral)
	b.WriteString(`,
        source_event_type: sourceEventType,
        source_payload: sourcePayload,
        hitch_client_version: `)
	b.WriteString(versionLiteral)
	b.WriteString(`
      })
    });
    if (!response.ok) return undefined;
    if (!HITCH_SYNC_EVENT_SET.has(sourceEventType)) return undefined;
    const payload = await response.json();
    return applyAdapterResponse(payload.native_response, event);
  } catch {
    return undefined;
  }
}

export default function(pi) {
  for (const sourceEventType of HITCH_EVENTS) {
    pi.on(sourceEventType, async (event, ctx) => dispatchToHitch(sourceEventType, event, ctx));
  }
}
`)
	return []byte(b.String()), nil
}

func openCodePluginContent(harnessName, clientVersion string, sourceEvents []string, apiURL string) ([]byte, error) {
	urlLiteral, err := jsonStringLiteral(apiURL)
	if err != nil {
		return nil, err
	}
	eventsLiteral, err := jsonArrayLiteral(sourceEvents)
	if err != nil {
		return nil, err
	}
	harnessLiteral, err := jsonStringLiteral(harnessName)
	if err != nil {
		return nil, err
	}
	versionLiteral, err := jsonStringLiteral(clientVersion)
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	b.Grow(5000 + len(piManagedExtensionMarker) + len(urlLiteral) + len(eventsLiteral) + len(harnessLiteral) + len(versionLiteral))
	b.WriteString("// ")
	b.WriteString(piManagedExtensionMarker)
	b.WriteString(`.
// Generated by hitch-client install. Do not edit by hand.

const HITCH_PINNED_API_URL = `)
	b.WriteString(urlLiteral)
	b.WriteString(`;
const HITCH_DEFAULT_API_URL = "http://127.0.0.1:8799";
const HITCH_EVENTS = `)
	b.WriteString(eventsLiteral)
	b.WriteString(`;

function envString(name) {
  const processLike = typeof globalThis === "object" ? Object(globalThis).process : undefined;
  const env = processLike && typeof processLike === "object" ? Object(processLike).env : undefined;
  const value = env && typeof env === "object" ? Object(env)[name] : undefined;
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function firstString(...values) {
  for (const value of values) {
    if (typeof value === "string" && value.length > 0) return value;
    if (typeof value === "number" && Number.isFinite(value)) return String(value);
  }
  return undefined;
}

function hitchAPIURL() {
  return firstString(HITCH_PINNED_API_URL, envString("HITCH_URL"), HITCH_DEFAULT_API_URL);
}

function safeClone(value) {
  if (value === undefined) return {};
  try {
    return JSON.parse(JSON.stringify(value));
  } catch {
    return {};
  }
}

function modelString(model) {
  if (typeof model === "string") return model;
  if (!model || typeof model !== "object") return undefined;
  const provider = firstString(model.providerID, model.providerId, model.provider, model.vendor);
  const id = firstString(model.modelID, model.modelId, model.id, model.model, model.name);
  if (provider && id) return provider + "/" + id;
  return id || provider;
}

function eventSessionID(event) {
  return firstString(event?.sessionID, event?.session_id, event?.properties?.sessionID, event?.properties?.session_id);
}

function collectMetadata(input, event, ctx) {
  const sessionID = firstString(input?.sessionID, input?.session_id, eventSessionID(event), ctx?.project?.id);
  return {
    session_id: sessionID,
    turn_id: firstString(input?.messageID, input?.message_id, event?.properties?.messageID, event?.properties?.message_id),
    cwd: firstString(input?.cwd, input?.path?.cwd, ctx?.directory, ctx?.worktree),
    model: modelString(input?.model) || modelString(event?.properties?.info?.model),
    transcript_path: firstString(input?.transcriptPath, input?.transcript_path)
  };
}

function setPath(target, path, value) {
  if (!target || !Array.isArray(path) || path.length === 0) return;
  let cursor = target;
  for (const key of path.slice(0, -1)) {
    if (!cursor[key] || typeof cursor[key] !== "object") cursor[key] = {};
    cursor = cursor[key];
  }
  cursor[path[path.length - 1]] = value;
}

async function applyAdapterResponse(adapterResponse, output, input, ctx) {
  if (!adapterResponse || adapterResponse.adapter_action === "noop") return;
  if (adapterResponse.adapter_action === "throw") {
    const err = new Error(adapterResponse.message || "blocked by Hitch");
    err.hitchAdapterThrow = true;
    throw err;
  }
  if (adapterResponse.adapter_action === "set") {
    if (Array.isArray(adapterResponse.path) && adapterResponse.path.length > 0) {
      setPath(output, adapterResponse.path, adapterResponse.value);
    } else if (adapterResponse.value && typeof adapterResponse.value === "object") {
      Object.assign(output, adapterResponse.value);
    }
    return;
  }
  if (adapterResponse.adapter_action === "append") {
    const path = Array.isArray(adapterResponse.path) ? adapterResponse.path : [];
    if (path.length === 0) return;
    let cursor = output;
    for (const key of path.slice(0, -1)) {
      if (!cursor[key] || typeof cursor[key] !== "object") cursor[key] = {};
      cursor = cursor[key];
    }
    const key = path[path.length - 1];
    if (!Array.isArray(cursor[key])) cursor[key] = [];
    cursor[key].push(adapterResponse.value);
    return;
  }
  if (adapterResponse.adapter_action === "inject_context") {
    const sessionID = firstString(input?.sessionID, input?.session_id);
    if (!sessionID || typeof adapterResponse.value !== "string" || adapterResponse.value.length === 0) return;
    await ctx.client.session.prompt({
      path: { id: sessionID },
      body: {
        noReply: true,
        parts: [{ type: "text", text: adapterResponse.value }]
      }
    });
  }
}

async function postToHitch(endpoint, sourceEventType, input, output, ctx) {
  const event = { input: safeClone(input), output: safeClone(output) };
  const sourcePayload = {
    event,
    metadata: collectMetadata(input, event, ctx)
  };
  try {
    const response = await fetch(hitchAPIURL() + endpoint, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        harness: `)
	b.WriteString(harnessLiteral)
	b.WriteString(`,
        source_event_type: sourceEventType,
        source_payload: sourcePayload,
        hitch_client_version: `)
	b.WriteString(versionLiteral)
	b.WriteString(`
      })
    });
    if (!response.ok || endpoint !== "/v1/dispatch-sync") return;
    const payload = await response.json();
    await applyAdapterResponse(payload.native_response, output, input, ctx);
  } catch (err) {
    if (err && err.hitchAdapterThrow) throw err;
  }
}

async function dispatchToHitch(sourceEventType, input, output, ctx) {
  await postToHitch("/v1/dispatch-sync", sourceEventType, input, output, ctx);
}

async function observeWithHitch(sourceEventType, event, ctx) {
  const sourcePayload = {
    event: safeClone(event),
    metadata: collectMetadata(event, event, ctx)
  };
  try {
    await fetch(hitchAPIURL() + "/v1/events", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        harness: `)
	b.WriteString(harnessLiteral)
	b.WriteString(`,
        source_event_type: sourceEventType,
        source_payload: sourcePayload,
        hitch_client_version: `)
	b.WriteString(versionLiteral)
	b.WriteString(`
      })
    });
  } catch {}
}


export const HitchPlugin = async (ctx) => {
  return {
    event: async ({ event }) => {
      if (event && HITCH_EVENTS.includes(event.type)) {
        await observeWithHitch(event.type, event, ctx);
      }
    },
    "chat.message": async (input, output) => dispatchToHitch("chat.message", input, output, ctx),
    "chat.params": async (input, output) => dispatchToHitch("chat.params", input, output, ctx),
    "chat.headers": async (input, output) => dispatchToHitch("chat.headers", input, output, ctx),
    "command.execute.before": async (input, output) => dispatchToHitch("command.execute.before", input, output, ctx),
    "permission.ask": async (input, output) => dispatchToHitch("permission.ask", input, output, ctx),
    "tool.execute.before": async (input, output) => dispatchToHitch("tool.execute.before", input, output, ctx),
    "tool.execute.after": async (input, output) => dispatchToHitch("tool.execute.after", input, output, ctx),
    "tool.definition": async (input, output) => dispatchToHitch("tool.definition", input, output, ctx),
    "shell.env": async (input, output) => dispatchToHitch("shell.env", input, output, ctx),
    "experimental.session.compacting": async (input, output) => dispatchToHitch("experimental.session.compacting", input, output, ctx),
    "experimental.compaction.autocontinue": async (input, output) => dispatchToHitch("experimental.compaction.autocontinue", input, output, ctx),
    "experimental.text.complete": async (input, output) => dispatchToHitch("experimental.text.complete", input, output, ctx),
  };
};
`)
	return []byte(b.String()), nil
}

func jsonArrayLiteral(values []string) (string, error) {
	b, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func jsonStringLiteral(value string) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func readHermesConfig(path string) (*yaml.Node, bool, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return emptyYAMLDocument(), false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return emptyYAMLDocument(), true, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, true, fmt.Errorf("parse %s: %w", path, err)
	}
	if doc.Kind == 0 {
		return emptyYAMLDocument(), true, nil
	}
	ensureDocumentMapping(&doc)
	return &doc, true, nil
}

func emptyYAMLDocument() *yaml.Node {
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
}

func ensureDocumentMapping(doc *yaml.Node) *yaml.Node {
	if doc.Kind != yaml.DocumentNode {
		*doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode}}
	}
	return doc.Content[0]
}

func hermesAdapterCommand(commandBase, event string) string {
	return commandBase + " -harness hermes -event " + event + " -sync"
}

func upsertHermesHook(doc *yaml.Node, event, command string) bool {
	root := ensureDocumentMapping(doc)
	hooks := ensureYAMLMapping(root, "hooks")
	entries := ensureYAMLSequence(hooks, event)
	newHook := hermesHookNode(command)
	needle := hermesManagedHookNeedle(event)
	for i, entry := range entries.Content {
		if yamlHookCommandContains(entry, needle) {
			if yamlScalarValue(entry, "command") == command && yamlScalarValue(entry, "timeout") == "30" {
				return false
			}
			entries.Content[i] = newHook
			return true
		}
	}
	entries.Content = append(entries.Content, newHook)
	return true
}

func removeHermesHook(doc *yaml.Node, event string) bool {
	root := ensureDocumentMapping(doc)
	hooks := yamlMappingValue(root, "hooks")
	if hooks == nil || hooks.Kind != yaml.MappingNode {
		return false
	}
	entries := yamlMappingValue(hooks, event)
	if entries == nil || entries.Kind != yaml.SequenceNode {
		return false
	}
	changed := false
	kept := entries.Content[:0]
	needle := hermesManagedHookNeedle(event)
	for _, entry := range entries.Content {
		if yamlHookCommandContains(entry, needle) {
			changed = true
			continue
		}
		kept = append(kept, entry)
	}
	if !changed {
		return false
	}
	if len(kept) == 0 {
		deleteYAMLMappingKey(hooks, event)
		return true
	}
	entries.Content = kept
	return true
}

func hermesEventHasManagedHook(doc *yaml.Node, event string) bool {
	root := ensureDocumentMapping(doc)
	hooks := yamlMappingValue(root, "hooks")
	if hooks == nil || hooks.Kind != yaml.MappingNode {
		return false
	}
	entries := yamlMappingValue(hooks, event)
	if entries == nil || entries.Kind != yaml.SequenceNode {
		return false
	}
	needle := hermesManagedHookNeedle(event)
	for _, entry := range entries.Content {
		if yamlHookCommandContains(entry, needle) {
			return true
		}
	}
	return false
}

func hermesManagedHookNeedle(event string) string {
	return "-harness hermes -event " + event
}

func hermesHookNode(command string) *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		yamlScalar("command"), yamlScalar(command),
		yamlScalar("timeout"), yamlIntScalar("30"),
	}}
}

func ensureYAMLMapping(parent *yaml.Node, key string) *yaml.Node {
	existing := yamlMappingValue(parent, key)
	if existing != nil && existing.Kind == yaml.MappingNode {
		return existing
	}
	node := &yaml.Node{Kind: yaml.MappingNode}
	setYAMLMappingValue(parent, key, node)
	return node
}

func ensureYAMLSequence(parent *yaml.Node, key string) *yaml.Node {
	existing := yamlMappingValue(parent, key)
	if existing != nil && existing.Kind == yaml.SequenceNode {
		return existing
	}
	node := &yaml.Node{Kind: yaml.SequenceNode}
	setYAMLMappingValue(parent, key, node)
	return node
}

func yamlMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func setYAMLMappingValue(mapping *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content, yamlScalar(key), value)
}

func deleteYAMLMappingKey(mapping *yaml.Node, key string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return
		}
	}
}

func yamlHookCommandContains(entry *yaml.Node, needle string) bool {
	command := yamlScalarValue(entry, "command")
	return strings.Contains(command, needle)
}
func yamlIntScalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: value}
}

func yamlScalarValue(mapping *yaml.Node, key string) string {
	value := yamlMappingValue(mapping, key)
	if value == nil {
		return ""
	}
	return value.Value
}

func yamlScalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func writeYAMLFile(path string, doc *yaml.Node) error {
	b, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func readCodexHooks(path string) (map[string]any, bool, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]any{"hooks": map[string]any{}}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, true, fmt.Errorf("parse %s: %w", path, err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	return doc, true, nil
}

func codexAdapterCommand(commandBase, event string) string {
	return commandBase + " -harness codex -event " + event + " -sync"
}

func upsertCodexHook(doc map[string]any, event, command string) bool {
	groups := codexEventGroups(doc, event)
	newHook := map[string]any{"type": "command", "command": command, "timeout": float64(30), "statusMessage": "Dispatching to Hitch"}
	needle := codexManagedHookNeedle(event)
	for _, groupValue := range groups {
		group, ok := groupValue.(map[string]any)
		if !ok {
			continue
		}
		hooks, _ := group["hooks"].([]any)
		for i, hookValue := range hooks {
			hook, ok := hookValue.(map[string]any)
			if !ok {
				continue
			}
			if hookCommandContains(hook, needle) {
				if hook["command"] == command {
					return false
				}
				hooks[i] = newHook
				group["hooks"] = hooks
				return true
			}
		}
	}
	group := map[string]any{"matcher": "*", "hooks": []any{newHook}}
	setCodexEventGroups(doc, event, append(groups, group))
	return true
}

func removeCodexHook(doc map[string]any, event string) bool {
	groups := codexEventGroups(doc, event)
	changed := false
	keptGroups := make([]any, 0, len(groups))
	needle := codexManagedHookNeedle(event)
	for _, groupValue := range groups {
		group, ok := groupValue.(map[string]any)
		if !ok {
			keptGroups = append(keptGroups, groupValue)
			continue
		}
		hooks, _ := group["hooks"].([]any)
		keptHooks := make([]any, 0, len(hooks))
		for _, hookValue := range hooks {
			hook, ok := hookValue.(map[string]any)
			if ok && hookCommandContains(hook, needle) {
				changed = true
				continue
			}
			keptHooks = append(keptHooks, hookValue)
		}
		if len(keptHooks) == 0 {
			changed = true
			continue
		}
		group["hooks"] = keptHooks
		keptGroups = append(keptGroups, group)
	}
	if changed {
		setCodexEventGroups(doc, event, keptGroups)
	}
	return changed
}

func codexEventHasManagedHook(doc map[string]any, event string) bool {
	needle := codexManagedHookNeedle(event)
	for _, groupValue := range codexEventGroups(doc, event) {
		group, ok := groupValue.(map[string]any)
		if !ok {
			continue
		}
		hooks, _ := group["hooks"].([]any)
		for _, hookValue := range hooks {
			hook, ok := hookValue.(map[string]any)
			if ok && hookCommandContains(hook, needle) {
				return true
			}
		}
	}
	return false
}

func codexManagedHookNeedle(event string) string {
	return "-harness codex -event " + event
}

func codexEventGroups(doc map[string]any, event string) []any {
	hooks, ok := doc["hooks"].(map[string]any)
	if !ok {
		return nil
	}
	groups, _ := hooks[event].([]any)
	return groups
}

func setCodexEventGroups(doc map[string]any, event string, groups []any) {
	hooks, ok := doc["hooks"].(map[string]any)
	if !ok {
		hooks = map[string]any{}
		doc["hooks"] = hooks
	}
	if len(groups) == 0 {
		delete(hooks, event)
		return
	}
	hooks[event] = groups
}

func hookCommandContains(hook map[string]any, needle string) bool {
	command, ok := hook["command"].(string)
	return ok && strings.Contains(command, needle)
}

func backupFile(path, backup string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
		return err
	}
	return os.WriteFile(backup, b, 0o600)
}

func writeJSONFile(path string, doc map[string]any) error {
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

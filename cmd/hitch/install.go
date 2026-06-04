package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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
}

func knownHarnessSpecs() []harnessSpec {
	return []harnessSpec{
		{Name: "codex", Title: "Codex", Command: "codex", ConfigPath: "~/.codex/hooks.json", Supported: true},
		{Name: "hermes", Title: "Hermes", Command: "hermes", ConfigPath: "~/.hermes/config.yaml", Supported: true},
		{Name: "pi", Title: "Pi", Command: "pi", ConfigPath: "~/.pi/agent", Supported: false, Reason: "Pi extension hook installation is not implemented yet"},
		{Name: "omp", Title: "OMP", Command: "omp", ConfigPath: "~/.omp/agent", Supported: false, Reason: "OMP extension hook installation is not implemented yet"},
	}
}

func install(args []string, uninstall bool) {
	fs := flagSet("install")
	only := fs.String("only", "codex,hermes,pi,omp", "comma-separated harness list")
	dryRun := fs.Bool("dry-run", false, "show changes without writing")
	yes := fs.Bool("yes", false, "confirm filesystem changes")
	jsonOut := fs.Bool("json", false, "emit JSON")
	all := fs.Bool("all", false, "select all harnesses")
	_ = fs.Parse(args)

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
			fatal(fmt.Errorf("--yes is required unless --dry-run is used"))
		}
		if !confirmInstall(detections, selected, uninstall) {
			writeCLI(*jsonOut, map[string]interface{}{"dry_run": *dryRun, "uninstall": uninstall, "harnesses": detections, "operations": []installOperation{}})
			return
		}
	}

	ops, err := plannedOps(selected, uninstall)
	if err != nil {
		fatal(err)
	}
	if !*dryRun {
		if err := applyOps(ops, uninstall); err != nil {
			fatal(err)
		}
	}
	writeCLI(*jsonOut, map[string]interface{}{"dry_run": *dryRun, "uninstall": uninstall, "harnesses": detections, "operations": ops})
}

func flagSet(name string) *flag.FlagSet {
	return flag.NewFlagSet(name, flag.ExitOnError)
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

func plannedOps(harnesses []string, uninstall bool) ([]installOperation, error) {
	ops := []installOperation{}
	if !uninstall {
		ops = append(ops, installOperation{Harness: "hitch", Action: "seed_config", Path: config.ExpandHome(config.DefaultPath)})
	}
	known := map[string]harnessSpec{}
	for _, spec := range knownHarnessSpecs() {
		known[spec.Name] = spec
	}
	binaryPath, err := installedBinaryPath()
	if err != nil && !uninstall {
		return nil, err
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
			ops = append(ops, installOperation{Harness: h, Action: action, Path: detection.ConfigPath, BackupPath: timestampedBackupPath(h, filepath.Base(detection.ConfigPath)), Status: "planned", Reason: binaryPath})
		case "hermes":
			action := "install_hermes_hook"
			if uninstall {
				action = "uninstall_hermes_hook"
			}
			ops = append(ops, installOperation{Harness: h, Action: action, Path: detection.ConfigPath, BackupPath: timestampedBackupPath(h, filepath.Base(detection.ConfigPath)), Status: "planned", Reason: binaryPath})
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
	if override := os.Getenv("HITCH_BINARY_PATH"); override != "" {
		return filepath.Abs(override)
	}
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(p)
}

func applyOps(ops []installOperation, uninstall bool) error {
	for _, op := range ops {
		switch op.Action {
		case "seed_config":
			if uninstall {
				continue
			}
			if err := seedConfig(op.Path); err != nil {
				return err
			}
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

func seedConfig(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(config.DefaultConfigTOML), 0o644)
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

func hermesAdapterCommand(binaryPath, event string) string {
	return shellQuote(binaryPath) + " adapter -harness hermes -event " + event + " -sync"
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
	return "adapter -harness hermes -event " + event
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

func codexAdapterCommand(binaryPath, event string) string {
	return shellQuote(binaryPath) + " adapter -harness codex -event " + event + " -sync"
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
	return "adapter -harness codex -event " + event
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

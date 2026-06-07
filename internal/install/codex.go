package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

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

package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

func claudecodeHookInstalled(path string) bool {
	doc, _, err := readClaudeCodeSettings(path)
	if err != nil {
		return false
	}
	for _, event := range claudecodeHookEvents {
		if !claudecodeEventHasManagedHook(doc, event) {
			return false
		}
	}
	return true
}

func installClaudeCodeHook(path, backup, binaryPath string) error {
	doc, existed, err := readClaudeCodeSettings(path)
	if err != nil {
		return err
	}
	changed := false
	for _, event := range claudecodeHookEvents {
		command := claudecodeAdapterCommand(binaryPath, event)
		if upsertClaudeCodeHook(doc, event, command) {
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

func uninstallClaudeCodeHook(path, backup string) error {
	doc, existed, err := readClaudeCodeSettings(path)
	if err != nil {
		return err
	}
	if !existed {
		return nil
	}
	changed := false
	for _, event := range claudecodeHookEvents {
		if removeClaudeCodeHook(doc, event) {
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

func readClaudeCodeSettings(path string) (map[string]any, bool, error) {
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

func claudecodeAdapterCommand(commandBase, event string) string {
	return commandBase + " -harness claudecode -event " + event + " -sync"
}

func upsertClaudeCodeHook(doc map[string]any, event, command string) bool {
	groups := claudecodeEventGroups(doc, event)
	newHook := map[string]any{"type": "command", "command": command, "timeout": float64(30), "statusMessage": "Dispatching to Hitch"}
	needle := claudecodeManagedHookNeedle(event)
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
	setClaudeCodeEventGroups(doc, event, append(groups, group))
	return true
}

func removeClaudeCodeHook(doc map[string]any, event string) bool {
	groups := claudecodeEventGroups(doc, event)
	changed := false
	keptGroups := make([]any, 0, len(groups))
	needle := claudecodeManagedHookNeedle(event)
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
		setClaudeCodeEventGroups(doc, event, keptGroups)
	}
	return changed
}

func claudecodeEventHasManagedHook(doc map[string]any, event string) bool {
	needle := claudecodeManagedHookNeedle(event)
	for _, groupValue := range claudecodeEventGroups(doc, event) {
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

func claudecodeManagedHookNeedle(event string) string {
	return "-harness claudecode -event " + event
}

func claudecodeEventGroups(doc map[string]any, event string) []any {
	hooks, ok := doc["hooks"].(map[string]any)
	if !ok {
		return nil
	}
	groups, _ := hooks[event].([]any)
	return groups
}

func setClaudeCodeEventGroups(doc map[string]any, event string, groups []any) {
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

func claudecodeSettingsPreservesKeys(doc map[string]any) bool {
	for key := range doc {
		if key != "hooks" {
			return true
		}
	}
	return false
}

func isClaudeCodeManagedHook(hook map[string]any) bool {
	command, ok := hook["command"].(string)
	return ok && strings.Contains(command, "-harness claudecode")
}

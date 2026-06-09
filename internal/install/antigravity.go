package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

const antigravityHookKey = "hitch-managed-hook"

func antigravityHookInstalled(path string) bool {
	doc, _, err := readAntigravityHooks(path)
	if err != nil {
		return false
	}
	hookDef, ok := doc[antigravityHookKey].(map[string]any)
	if !ok {
		return false
	}
	for _, event := range antigravityHookEvents {
		if !antigravityEventHasManagedHook(hookDef, event) {
			return false
		}
	}
	return true
}

func installAntigravityHook(path, backup, binaryPath string) error {
	doc, existed, err := readAntigravityHooks(path)
	if err != nil {
		return err
	}
	hookDef, ok := doc[antigravityHookKey].(map[string]any)
	if !ok {
		hookDef = map[string]any{"enabled": true}
		doc[antigravityHookKey] = hookDef
	}

	changed := false
	for _, event := range antigravityHookEvents {
		command := antigravityAdapterCommand(binaryPath, event)
		if upsertAntigravityHook(hookDef, event, command) {
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

func uninstallAntigravityHook(path, backup string) error {
	doc, existed, err := readAntigravityHooks(path)
	if err != nil {
		return err
	}
	if !existed {
		return nil
	}
	if _, ok := doc[antigravityHookKey]; !ok {
		return nil
	}
	delete(doc, antigravityHookKey)
	if err := backupFile(path, backup); err != nil {
		return err
	}
	return writeJSONFile(path, doc)
}

func readAntigravityHooks(path string) (map[string]any, bool, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]any{}, false, nil
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

func antigravityAdapterCommand(commandBase, event string) string {
	return commandBase + " -harness antigravity -event " + event + " -sync"
}

func upsertAntigravityHook(hookDef map[string]any, event, command string) bool {
	newHook := map[string]any{"type": "command", "command": command, "timeout": float64(30)}
	
	switch event {
	case "PreToolUse", "PostToolUse":
		groups, _ := hookDef[event].([]any)
		needle := antigravityManagedHookNeedle(event)
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
		hookDef[event] = append(groups, group)
		return true

	case "PreInvocation", "PostInvocation", "Stop":
		hooks, _ := hookDef[event].([]any)
		needle := antigravityManagedHookNeedle(event)
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
				hookDef[event] = hooks
				return true
			}
		}
		hookDef[event] = append(hooks, newHook)
		return true
	}
	return false
}

func antigravityEventHasManagedHook(hookDef map[string]any, event string) bool {
	needle := antigravityManagedHookNeedle(event)
	
	switch event {
	case "PreToolUse", "PostToolUse":
		groups, _ := hookDef[event].([]any)
		for _, groupValue := range groups {
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
		
	case "PreInvocation", "PostInvocation", "Stop":
		hooks, _ := hookDef[event].([]any)
		for _, hookValue := range hooks {
			hook, ok := hookValue.(map[string]any)
			if ok && hookCommandContains(hook, needle) {
				return true
			}
		}
		return false
	}
	return false
}

func antigravityManagedHookNeedle(event string) string {
	return "-harness antigravity -event " + event
}

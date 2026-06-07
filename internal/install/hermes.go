package install

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

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

func hermesAdapterCommand(commandBase, event string) string {
	if _, ok := hermesHookSyncEvents[event]; ok {
		return commandBase + " -harness hermes -event " + event + " -sync"
	}
	return commandBase + " -harness hermes -event " + event
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

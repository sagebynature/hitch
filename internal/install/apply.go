package install

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

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
		case "install_antigravity_hook":
			if err := installAntigravityHook(op.Path, op.BackupPath, op.Reason); err != nil {
				return err
			}
		case "uninstall_antigravity_hook":
			if err := uninstallAntigravityHook(op.Path, op.BackupPath); err != nil {
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

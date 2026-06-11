package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sagebynature/hitch/internal/config"
)

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
		case "antigravity":
			action := "install_antigravity_hook"
			if uninstall {
				action = "uninstall_antigravity_hook"
			}
			ops = append(ops, installOperation{Harness: h, Action: action, Path: detection.ConfigPath, BackupPath: timestampedBackupPath(h, filepath.Base(detection.ConfigPath)), Status: "planned", Reason: adapterCommandBase(binaryPath, apiURL)})
		case "claudecode":
			action := "install_claudecode_hook"
			if uninstall {
				action = "uninstall_claudecode_hook"
			}
			ops = append(ops, installOperation{Harness: h, Action: action, Path: detection.ConfigPath, BackupPath: timestampedBackupPath(h, filepath.Base(detection.ConfigPath)), Status: "planned", Reason: adapterCommandBase(binaryPath, apiURL)})
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

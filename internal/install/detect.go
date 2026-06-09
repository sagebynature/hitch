package install

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sagebynature/hitch/internal/config"
)

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
	case "antigravity":
		d.Installed = antigravityHookInstalled(d.ConfigPath)
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

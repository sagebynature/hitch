package install

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

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

package main

import (
	"fmt"
	"io"
)

func isHelp(args []string) bool {
	return len(args) == 1 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help")
}

func printHitchHelp(w io.Writer, topics ...string) {
	if len(topics) > 0 {
		switch topics[0] {
		case "serve":
			printServeHelp(w)
		case "handler":
			printHandlerHelp(w)
		case "status":
			printStatusHelp(w)
		case "doctor":
			printDoctorHelp(w)
		case "config":
			if len(topics) > 1 && topics[1] == "init" {
				printConfigInitHelp(w)
			} else {
				printConfigHelp(w)
			}
		case "inspect-event":
			printInspectEventHelp(w)
		case "replay":
			printReplayHelp(w)
		default:
			fmt.Fprintf(w, "Unknown command %q.\n\n", topics[0])
			printRootHelp(w)
		}
		return
	}
	printRootHelp(w)
}

func printRootHelp(w io.Writer) {
	fmt.Fprint(w, `Hitch routes source harness events to local policy/observer handlers.

Usage:
  hitch <command> [options]

Commands:
  serve           Run the local Hitch API server
  handler         Run a bundled handler
  status          Print runtime/config status
  doctor          Run basic diagnostics
  config          Manage Hitch server configuration
  inspect-event   Inspect an audited event
  replay          Replay handlers for an audited event

Global:
  hitch --version
  hitch --help
  hitch help <command>

Examples:
  hitch serve --config config/default.config.toml
  hitch status --json
  hitch inspect-event norm_...

Use hitch-client to dispatch source hook payloads from stdin.
`)
}

func printServeHelp(w io.Writer) {
	fmt.Fprint(w, `Run the local Hitch API server.

Usage:
  hitch serve [options]

Options:
  --config string   config file (default "config/default.config.toml")

Examples:
  hitch serve
  hitch serve --config ~/.config/hitch/config.toml
`)
}

func printHandlerHelp(w io.Writer) {
	fmt.Fprint(w, `Run a bundled Hitch handler.

Usage:
  hitch handler <name>

Bundled handlers:
  noop-observer   Read one Hitch envelope from stdin and return behavior:none

Examples:
  hitch handler noop-observer < envelope.json
`)
}

func printStatusHelp(w io.Writer) {
	fmt.Fprint(w, `Print runtime/config status.

Usage:
  hitch status [options]

Options:
  --json   emit JSON

Examples:
  hitch status
  hitch status --json
`)
}

func printDoctorHelp(w io.Writer) {
	fmt.Fprint(w, `Run basic diagnostics.

Usage:
  hitch doctor [options]

Options:
  --json   emit JSON

Examples:
  hitch doctor
  hitch doctor --json
`)
}

func printConfigHelp(w io.Writer) {
	fmt.Fprint(w, `Manage Hitch server configuration.

Usage:
  hitch config <command> [options]

Commands:
  init   Create the default server config if it is missing

Examples:
  hitch config init
  hitch config init --path ~/.config/hitch/config.toml --json
`)
}

func printConfigInitHelp(w io.Writer) {
	fmt.Fprint(w, `Create the default Hitch server config if it is missing.

Usage:
  hitch config init [options]

Options:
  --path string   config file (default "~/.config/hitch/config.toml")
  --json          emit JSON

Examples:
  hitch config init
  hitch config init --json
`)
}

func printInspectEventHelp(w io.Writer) {
	fmt.Fprint(w, `Inspect an audited event.

Usage:
  hitch inspect-event [options] <normalized-event-id>

Options:
  --config string   config file (default "config/default.config.toml")

Examples:
  hitch inspect-event norm_abc123
  hitch inspect-event --config ~/.config/hitch/config.toml norm_abc123
`)
}

func printReplayHelp(w io.Writer) {
	fmt.Fprint(w, `Replay handlers for an audited event.

Usage:
  hitch replay [options] <normalized-event-id>

Options:
  --config string   config file (default "config/default.config.toml")
  --dry-run         do not create replay records

Examples:
  hitch replay --dry-run norm_abc123
  hitch replay norm_abc123
`)
}

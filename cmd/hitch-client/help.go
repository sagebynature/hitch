package main

import (
	"fmt"
	"io"
)

func isHelp(args []string) bool {
	return len(args) == 1 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help")
}

func printClientHelp(w io.Writer, topics ...string) {
	if len(topics) > 0 {
		switch topics[0] {
		case "install":
			printInstallHelp(w, false)
		case "uninstall":
			printInstallHelp(w, true)
		case "dispatch":
			printDispatchHelp(w)
		default:
			fmt.Fprintf(w, "Unknown command %q.\n\n", topics[0])
			printClientRootHelp(w)
		}
		return
	}
	printClientRootHelp(w)
}

func printClientRootHelp(w io.Writer) {
	fmt.Fprint(w, `hitch-client dispatches source hook payloads to Hitch.

Usage:
  hitch-client [options] < payload.json
  hitch-client install [options]
  hitch-client uninstall [options]

Hook flags:
  -harness string   source harness: codex, hermes, pi, omp, opencode
  -event string     source event type, e.g. PreToolUse
  -sync             wait for native response on stdout
  -url string       Hitch API URL (overrides HITCH_URL)
  -version          print version

Examples:
  hitch-client -harness codex -event PreToolUse -sync < payload.json
  hitch-client install --only codex,hermes,opencode --yes
  hitch-client uninstall --only opencode --yes

Use hitch-client help install for installer options.
`)
}

func printDispatchHelp(w io.Writer) {
	fmt.Fprint(w, `Dispatch one source hook payload to Hitch.

Usage:
  hitch-client [options] < payload.json

Options:
  -harness string   source harness: codex, hermes, pi, omp, opencode
  -event string     source event type, e.g. PreToolUse
  -sync             wait for native response on stdout
  -url string       Hitch API URL (overrides HITCH_URL)
  -version          print version

Examples:
  hitch-client -harness codex -event PreToolUse -sync < payload.json
  hitch-client -harness hermes -event post_tool_call < payload.json
`)
}

func printInstallHelp(w io.Writer, uninstall bool) {
	verb := "Install"
	command := "install"
	example := "hitch-client install --only codex,hermes,opencode --yes"
	if uninstall {
		verb = "Uninstall"
		command = "uninstall"
		example = "hitch-client uninstall --only opencode --yes"
	}
	fmt.Fprintf(w, `%s Hitch-managed harness hooks.

Usage:
  hitch-client %s [options]

Options:
  --only string    comma-separated harness list (default: detected supported harnesses)
  --all            select all known harnesses
  --url string     Hitch API URL pinned into installed hook commands/extensions
  --dry-run        show changes without writing
  --yes            confirm filesystem changes
  --json           emit JSON

Examples:
  %s
  hitch-client %s --dry-run --json
`, verb, command, example, command)
}

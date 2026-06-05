package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/sagebynature/hitch/internal/clientshim"
	"github.com/sagebynature/hitch/internal/install"
)

var version = "0.1.0"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fatal(err)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "install":
			if isHelp(args[1:]) {
				printInstallHelp(stdout, false)
				return nil
			}
			return install.Run(args[1:], false)
		case "uninstall":
			if isHelp(args[1:]) {
				printInstallHelp(stdout, true)
				return nil
			}
			return install.Run(args[1:], true)
		case "help":
			printClientHelp(stdout, args[1:]...)
			return nil
		case "-h", "--help":
			printClientHelp(stdout)
			return nil
		}
	}
	return runHook(args, stdin, stdout)
}

func runHook(args []string, stdin io.Reader, stdout io.Writer) error {
	if isHelp(args) {
		printDispatchHelp(stdout)
		return nil
	}
	fs := flag.NewFlagSet("hitch-client", flag.ExitOnError)
	harness := fs.String("harness", "", "source harness")
	event := fs.String("event", "", "source event type")
	syncMode := fs.Bool("sync", false, "dispatch synchronously")
	url := fs.String("url", clientshim.DefaultURL(), "hitch API URL")
	versionFlag := fs.Bool("version", false, "print version")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *versionFlag {
		_, err := fmt.Fprintf(stdout, "hitch-client %s\n", version)
		return err
	}
	return clientshim.Run(context.Background(), clientshim.Options{Harness: *harness, Event: *event, Sync: *syncMode, URL: *url, Stdin: stdin, Stdout: stdout})
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "hitch-client:", err)
	os.Exit(1)
}

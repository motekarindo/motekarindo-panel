package main

import (
	"fmt"
	"os"

	"github.com/motekar/motekar-panel/internal/buildinfo"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "motekarctl: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "version"
	if len(args) > 0 {
		command = args[0]
	}

	switch command {
	case "preflight":
		return preflightCommand(args[1:])
	case "version":
		fmt.Println(buildinfo.String())
		return nil
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

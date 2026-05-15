package main

import (
	"flag"
	"fmt"
	"os"

	"lntab/internal/config"
	"lntab/internal/linker"
)

func main() {
	configPath := flag.String("config", "lntab.yml", "path to config file")
	dryRun := flag.Bool("dry-run", false, "print actions without executing")
	verbose := flag.Bool("verbose", false, "print each created symlink")
	flag.BoolVar(dryRun, "n", false, "print actions without executing")
	flag.BoolVar(verbose, "v", false, "print each created symlink")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: lntab [-config <path>] [-n] [-v] <command> [groups...]")
		fmt.Fprintln(os.Stderr, "commands: apply, clean")
		os.Exit(1)
	}

	command := args[0]
	groups := args[1:]

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	switch command {
	case "apply":
		l := linker.New(*dryRun, *verbose)
		if err := l.Apply(cfg, groups); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "clean":
		l := linker.New(*dryRun, *verbose)
		if err := l.Clean(cfg, groups); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
		os.Exit(1)
	}
}

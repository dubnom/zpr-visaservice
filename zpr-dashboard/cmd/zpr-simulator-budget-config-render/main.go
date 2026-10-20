package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"neboagency.com/zpr-dashborad/internal/simulatorbudgetconfig"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("zpr-simulator-budget-config-render", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "Simulator-only budget client JSON file (required)")
	outputPath := flags.String("output", "", "new runtime-input JSON output path (required; existing files are refused)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *configPath == "" || *outputPath == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: zpr-simulator-budget-config-render -config /path/to/simulator-budget.json -output /new/path/runtime-inputs.json")
		return 2
	}
	input, err := os.Open(*configPath)
	if err != nil {
		fmt.Fprintln(stderr, "zpr-simulator-budget-config-render: cannot open Simulator budget client configuration")
		return 1
	}
	config, readErr := simulatorbudgetconfig.Read(input)
	closeErr := input.Close()
	if readErr != nil {
		fmt.Fprintf(stderr, "zpr-simulator-budget-config-render: %v\n", readErr)
		return 1
	}
	if closeErr != nil {
		fmt.Fprintln(stderr, "zpr-simulator-budget-config-render: cannot close Simulator budget client configuration")
		return 1
	}
	if err := config.CheckFiles(); err != nil {
		fmt.Fprintf(stderr, "zpr-simulator-budget-config-render: offline file checks failed: %v\n", err)
		return 1
	}
	data, err := json.MarshalIndent(config.Runtime(), "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, "zpr-simulator-budget-config-render: cannot encode runtime inputs")
		return 1
	}
	data = append(data, '\n')
	if err := writeNewFile(*outputPath, data); err != nil {
		fmt.Fprintf(stderr, "zpr-simulator-budget-config-render: cannot create runtime input file: %v\n", err)
		return 1
	}
	if _, err := fmt.Fprintln(stdout, "Rendered Simulator-only budget resolver inputs after offline file checks.\nNo credentials were copied, no services were contacted, and no configuration was applied."); err != nil {
		fmt.Fprintln(stderr, "zpr-simulator-budget-config-render: cannot write result")
		return 1
	}
	return 0
}

func writeNewFile(path string, data []byte) error {
	directory := filepath.Dir(path)
	file, err := os.CreateTemp(directory, ".zpr-simulator-budget-runtime-*")
	if err != nil {
		return err
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Link(temporaryPath, path); err != nil {
		return err
	}
	if err := os.Remove(temporaryPath); err != nil {
		return err
	}
	directoryFile, err := os.Open(directory)
	if err != nil {
		return err
	}
	if err := directoryFile.Sync(); err != nil {
		_ = directoryFile.Close()
		return err
	}
	return directoryFile.Close()
}

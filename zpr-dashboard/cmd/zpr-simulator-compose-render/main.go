package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"neboagency.com/zpr-dashborad/internal/simulatorruntimecompose"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("zpr-simulator-compose-render", flag.ContinueOnError)
	flags.SetOutput(stderr)
	inputPath := flags.String("runtime-inputs", "", "Simulator budget resolver runtime JSON file (required)")
	serviceName := flags.String("service", "", "existing Compose service name to override (required)")
	outputPath := flags.String("output", "", "new Compose override JSON output path (required; existing files are refused)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *inputPath == "" || *serviceName == "" || *outputPath == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: zpr-simulator-compose-render -runtime-inputs /path/to/simulator-budget-runtime-inputs.json -service existing-simulator-service -output /new/path/simulator-budget.override.yaml")
		return 2
	}
	if !filepath.IsAbs(*outputPath) || filepath.Clean(*outputPath) != *outputPath {
		fmt.Fprintln(stderr, "zpr-simulator-compose-render: output path must be a canonical absolute path")
		return 1
	}
	input, err := os.Open(*inputPath)
	if err != nil {
		fmt.Fprintln(stderr, "zpr-simulator-compose-render: cannot open Simulator runtime inputs")
		return 1
	}
	runtime, readErr := simulatorruntimecompose.ReadRuntime(input)
	closeErr := input.Close()
	if readErr != nil {
		fmt.Fprintf(stderr, "zpr-simulator-compose-render: %v\n", readErr)
		return 1
	}
	if closeErr != nil {
		fmt.Fprintln(stderr, "zpr-simulator-compose-render: cannot close Simulator runtime inputs")
		return 1
	}
	fragment, err := simulatorruntimecompose.Build(runtime, *serviceName)
	if err != nil {
		fmt.Fprintf(stderr, "zpr-simulator-compose-render: %v\n", err)
		return 1
	}
	data, err := json.MarshalIndent(fragment, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, "zpr-simulator-compose-render: cannot encode Compose override")
		return 1
	}
	data = append(data, '\n')
	if err := writeNewFile(*outputPath, data); err != nil {
		fmt.Fprintf(stderr, "zpr-simulator-compose-render: cannot create Compose override: %v\n", err)
		return 1
	}
	if _, err := fmt.Fprintln(stdout, "Rendered a merge-safe, non-starting Simulator budget Compose override.\nNo Docker or Compose commands were run; no services or host/network state were changed."); err != nil {
		fmt.Fprintln(stderr, "zpr-simulator-compose-render: cannot write result")
		return 1
	}
	return 0
}

func writeNewFile(outputPath string, data []byte) (returnErr error) {
	directory := filepath.Dir(outputPath)
	directoryInfo, err := os.Lstat(directory)
	if err != nil || !directoryInfo.IsDir() || directoryInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("output parent must be an existing regular directory: %s", directory)
	}
	file, err := os.CreateTemp(directory, ".zpr-simulator-compose-*")
	if err != nil {
		return err
	}
	temporaryPath := file.Name()
	defer func() {
		if err := os.Remove(temporaryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			returnErr = errors.Join(returnErr, fmt.Errorf("remove temporary output: %w", err))
		}
	}()
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
	if err := os.Link(temporaryPath, outputPath); err != nil {
		return err
	}
	if err := os.Remove(temporaryPath); err != nil {
		return errors.Join(err, os.Remove(outputPath))
	}
	directoryFile, err := os.Open(directory)
	if err != nil {
		return errors.Join(err, os.Remove(outputPath))
	}
	if err := directoryFile.Sync(); err != nil {
		_ = directoryFile.Close()
		return errors.Join(err, os.Remove(outputPath))
	}
	if err := directoryFile.Close(); err != nil {
		return errors.Join(err, os.Remove(outputPath))
	}
	return nil
}

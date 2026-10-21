package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"neboagency.com/zpr-dashborad/internal/systemruntimecompose"
)

type outputFile struct {
	path string
	data []byte
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("zpr-system-compose-render", flag.ContinueOnError)
	flags.SetOutput(stderr)
	inputPath := flags.String("runtime-inputs", "", "new Control Room runtime-input JSON from zpr-system-config-render (required)")
	image := flags.String("image", "", "repository@sha256 digest or local sha256 image ID (required)")
	composePath := flags.String("compose-output", "", "new Compose plan JSON output path (required)")
	oidcPath := flags.String("oidc-output", "", "new private OIDC runtime JSON output path (required)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *inputPath == "" || *image == "" || *composePath == "" || *oidcPath == "" ||
		flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: zpr-system-compose-render -runtime-inputs /path/to/runtime-inputs.json -image repository@sha256:<digest>|sha256:<local-image-id> -compose-output /new/path/compose.json -oidc-output /new/private/path/operator-oidc.json")
		return 2
	}
	if !filepath.IsAbs(*composePath) || filepath.Clean(*composePath) != *composePath ||
		!filepath.IsAbs(*oidcPath) || filepath.Clean(*oidcPath) != *oidcPath ||
		*composePath == *oidcPath {
		fmt.Fprintln(stderr, "zpr-system-compose-render: output paths must be distinct canonical absolute paths")
		return 1
	}
	input, err := os.Open(*inputPath)
	if err != nil {
		fmt.Fprintln(stderr, "zpr-system-compose-render: cannot open runtime inputs")
		return 1
	}
	runtime, readErr := systemruntimecompose.ReadRuntime(input)
	closeErr := input.Close()
	if readErr != nil {
		fmt.Fprintf(stderr, "zpr-system-compose-render: %v\n", readErr)
		return 1
	}
	if closeErr != nil {
		fmt.Fprintln(stderr, "zpr-system-compose-render: cannot close runtime inputs")
		return 1
	}
	plan, oidc, err := systemruntimecompose.Build(runtime, *image, *oidcPath)
	if err != nil {
		fmt.Fprintf(stderr, "zpr-system-compose-render: %v\n", err)
		return 1
	}
	compose, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, "zpr-system-compose-render: cannot encode Compose plan")
		return 1
	}
	compose = append(compose, '\n')
	if err := writeNewFiles([]outputFile{
		{path: *composePath, data: compose},
		{path: *oidcPath, data: oidc},
	}); err != nil {
		fmt.Fprintf(stderr, "zpr-system-compose-render: cannot publish new plan files: %v\n", err)
		return 1
	}
	if _, err := fmt.Fprintln(stdout, "Rendered a non-starting, host-network Compose plan and mode-0600 OIDC runtime file.\nNo Docker or Compose commands were run; no service was started or host/network state changed."); err != nil {
		fmt.Fprintln(stderr, "zpr-system-compose-render: cannot write result")
		return 1
	}
	return 0
}

func writeNewFiles(outputs []outputFile) (returnErr error) {
	if len(outputs) == 0 {
		return errors.New("no output files were provided")
	}
	seen := make(map[string]bool, len(outputs))
	temporaryPaths := make([]string, 0, len(outputs))
	publishedPaths := make([]string, 0, len(outputs))
	defer func() {
		for _, path := range temporaryPaths {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				returnErr = errors.Join(returnErr, fmt.Errorf("remove temporary output %s: %w", path, err))
			}
		}
	}()
	for _, output := range outputs {
		if !filepath.IsAbs(output.path) || filepath.Clean(output.path) != output.path ||
			seen[output.path] {
			return errors.New("output paths must be distinct canonical absolute paths")
		}
		seen[output.path] = true
		directory := filepath.Dir(output.path)
		info, err := os.Lstat(directory)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("output parent must be an existing regular directory: %s", directory)
		}
		file, err := os.CreateTemp(directory, ".zpr-system-compose-*")
		if err != nil {
			return err
		}
		temporaryPaths = append(temporaryPaths, file.Name())
		if err := file.Chmod(0o600); err != nil {
			_ = file.Close()
			return err
		}
		if _, err := file.Write(output.data); err != nil {
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
	}
	for index, output := range outputs {
		if err := os.Link(temporaryPaths[index], output.path); err != nil {
			var rollbackErr error
			for _, published := range publishedPaths {
				if removeErr := os.Remove(published); removeErr != nil {
					rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove published output %s: %w", published, removeErr))
				}
			}
			return errors.Join(err, rollbackErr)
		}
		publishedPaths = append(publishedPaths, output.path)
	}
	for _, output := range outputs {
		directory, err := os.Open(filepath.Dir(output.path))
		if err != nil {
			return rollbackPublished(outputs, publishedPaths, err)
		}
		if err := directory.Sync(); err != nil {
			_ = directory.Close()
			return rollbackPublished(outputs, publishedPaths, err)
		}
		if err := directory.Close(); err != nil {
			return rollbackPublished(outputs, publishedPaths, err)
		}
	}
	return nil
}

func rollbackPublished(outputs []outputFile, paths []string, cause error) error {
	errs := []error{cause}
	for _, path := range paths {
		if err := os.Remove(path); err != nil {
			errs = append(errs, fmt.Errorf("remove published output %s: %w", path, err))
		}
	}
	for _, output := range outputs {
		directory, err := os.Open(filepath.Dir(output.path))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := directory.Sync(); err != nil {
			errs = append(errs, err)
		}
		if err := directory.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

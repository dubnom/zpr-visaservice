package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"neboagency.com/zpr-dashborad/internal/systemconfig"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("zpr-system-config-render", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "company management-bootstrap JSON file (required)")
	outputPath := flags.String("output", "", "new runtime-input JSON output path (required; existing files are refused)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *configPath == "" || *outputPath == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: zpr-system-config-render -config /path/to/company.json -output /new/path/runtime-inputs.json")
		return 2
	}
	input, err := os.Open(*configPath)
	if err != nil {
		fmt.Fprintln(stderr, "zpr-system-config-render: cannot open company configuration")
		return 1
	}
	config, readErr := systemconfig.Read(input)
	closeErr := input.Close()
	if readErr != nil {
		fmt.Fprintf(stderr, "zpr-system-config-render: %v\n", readErr)
		return 1
	}
	if closeErr != nil {
		fmt.Fprintln(stderr, "zpr-system-config-render: cannot close company configuration")
		return 1
	}
	if err := config.CheckFiles(time.Now()); err != nil {
		fmt.Fprintf(stderr, "zpr-system-config-render: offline company file checks failed: %v\n", err)
		return 1
	}
	runtime, err := systemconfig.RenderControlRoomRuntime(config)
	if err != nil {
		fmt.Fprintf(stderr, "zpr-system-config-render: %v\n", err)
		return 1
	}
	data, err := json.MarshalIndent(runtime, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, "zpr-system-config-render: cannot encode runtime inputs")
		return 1
	}
	data = append(data, '\n')
	if err := writeNewFile(*outputPath, data); err != nil {
		fmt.Fprintf(stderr, "zpr-system-config-render: cannot create runtime input file: %v\n", err)
		return 1
	}
	if _, err := fmt.Fprintln(stdout, "Rendered Control Room runtime inputs after offline file checks.\nNo services were started, no configuration was applied, and service connectivity was not checked."); err != nil {
		fmt.Fprintln(stderr, "zpr-system-config-render: cannot write result")
		return 1
	}
	return 0
}

func writeNewFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
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
	return nil
}

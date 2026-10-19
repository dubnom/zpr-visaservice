package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"neboagency.com/zpr-dashborad/internal/discovery"
)

func runPolicy(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("zpr-discover policy", flag.ContinueOnError)
	flags.SetOutput(stderr)
	input := flags.String("input", "", "saved version-2 discovery report JSON (required; no network activity)")
	output := flags.String("output", "-", "ZPL draft destination; '-' is stdout; files are private and never overwritten")
	allowPartial := flags.Bool("allow-partial", false, "explicitly accept an incomplete or error-bearing report; known open endpoints only")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "zpr-discover policy: %v\n", err)
		return 2
	}
	if *input == "" || flags.NArg() != 0 {
		return fail(errors.New("require -input report.json and no positional arguments"))
	}
	file, err := os.Open(*input)
	if err != nil {
		return fail(fmt.Errorf("open report: %w", err))
	}
	report, err := discovery.ReadPolicyReport(file)
	closeErr := file.Close()
	if err != nil {
		return fail(err)
	}
	if closeErr != nil {
		return fail(closeErr)
	}
	draft, count, err := discovery.PolicyDraft(report, *allowPartial)
	if err != nil {
		return fail(err)
	}
	if err := writeOutput(stdout, *output, func(writer io.Writer) error {
		_, err := io.WriteString(writer, draft)
		return err
	}); err != nil {
		return fail(fmt.Errorf("write draft: %w", err))
	}
	fmt.Fprintf(stderr, "Exported %d deny-by-default service drafts; no allow rules, provider bindings, DNS changes or policy activation. Review before use.\n", count)
	if !report.Complete || report.Error != "" || report.EndpointErrors != 0 || report.HostCheckErrors != 0 {
		fmt.Fprintln(stderr, "WARNING: partial/error-bearing inventory accepted; this draft is not a complete service inventory.")
	}
	return 0
}

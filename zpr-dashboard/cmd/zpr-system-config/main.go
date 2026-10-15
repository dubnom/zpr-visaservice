package main

import (
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
	flags := flag.NewFlagSet("zpr-system-config", flag.ContinueOnError)
	flags.SetOutput(stderr)
	filename := flags.String("config", "", "company management-bootstrap JSON file to validate (required; no mutations)")
	checkFiles := flags.Bool("check-files", false, "also inspect referenced local TLS, secret and CA files (no network access or mutations)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *filename == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: zpr-system-config -config /path/to/company.json [-check-files]")
		return 2
	}
	file, err := os.Open(*filename)
	if err != nil {
		fmt.Fprintln(stderr, "zpr-system-config: cannot open company configuration")
		return 1
	}
	config, readErr := systemconfig.Read(file)
	closeErr := file.Close()
	if readErr != nil {
		fmt.Fprintf(stderr, "zpr-system-config: %v\n", readErr)
		return 1
	}
	if closeErr != nil {
		fmt.Fprintln(stderr, "zpr-system-config: cannot close company configuration")
		return 1
	}
	result := "Management bootstrap configuration structure and file-reference syntax are valid.\nReferenced files, TLS trust, OIDC discovery/login, DNS and service readiness have NOT been checked."
	if *checkFiles {
		if err := config.CheckFiles(time.Now()); err != nil {
			fmt.Fprintf(stderr, "zpr-system-config: %v\n", err)
			return 1
		}
		result = "Management bootstrap configuration and referenced local files pass offline checks.\nFile protection, TLS key/hostname/validity/supplied-chain checks, OIDC secret format and optional CA format/validity were checked.\nBrowser trust, revocation, OIDC discovery/login, DNS and service readiness have NOT been checked."
	}
	if _, err := fmt.Fprintln(stdout, result+"\nNo configuration was applied; runtime rendering and installation are not yet implemented."); err != nil {
		fmt.Fprintln(stderr, "zpr-system-config: cannot write validation result")
		return 1
	}
	return 0
}

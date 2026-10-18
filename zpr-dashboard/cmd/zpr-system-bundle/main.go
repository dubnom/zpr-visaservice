package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"neboagency.com/zpr-dashborad/internal/systembundle"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("zpr-system-bundle", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", "", "bundle manifest path relative to -bundle-dir (required)")
	bundleDirectory := flags.String("bundle-dir", "", "directory containing manifest payloads (required)")
	pairedManifestPath := flags.String("paired-system-manifest", "", "system manifest path relative to -paired-system-dir when validating a companion")
	pairedBundleDirectory := flags.String("paired-system-dir", "", "system bundle directory required when validating a companion")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || *manifestPath == "" || *bundleDirectory == "" {
		fmt.Fprintln(stderr, "usage: zpr-system-bundle -manifest manifest.json -bundle-dir /path/bundle [-paired-system-manifest manifest.json -paired-system-dir /path/system-bundle]")
		return 2
	}
	if (*pairedManifestPath == "") != (*pairedBundleDirectory == "") {
		fmt.Fprintln(stderr, "zpr-system-bundle: paired system manifest and directory must be provided together")
		return 2
	}

	manifest, err := readManifest(*bundleDirectory, *manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "zpr-system-bundle: %v\n", err)
		return 1
	}
	if manifest.BundleKind == "companion" && *pairedManifestPath == "" {
		fmt.Fprintln(stderr, "zpr-system-bundle: companion validation requires a paired system manifest and directory")
		return 2
	}
	if manifest.BundleKind == "system" && *pairedManifestPath != "" {
		fmt.Fprintln(stderr, "zpr-system-bundle: system validation must not include paired-system arguments")
		return 2
	}
	if err := systembundle.ValidateBundle(manifest, *bundleDirectory); err != nil {
		fmt.Fprintf(stderr, "zpr-system-bundle: %v\n", err)
		return 1
	}
	if manifest.BundleKind == "companion" {
		systemManifest, err := readManifest(*pairedBundleDirectory, *pairedManifestPath)
		if err != nil {
			fmt.Fprintf(stderr, "zpr-system-bundle: %v\n", err)
			return 1
		}
		if err := systembundle.ValidateBundle(systemManifest, *pairedBundleDirectory); err != nil {
			fmt.Fprintf(stderr, "zpr-system-bundle: paired system bundle: %v\n", err)
			return 1
		}
		if err := systembundle.ValidateCompanionPair(manifest, systemManifest); err != nil {
			fmt.Fprintf(stderr, "zpr-system-bundle: %v\n", err)
			return 1
		}
	}
	message := "Bundle manifest and local payload checks passed."
	if manifest.BundleKind == "companion" {
		message = "Companion manifest, local payload checks and exact ZPR pairing passed."
	}
	message += "\nSignatures, image contents, live dependencies, installation, runtime health and release qualification were NOT checked.\nNo services were started and no host changes were made."
	if _, err := fmt.Fprintln(stdout, message); err != nil {
		fmt.Fprintln(stderr, "zpr-system-bundle: cannot write validation result")
		return 1
	}
	return 0
}

func readManifest(directory, filename string) (systembundle.Manifest, error) {
	return systembundle.ReadBundleManifest(directory, filename)
}

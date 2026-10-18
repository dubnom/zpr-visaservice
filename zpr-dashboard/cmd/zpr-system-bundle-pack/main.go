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
	flags := flag.NewFlagSet("zpr-system-bundle-pack", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", "", "bundle manifest path relative to -bundle-dir (required)")
	bundleDirectory := flags.String("bundle-dir", "", "directory containing manifest and payloads (required)")
	outputPath := flags.String("output", "", "new archive output path outside the bundle directory (required)")
	pairedManifestPath := flags.String("paired-system-manifest", "", "system manifest path relative to -paired-system-dir when packaging a companion")
	pairedBundleDirectory := flags.String("paired-system-dir", "", "system bundle directory required when packaging a companion")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || *manifestPath == "" || *bundleDirectory == "" || *outputPath == "" {
		fmt.Fprintln(stderr, "usage: zpr-system-bundle-pack -manifest manifest.json -bundle-dir /path/bundle -output /path/bundle.tar.gz [-paired-system-manifest manifest.json -paired-system-dir /path/system-bundle]")
		return 2
	}
	if (*pairedManifestPath == "") != (*pairedBundleDirectory == "") {
		fmt.Fprintln(stderr, "zpr-system-bundle-pack: paired system manifest and directory must be provided together")
		return 2
	}

	manifest, err := systembundle.ReadBundleManifest(*bundleDirectory, *manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "zpr-system-bundle-pack: %v\n", err)
		return 1
	}
	if manifest.BundleKind == "companion" && *pairedManifestPath == "" {
		fmt.Fprintln(stderr, "zpr-system-bundle-pack: companion packaging requires a paired system manifest and directory")
		return 2
	}
	if manifest.BundleKind == "system" && *pairedManifestPath != "" {
		fmt.Fprintln(stderr, "zpr-system-bundle-pack: system packaging must not include paired-system arguments")
		return 2
	}
	if err := systembundle.ValidateBundle(manifest, *bundleDirectory); err != nil {
		fmt.Fprintf(stderr, "zpr-system-bundle-pack: %v\n", err)
		return 1
	}
	if manifest.BundleKind == "companion" {
		systemManifest, err := systembundle.ReadBundleManifest(*pairedBundleDirectory, *pairedManifestPath)
		if err != nil {
			fmt.Fprintf(stderr, "zpr-system-bundle-pack: %v\n", err)
			return 1
		}
		if err := systembundle.ValidateBundle(systemManifest, *pairedBundleDirectory); err != nil {
			fmt.Fprintf(stderr, "zpr-system-bundle-pack: paired system bundle: %v\n", err)
			return 1
		}
		if err := systembundle.ValidateCompanionPair(manifest, systemManifest); err != nil {
			fmt.Fprintf(stderr, "zpr-system-bundle-pack: %v\n", err)
			return 1
		}
	}
	digest, err := systembundle.PackageBundle(*bundleDirectory, *manifestPath, *outputPath)
	if err != nil {
		fmt.Fprintf(stderr, "zpr-system-bundle-pack: %v\n", err)
		return 1
	}
	if _, err := fmt.Fprintf(stdout, "Created engineering-unqualified %s bundle archive.\nSHA-256: %s\nSignatures, manifest authenticity, image contents, installation, runtime health and release qualification were NOT checked.\n", manifest.BundleKind, digest); err != nil {
		fmt.Fprintln(stderr, "zpr-system-bundle-pack: cannot write packaging result")
		return 1
	}
	return 0
}

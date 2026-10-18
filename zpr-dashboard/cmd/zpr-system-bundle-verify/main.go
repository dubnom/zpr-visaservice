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
	flags := flag.NewFlagSet("zpr-system-bundle-verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	archivePath := flags.String("archive", "", "downloaded system or companion .tar.gz bundle (required)")
	pairedSystemArchive := flags.String("paired-system-archive", "", "matching downloaded system .tar.gz archive required for a companion")
	signaturePath := flags.String("signature", "", "detached signature JSON file; must be used with -trusted-key")
	trustedKeyPath := flags.String("trusted-key", "", "independently trusted Ed25519 public key PEM; must be used with -signature")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || *archivePath == "" {
		fmt.Fprintln(stderr, "usage: zpr-system-bundle-verify -archive /path/bundle.tar.gz [-paired-system-archive /path/system.tar.gz] [-signature /path/bundle.sig.json -trusted-key /trusted/release-key.pem]")
		return 2
	}
	if (*signaturePath == "") != (*trustedKeyPath == "") {
		fmt.Fprintln(stderr, "zpr-system-bundle-verify: -signature and -trusted-key must be provided together")
		return 2
	}
	manifest, digest, err := systembundle.ValidateArchive(*archivePath)
	if err != nil {
		fmt.Fprintf(stderr, "zpr-system-bundle-verify: %v\n", err)
		return 1
	}
	if manifest.BundleKind == "companion" && *pairedSystemArchive == "" {
		fmt.Fprintln(stderr, "zpr-system-bundle-verify: companion verification requires a paired system archive")
		return 2
	}
	if manifest.BundleKind == "system" && *pairedSystemArchive != "" {
		fmt.Fprintln(stderr, "zpr-system-bundle-verify: system verification must not include a paired system archive")
		return 2
	}
	if *pairedSystemArchive != "" {
		systemManifest, _, err := systembundle.ValidateArchive(*pairedSystemArchive)
		if err != nil {
			fmt.Fprintf(stderr, "zpr-system-bundle-verify: paired system archive: %v\n", err)
			return 1
		}
		if err := systembundle.ValidateCompanionPair(manifest, systemManifest); err != nil {
			fmt.Fprintf(stderr, "zpr-system-bundle-verify: %v\n", err)
			return 1
		}
	}
	authentication := "No publisher signature was checked."
	if *signaturePath != "" {
		signature, err := readSignature(*signaturePath)
		if err != nil {
			fmt.Fprintf(stderr, "zpr-system-bundle-verify: %v\n", err)
			return 1
		}
		if err := systembundle.VerifyArchiveSignature(*archivePath, signature, *trustedKeyPath); err != nil {
			fmt.Fprintf(stderr, "zpr-system-bundle-verify: %v\n", err)
			return 1
		}
		authentication = fmt.Sprintf("Ed25519 signature verified against the supplied trusted key (SHA-256 %s).", signature.SignerKeySHA256)
	}
	message := fmt.Sprintf("Engineering-unqualified %s bundle archive and declared payload checks passed.\nArchive SHA-256: %s\n", manifest.BundleKind, digest)
	if manifest.BundleKind == "companion" {
		message = fmt.Sprintf("Engineering-unqualified companion archive, declared payload checks and exact ZPR pairing passed.\nArchive SHA-256: %s\n", digest)
	}
	message += authentication + "\nImage contents, installation, runtime health and release qualification were NOT checked.\nNo files were extracted, services were started or host changes made."
	if _, err := fmt.Fprintln(stdout, message); err != nil {
		fmt.Fprintln(stderr, "zpr-system-bundle-verify: cannot write verification result")
		return 1
	}
	return 0
}

func readSignature(path string) (systembundle.ArchiveSignature, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return systembundle.ArchiveSignature{}, errors.New("signature must be a regular non-symlink file")
	}
	file, err := os.Open(path)
	if err != nil {
		return systembundle.ArchiveSignature{}, errors.New("signature file cannot be opened")
	}
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return systembundle.ArchiveSignature{}, errors.New("signature file changed while opening")
	}
	signature, readErr := systembundle.ReadArchiveSignature(file)
	closeErr := file.Close()
	if readErr != nil {
		return systembundle.ArchiveSignature{}, readErr
	}
	if closeErr != nil {
		return systembundle.ArchiveSignature{}, errors.New("signature file cannot be closed")
	}
	return signature, nil
}

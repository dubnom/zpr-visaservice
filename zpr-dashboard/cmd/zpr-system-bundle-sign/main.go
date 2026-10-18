package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"neboagency.com/zpr-dashborad/internal/systembundle"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("zpr-system-bundle-sign", flag.ContinueOnError)
	flags.SetOutput(stderr)
	archivePath := flags.String("archive", "", "engineering bundle .tar.gz to sign (required)")
	privateKeyPath := flags.String("private-key", "", "protected PKCS#8 Ed25519 private key PEM (required)")
	signaturePath := flags.String("signature", "", "new detached signature JSON output path (required)")
	pairedSystemArchive := flags.String("paired-system-archive", "", "matching system archive required when signing a companion")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || *archivePath == "" || *privateKeyPath == "" || *signaturePath == "" {
		fmt.Fprintln(stderr, "usage: zpr-system-bundle-sign -archive /path/bundle.tar.gz -private-key /protected/release-key.pem -signature /new/path/bundle.sig.json [-paired-system-archive /path/system.tar.gz]")
		return 2
	}
	manifest, _, err := systembundle.ValidateArchive(*archivePath)
	if err != nil {
		fmt.Fprintf(stderr, "zpr-system-bundle-sign: %v\n", err)
		return 1
	}
	if manifest.BundleKind == "companion" && *pairedSystemArchive == "" {
		fmt.Fprintln(stderr, "zpr-system-bundle-sign: companion signing requires a paired system archive")
		return 2
	}
	if manifest.BundleKind == "system" && *pairedSystemArchive != "" {
		fmt.Fprintln(stderr, "zpr-system-bundle-sign: system signing must not include a paired system archive")
		return 2
	}
	if *pairedSystemArchive != "" {
		systemManifest, _, err := systembundle.ValidateArchive(*pairedSystemArchive)
		if err != nil {
			fmt.Fprintf(stderr, "zpr-system-bundle-sign: paired system archive: %v\n", err)
			return 1
		}
		if err := systembundle.ValidateCompanionPair(manifest, systemManifest); err != nil {
			fmt.Fprintf(stderr, "zpr-system-bundle-sign: %v\n", err)
			return 1
		}
	}
	signature, err := systembundle.SignArchive(*archivePath, *privateKeyPath)
	if err != nil {
		fmt.Fprintf(stderr, "zpr-system-bundle-sign: %v\n", err)
		return 1
	}
	if err := writeSignatureExclusive(*signaturePath, signature); err != nil {
		fmt.Fprintf(stderr, "zpr-system-bundle-sign: %v\n", err)
		return 1
	}
	if _, err := fmt.Fprintf(stdout, "Created detached Ed25519 signature for engineering-unqualified %s bundle %q.\nSigner public-key SHA-256: %s\nThis signature authenticates these archive bytes only; it does not qualify the bundle or authorize installation.\n", manifest.BundleKind, manifest.ReleaseID, signature.SignerKeySHA256); err != nil {
		fmt.Fprintln(stderr, "zpr-system-bundle-sign: cannot write signing result")
		return 1
	}
	return 0
}

func writeSignatureExclusive(path string, signature systembundle.ArchiveSignature) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return errors.New("signature output path is invalid")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return errors.New("signature output parent is unavailable")
	}
	output := filepath.Join(parent, filepath.Base(absolute))
	if _, err := os.Lstat(output); err == nil {
		return errors.New("signature output already exists; refusing to overwrite")
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("signature output cannot be inspected")
	}
	temporary, err := os.CreateTemp(parent, ".zpr-signature-*.tmp")
	if err != nil {
		return errors.New("signature output cannot be created")
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0644); err != nil {
		_ = temporary.Close()
		return errors.New("signature output permissions cannot be set")
	}
	if err := systembundle.WriteArchiveSignature(temporary, signature); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return errors.New("signature output cannot be synchronized")
	}
	if err := temporary.Close(); err != nil {
		return errors.New("signature output cannot be closed")
	}
	if err := os.Link(temporaryPath, output); err != nil {
		return errors.New("signature output cannot be published without overwriting an existing path")
	}
	return nil
}

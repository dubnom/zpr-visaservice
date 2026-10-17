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
	renderOIDC := flags.String("render-operator-oidc-config", "", "write the validated operator OIDC runtime config to a new protected file (requires the referenced input files to pass offline checks)")
	renderCompose := flags.String("render-control-room-compose", "", "write a protected host-network Compose service definition (requires rendered OIDC config and an explicit image)")
	controlRoomImage := flags.String("control-room-image", "", "explicit Control Room container image reference (required when rendering Compose)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *filename == "" || flags.NArg() != 0 ||
		(*renderCompose != "" && (*renderOIDC == "" || *controlRoomImage == "")) ||
		(*renderCompose == "" && *controlRoomImage != "") ||
		(*renderCompose != "" && *renderCompose == *renderOIDC) {
		fmt.Fprintln(stderr, "usage: zpr-system-config -config /path/to/company.json [-check-files] [-render-operator-oidc-config /absolute/path [-render-control-room-compose /absolute/path -control-room-image image]]")
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
	result := "Management bootstrap configuration structure and file-reference syntax are valid.\nReferenced files, TLS trust, OIDC discovery/login, DNS and service readiness have NOT been checked.\nNo configuration was written or applied; no service was started."
	if *checkFiles || *renderOIDC != "" || *renderCompose != "" {
		if err := config.CheckFiles(time.Now()); err != nil {
			fmt.Fprintf(stderr, "zpr-system-config: %v\n", err)
			return 1
		}
		result = "Management bootstrap configuration and referenced local files pass offline checks.\nFile protection, Control Room server certificate/key, Control-Service client certificate/key and server CA, OIDC secret format and optional CA format/validity were checked.\nBrowser trust, revocation, Control-Service reachability/server trust, OIDC discovery/login, DNS and service readiness have NOT been checked.\nComplete system runtime rendering and installation are not yet implemented.\nNo configuration was applied; no service was started."
	}
	if *renderOIDC != "" {
		if err := config.RenderOperatorOIDCConfig(*renderOIDC); err != nil {
			fmt.Fprintf(stderr, "zpr-system-config: %v\n", err)
			return 1
		}
		result = "Management bootstrap configuration and referenced local files pass offline checks.\nFile protection, Control Room server certificate/key, Control-Service client certificate/key and server CA, OIDC secret format and optional CA format/validity were checked.\nOperator OIDC runtime configuration was rendered without copying the client secret.\nOIDC discovery/login, DNS, Control-Service reachability/server trust and service readiness have NOT been checked.\nComplete system runtime rendering and installation are not yet implemented.\nNo service configuration was applied; no service was started."
	}
	if *renderCompose != "" {
		if err := config.RenderControlRoomCompose(*renderCompose, *controlRoomImage, *renderOIDC); err != nil {
			fmt.Fprintf(stderr, "zpr-system-config: %v\n", err)
			return 1
		}
		result = "Management bootstrap and referenced local files pass offline checks.\nOperator OIDC runtime configuration and a host-network Control Room Compose service definition were rendered as protected files.\nThe service uses the explicit Control-Service mTLS URL and mounted client trust files; no secret contents were copied.\nThis Compose service is not the complete ZPR system. OIDC discovery/login, Control-Service reachability/trust, host management-network reachability, change-review storage/signing, and service readiness have NOT been checked.\nNo service configuration was applied; no service was started."
	}
	if _, err := fmt.Fprintln(stdout, result+"\nNo service configuration was applied; no service was started."); err != nil {
		fmt.Fprintln(stderr, "zpr-system-config: cannot write validation result")
		return 1
	}
	return 0
}

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"neboagency.com/zpr-dashborad/internal/enrollment"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

type deviceConfig struct {
	Version                  int    `json:"version"`
	Audience                 string `json:"audience"`
	Organization             string `json:"organization"`
	InvitationID             string `json:"invitation_id"`
	StateDirectory           string `json:"state_directory"`
	CAFile                   string `json:"ca_file"`
	BASCAFile                string `json:"bas_ca_file,omitempty"`
	AllowSoftwareDevelopment bool   `json:"allow_software_development"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "zpr-enrollment-device: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("zpr-enrollment-device", flag.ContinueOnError)
	flags.SetOutput(errOut)
	configPath := flags.String("config", "", "private device configuration JSON")
	codePath := flags.String("code-file", "", "claim only: private one-time code file; never a command-line code")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || *configPath == "" {
		return errors.New("usage: zpr-enrollment-device -config /absolute/device.json [-code-file /absolute/private-code] initialize|claim|status|prepare-runtime")
	}
	action := flags.Arg(0)
	if action != "initialize" && action != "claim" && action != "status" && action != "prepare-runtime" {
		return errors.New("action must be initialize, claim, status or prepare-runtime")
	}
	if (action == "claim") != (*codePath != "") {
		return errors.New("claim requires a private code file; other actions must not receive one")
	}
	for _, path := range []string{*configPath, *codePath} {
		if path != "" && (!filepath.IsAbs(path) || filepath.Clean(path) != path) {
			return errors.New("configuration and code files require canonical absolute paths")
		}
	}
	data, err := servicehttp.ReadOperatorFile(*configPath, 65536, true)
	if err != nil {
		return fmt.Errorf("read private device configuration: %w", err)
	}
	var config deviceConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return errors.New("invalid device configuration JSON")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("device configuration must contain exactly one JSON object")
	}
	if config.Version != 1 || !config.AllowSoftwareDevelopment || config.Organization == "" || config.InvitationID == "" ||
		!filepath.IsAbs(config.StateDirectory) || filepath.Clean(config.StateDirectory) != config.StateDirectory ||
		!filepath.IsAbs(config.CAFile) || filepath.Clean(config.CAFile) != config.CAFile {
		return errors.New("version 1, explicit software-development approval, organization/invitation, canonical absolute state directory and CA file are required")
	}
	if action == "prepare-runtime" && (!filepath.IsAbs(config.BASCAFile) || filepath.Clean(config.BASCAFile) != config.BASCAFile) {
		return errors.New("prepare-runtime requires an explicit canonical absolute bas_ca_file")
	}
	ca, err := servicehttp.ReadOperatorFile(config.CAFile, 65536, false)
	if err != nil {
		return fmt.Errorf("read device service CA: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return errors.New("device service CA contains no certificates")
	}
	metadata := enrollment.LocalEnrollment{Audience: config.Audience, Organization: config.Organization, InvitationID: config.InvitationID}
	identity, err := enrollment.LoadSoftwareIdentity(config.StateDirectory)
	if errors.Is(err, os.ErrNotExist) && action == "initialize" {
		identity, err = enrollment.CreateSoftwareIdentity(config.StateDirectory, metadata)
	}
	if err != nil {
		return fmt.Errorf("load retained device identity (initialize explicitly for a new identity): %w", err)
	}
	if identity.Metadata() != metadata {
		return errors.New("retained identity belongs to a different audience, organization or invitation; rebinding is prohibited")
	}
	client, err := enrollment.NewClientWithRuntimeKey(config.Audience, roots, identity, identity.RuntimeSigner())
	if err != nil {
		return fmt.Errorf("configure key-bound enrollment client: %w", err)
	}
	defer client.Close()
	if action == "initialize" {
		public, err := x509.MarshalPKIXPublicKey(identity.RuntimeSigner().Public())
		if err != nil {
			return err
		}
		hash := sha256.Sum256(public)
		return json.NewEncoder(out).Encode(struct {
			Metadata           enrollment.LocalEnrollment `json:"enrollment"`
			Fingerprint        string                     `json:"key_fingerprint"`
			RuntimeFingerprint string                     `json:"runtime_key_fingerprint"`
		}{metadata, client.Fingerprint(), hex.EncodeToString(hash[:])})
	}
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	var status enrollment.DeviceStatus
	if action == "claim" {
		code, err := servicehttp.ReadOperatorFile(*codePath, 128, true)
		if err != nil {
			return errors.New("private enrollment code file is unavailable")
		}
		defer clear(code)
		secret := strings.TrimSuffix(string(code), "\n")
		if len(secret) != 26 || strings.ContainsAny(secret, "\r\n \t") {
			return errors.New("private enrollment code file must contain one 26-character code, optionally followed by a newline")
		}
		status, err = client.Claim(ctx, config.Organization, config.InvitationID, secret)
	} else {
		status, err = client.Status(ctx, config.Organization, config.InvitationID)
	}
	if err != nil {
		return fmt.Errorf("key-bound device %s failed: %w", action, err)
	}
	if action == "prepare-runtime" {
		return prepareBASRuntime(config, identity, status, out)
	}
	return json.NewEncoder(out).Encode(status)
}

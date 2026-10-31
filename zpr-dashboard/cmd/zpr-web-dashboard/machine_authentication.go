//go:build !production

package main

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const machineAuthenticationModeFile = "authentication-mode"

func machinePHAuthenticationArguments(bootstrapKey, authenticationConfig string) ([]string, error) {
	if (bootstrapKey == "") == (authenticationConfig == "") {
		return nil, errors.New("exactly one machine bootstrap key or enrollment BAS authentication configuration is required")
	}
	if authenticationConfig == "" {
		return []string{"--bootstrap-key", bootstrapKey}, nil
	}
	if !filepath.IsAbs(authenticationConfig) || filepath.Clean(authenticationConfig) != authenticationConfig {
		return nil, errors.New("machine authentication configuration requires a canonical absolute path")
	}
	data, err := readOperatorFile(authenticationConfig, 16384, true)
	if err != nil {
		return nil, fmt.Errorf("read protected machine authentication configuration: %w", err)
	}
	var config struct {
		Global         struct{} `toml:"global"`
		Adapter        struct{} `toml:"adapter"`
		Authentication struct {
			BASKey    string `toml:"bas_key"`
			TLSCAFile string `toml:"tls_ca_file"`
			ClientID  string `toml:"bas_client_id"`
		} `toml:"authentication"`
	}
	metadata, err := toml.Decode(string(data), &config)
	if err != nil || len(metadata.Undecoded()) != 0 {
		return nil, errors.New("machine authentication configuration requires empty global/adapter sections and only authentication.bas_key, authentication.tls_ca_file and authentication.bas_client_id")
	}
	if !metadata.IsDefined("global") || !metadata.IsDefined("adapter") {
		return nil, errors.New("machine PH authentication configuration requires explicit empty global and adapter sections")
	}
	if config.Authentication.ClientID == "" || len(config.Authentication.ClientID) > 256 ||
		strings.TrimSpace(config.Authentication.ClientID) != config.Authentication.ClientID ||
		strings.ContainsAny(config.Authentication.ClientID, "\x00\r\n") {
		return nil, errors.New("machine BAS authentication requires the exact registry bas_client_id")
	}
	for _, path := range []string{config.Authentication.BASKey, config.Authentication.TLSCAFile} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == authenticationConfig {
			return nil, errors.New("machine BAS key and CA require explicit canonical absolute paths distinct from the authentication configuration")
		}
	}
	if config.Authentication.BASKey == config.Authentication.TLSCAFile {
		return nil, errors.New("machine BAS key and CA must be distinct files")
	}
	keyPEM, err := readOperatorFile(config.Authentication.BASKey, 16384, true)
	if err != nil {
		return nil, fmt.Errorf("read retained machine BAS key: %w", err)
	}
	defer clear(keyPEM)
	block, rest := pem.Decode(keyPEM)
	if block == nil || len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("retained machine BAS key must be one RSA private key PEM")
	}
	var key *rsa.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		var parsed any
		parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		if err == nil {
			key, _ = parsed.(*rsa.PrivateKey)
		}
	}
	clear(block.Bytes)
	if err != nil || key == nil || key.N == nil || key.N.BitLen() < 2048 || key.N.BitLen() > 4096 ||
		key.E != 65537 || key.Validate() != nil {
		return nil, errors.New("retained machine BAS key must be a valid 2048-4096 bit RSA private key")
	}
	ca, err := readOperatorFile(config.Authentication.TLSCAFile, 65536, false)
	if err != nil {
		return nil, fmt.Errorf("read machine BAS CA: %w", err)
	}
	if !x509.NewCertPool().AppendCertsFromPEM(ca) {
		return nil, errors.New("machine BAS CA contains no certificates")
	}
	return []string{"--config-file", authenticationConfig}, nil
}

func machineWorkloadAuthenticationArguments(runtimeDir, assets, agent, bootstrapKey string) ([]string, error) {
	mode, err := os.ReadFile(filepath.Join(runtimeDir, machineAuthenticationModeFile))
	if err != nil {
		return nil, fmt.Errorf("read machine workload authentication mode: %w", err)
	}
	switch string(mode) {
	case "bootstrap\n":
		return machinePHAuthenticationArguments(bootstrapKey, "")
	case "enrollment-bas\n":
		return machinePHAuthenticationArguments("", filepath.Join(assets, "authentication", agent+".toml"))
	default:
		return nil, errors.New("unknown machine workload authentication mode")
	}
}

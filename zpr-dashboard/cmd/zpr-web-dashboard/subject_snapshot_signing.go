package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// subjectSnapshotSigning is the Control-Service-owned snapshot signing key.
// Pins hold every accepted public key so verifiers can rotate across key IDs;
// only the configured active key ID may sign.
type subjectSnapshotSigning struct {
	keyID  string
	pins   map[string]ed25519.PublicKey
	signer ed25519.PrivateKey
}

type subjectSnapshotSigningConfig struct {
	Version int                         `json:"version"`
	KeyID   string                      `json:"key_id"`
	Pins    []subjectSnapshotSigningPin `json:"pins"`
}

type subjectSnapshotSigningPin struct {
	KeyID     string `json:"key_id"`
	PublicKey string `json:"public_key"`
}

func configuredSubjectSnapshotSigning() (*subjectSnapshotSigning, error) {
	configPath := strings.TrimSpace(os.Getenv("ZPR_SUBJECT_SNAPSHOT_SIGNING_FILE"))
	keyPath := strings.TrimSpace(os.Getenv("ZPR_SUBJECT_SNAPSHOT_SIGNING_KEY_FILE"))
	if configPath == "" && keyPath == "" {
		log.Print("Subject-link snapshot signing is disabled: signing configuration and key are not configured")
		return nil, nil
	}
	if !filepath.IsAbs(configPath) || !filepath.IsAbs(keyPath) {
		return nil, errors.New("subject-link snapshot signing requires absolute ZPR_SUBJECT_SNAPSHOT_SIGNING_FILE and ZPR_SUBJECT_SNAPSHOT_SIGNING_KEY_FILE paths")
	}
	data, err := readOperatorFile(configPath, 65536, false)
	if err != nil {
		return nil, err
	}
	var config subjectSnapshotSigningConfig
	if err := decodeOperatorConfig(data, &config); err != nil {
		return nil, err
	}
	invalid := errors.New("subject-link snapshot signing configuration requires version 1, an active key_id and 1-64 unique Ed25519 pins")
	if config.Version != 1 || !reviewIdentifier.MatchString(config.KeyID) || len(config.Pins) == 0 || len(config.Pins) > 64 {
		return nil, invalid
	}
	pins := make(map[string]ed25519.PublicKey, len(config.Pins))
	seen := make(map[string]bool, len(config.Pins))
	for _, pin := range config.Pins {
		public, err := base64.RawURLEncoding.DecodeString(pin.PublicKey)
		if err != nil || len(public) != ed25519.PublicKeySize || base64.RawURLEncoding.EncodeToString(public) != pin.PublicKey ||
			!reviewIdentifier.MatchString(pin.KeyID) || pins[pin.KeyID] != nil || seen[pin.PublicKey] {
			return nil, invalid
		}
		pins[pin.KeyID], seen[pin.PublicKey] = ed25519.PublicKey(public), true
	}
	keyData, err := readOperatorFile(keyPath, 65536, true)
	if err != nil {
		return nil, err
	}
	block, rest := pem.Decode(keyData)
	if block == nil || block.Type != "PRIVATE KEY" || len(block.Headers) != 0 || len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("subject-link snapshot signing key must be one PKCS#8 Ed25519 PEM private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("invalid subject-link snapshot signing PKCS#8 key")
	}
	private, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("subject-link snapshot signing key must be Ed25519")
	}
	active, pinned := pins[config.KeyID]
	if !pinned || !active.Equal(private.Public()) {
		return nil, errors.New("subject-link snapshot signing key must match the pin for its active key_id")
	}
	return &subjectSnapshotSigning{keyID: config.KeyID, pins: pins, signer: private}, nil
}

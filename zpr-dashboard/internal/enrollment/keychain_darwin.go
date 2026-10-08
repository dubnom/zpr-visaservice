package enrollment

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

const keychainIdentityFile = "identity.keychain.json"
const keychainProtection = "macos-keychain-development"

type keychainReference struct {
	Version    int    `json:"version"`
	Protection string `json:"protection"`
	Reference  string `json:"reference"`
}

type identityKeychain interface {
	add(account string, data []byte) ([]byte, error)
	read(reference []byte) ([]byte, error)
	remove(reference []byte) error
}

func createKeychainIdentity(directory string, metadata LocalEnrollment, keychain identityKeychain) (*SoftwareIdentity, error) {
	if !validLocalEnrollment(metadata) {
		return nil, ErrInvalid
	}
	root, err := openIdentityDirectory(directory, true)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	for _, name := range []string{identityFile, keychainIdentityFile} {
		if _, err := root.Lstat(name); err == nil {
			return nil, fmt.Errorf("enrollment identity already exists: %w", os.ErrExist)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	identity, data, err := newSoftwareIdentityRecord(metadata, keychainProtection)
	if err != nil {
		return nil, err
	}
	defer clear(data)
	reference, err := keychain.add(rand.Text(), data)
	if err != nil {
		return nil, err
	}
	record, err := json.Marshal(keychainReference{Version: 1, Protection: keychainProtection,
		Reference: base64.StdEncoding.EncodeToString(reference)})
	if err == nil {
		err = publishIdentityFile(root, keychainIdentityFile, record)
	}
	if err != nil {
		// A post-publication sync error must not delete the newly bound key.
		published, readErr := readIdentityFile(root, keychainIdentityFile)
		if readErr == nil && bytes.Equal(published, record) {
			return nil, fmt.Errorf("Keychain reference published but durability uncertain; retain state and restart: %w", err)
		}
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return nil, errors.Join(err, fmt.Errorf("could not reconcile Keychain publication; key retained: %w", readErr))
		}
		return nil, errors.Join(err, keychain.remove(reference))
	}
	return identity, nil
}

func loadKeychainIdentity(directory string, keychain identityKeychain) (*SoftwareIdentity, error) {
	root, err := openIdentityDirectory(directory, false)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if _, err := root.Lstat(identityFile); err == nil {
		return nil, errors.New("legacy plaintext enrollment identity exists; explicit administrator recovery is required, not automatic migration")
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	data, err := readIdentityFile(root, keychainIdentityFile)
	if err != nil {
		return nil, err
	}
	var record keychainReference
	if decodeJSON(bytes.NewReader(data), &record) != nil || record.Version != 1 || record.Protection != keychainProtection {
		return nil, errors.New("invalid local Keychain enrollment reference")
	}
	reference, err := base64.StdEncoding.Strict().DecodeString(record.Reference)
	if err != nil || len(reference) == 0 || len(reference) > 4096 {
		return nil, errors.New("invalid Keychain reference encoding")
	}
	plaintext, err := keychain.read(reference)
	if err != nil {
		// Missing Keychain items are NOT missing local state. Never replace the key.
		return nil, fmt.Errorf("saved enrollment Keychain item unavailable; unlock/authorize Keychain or seek administrator recovery: %v", err)
	}
	defer clear(plaintext)
	return decodeSoftwareIdentity(plaintext, keychainProtection)
}

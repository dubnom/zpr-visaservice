package main

import (
	"bytes"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"neboagency.com/zpr-dashborad/internal/enrollment"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

func prepareBASRuntime(config deviceConfig, identity *enrollment.SoftwareIdentity, status enrollment.DeviceStatus, out io.Writer) error {
	if status.State != "approved" || status.Revision < 3 || status.InvitationID != config.InvitationID ||
		identity.Metadata() != (enrollment.LocalEnrollment{Audience: config.Audience, Organization: config.Organization, InvitationID: config.InvitationID}) {
		return errors.New("BAS runtime preparation requires fresh approved enrollment status for this exact retained identity")
	}
	if status.AssetID == "" || len(status.AssetID) > 256 || strings.TrimSpace(status.AssetID) != status.AssetID ||
		strings.ContainsAny(status.AssetID, "\x00\r\n") {
		return errors.New("BAS runtime preparation requires an authoritative registry asset ID in approved enrollment status; upgrade the Enrollment Service, never infer an ID from a display name")
	}
	key, ok := identity.RuntimeSigner().(*rsa.PrivateKey)
	if !ok || key == nil {
		return errors.New("retained software BAS runtime key is unavailable")
	}
	for _, binding := range []struct {
		public      any
		fingerprint string
	}{{identity.Public(), status.KeyFingerprint}, {key.Public(), status.RuntimeKeyFingerprint}} {
		der, err := x509.MarshalPKIXPublicKey(binding.public)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(der)
		if hex.EncodeToString(hash[:]) != binding.fingerprint {
			return errors.New("approved enrollment fingerprints differ from this endpoint's retained keys")
		}
	}
	ca, err := servicehttp.ReadOperatorFile(config.BASCAFile, 65536, false)
	if err != nil {
		return fmt.Errorf("read explicitly trusted BAS CA: %w", err)
	}
	if !x509.NewCertPool().AppendCertsFromPEM(ca) {
		return errors.New("explicit BAS CA contains no certificates")
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	defer clear(keyDER)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	defer clear(keyPEM)
	keyPath := filepath.Join(config.StateDirectory, "runtime-auth.key")
	caPath := filepath.Join(config.StateDirectory, "runtime-auth-ca.pem")
	configPath := filepath.Join(config.StateDirectory, "runtime-auth.toml")
	var authentication bytes.Buffer
	if err := toml.NewEncoder(&authentication).Encode(map[string]map[string]string{
		"global":         {},
		"adapter":        {},
		"authentication": {"bas_key": keyPath, "tls_ca_file": caPath, "bas_client_id": status.AssetID},
	}); err != nil {
		return fmt.Errorf("encode endpoint PH authentication configuration: %w", err)
	}
	root, err := os.OpenRoot(config.StateDirectory)
	if err != nil {
		return err
	}
	defer root.Close()
	directory, err := root.Stat(".")
	if err != nil || !directory.IsDir() || directory.Mode().Perm()&0077 != 0 {
		return errors.New("retained endpoint state directory must remain owner-private")
	}
	files := []struct {
		name string
		data []byte
	}{
		{"runtime-auth.key", keyPEM},
		{"runtime-auth-ca.pem", ca},
		{"runtime-auth.toml", authentication.Bytes()},
	}
	for _, item := range files {
		if err := writeRuntimeOnce(root, item.name, item.data); err != nil {
			return fmt.Errorf("prepare retained endpoint BAS runtime file %s: %w", item.name, err)
		}
	}
	return json.NewEncoder(out).Encode(struct {
		Organization string `json:"organization"`
		InvitationID string `json:"invitation_id"`
		Revision     int    `json:"approved_revision"`
		Fingerprint  string `json:"runtime_key_fingerprint"`
		Config       string `json:"authentication_config"`
		ClientID     string `json:"bas_client_id"`
		Admitted     bool   `json:"admission_verified"`
	}{config.Organization, config.InvitationID, status.Revision, status.RuntimeKeyFingerprint, configPath, status.AssetID, false})
}

func writeRuntimeOnce(root *os.Root, name string, data []byte) error {
	info, err := root.Lstat(name)
	if err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			return errors.New("existing runtime output must be an owner-private regular file")
		}
		file, err := root.Open(name)
		if err != nil {
			return err
		}
		defer file.Close()
		opened, err := file.Stat()
		if err != nil || !os.SameFile(info, opened) {
			return errors.New("existing runtime output changed while opening")
		}
		existing, err := io.ReadAll(io.LimitReader(file, int64(len(data))+1))
		defer clear(existing)
		if err != nil {
			return err
		}
		if !bytes.Equal(existing, data) {
			return errors.New("existing runtime output differs; refusing key replacement or trust rebinding")
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := errors.Join(file.Sync(), file.Close()); err != nil {
		return err
	}
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}

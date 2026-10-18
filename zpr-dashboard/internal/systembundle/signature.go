package systembundle

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
)

const MaxSignatureBytes = 16 << 10

type ArchiveSignature struct {
	SchemaVersion   int    `json:"schema_version"`
	Algorithm       string `json:"algorithm"`
	ReleaseID       string `json:"release_id"`
	BundleKind      string `json:"bundle_kind"`
	ArchiveSHA256   string `json:"archive_sha256"`
	SignerKeySHA256 string `json:"signer_key_sha256"`
	Signature       string `json:"signature"`
}

func SignArchive(archivePath, privateKeyPath string) (ArchiveSignature, error) {
	privateKey, err := readPrivateSigningKey(privateKeyPath)
	if err != nil {
		return ArchiveSignature{}, err
	}
	manifest, digest, err := ValidateArchive(archivePath)
	if err != nil {
		return ArchiveSignature{}, err
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	fingerprint := sha256.Sum256(publicKey)
	result := ArchiveSignature{
		SchemaVersion:   1,
		Algorithm:       "ed25519",
		ReleaseID:       manifest.ReleaseID,
		BundleKind:      manifest.BundleKind,
		ArchiveSHA256:   digest,
		SignerKeySHA256: hex.EncodeToString(fingerprint[:]),
	}
	result.Signature = hex.EncodeToString(ed25519.Sign(privateKey, signatureMessage(result)))
	return result, nil
}

func VerifyArchiveSignature(archivePath string, signature ArchiveSignature, publicKeyPath string) error {
	if err := signature.Validate(); err != nil {
		return err
	}
	manifest, digest, err := ValidateArchive(archivePath)
	if err != nil {
		return err
	}
	if signature.ReleaseID != manifest.ReleaseID || signature.BundleKind != manifest.BundleKind {
		return errors.New("signature release identity does not match the archive manifest")
	}
	if signature.ArchiveSHA256 != digest {
		return errors.New("signature archive sha256 does not match the archive")
	}
	publicKey, err := readPublicSigningKey(publicKeyPath)
	if err != nil {
		return err
	}
	fingerprint := sha256.Sum256(publicKey)
	if signature.SignerKeySHA256 != hex.EncodeToString(fingerprint[:]) {
		return errors.New("signature signer key fingerprint does not match the trusted public key")
	}
	decoded, err := hex.DecodeString(signature.Signature)
	if err != nil || len(decoded) != ed25519.SignatureSize {
		return errors.New("signature must be 128 lowercase hexadecimal characters")
	}
	if !ed25519.Verify(publicKey, signatureMessage(signature), decoded) {
		return errors.New("archive signature is invalid")
	}
	return nil
}

func (signature ArchiveSignature) Validate() error {
	if signature.SchemaVersion != 1 || signature.Algorithm != "ed25519" {
		return errors.New("signature requires schema_version 1 and algorithm ed25519")
	}
	if !releaseIDPattern.MatchString(signature.ReleaseID) ||
		(signature.BundleKind != "system" && signature.BundleKind != "companion") {
		return errors.New("signature release identity is invalid")
	}
	if !sha256Pattern.MatchString(signature.ArchiveSHA256) ||
		!sha256Pattern.MatchString(signature.SignerKeySHA256) {
		return errors.New("signature archive and signer fingerprints must be lowercase sha256 values")
	}
	if len(signature.Signature) != ed25519.SignatureSize*2 {
		return errors.New("signature must be 128 lowercase hexadecimal characters")
	}
	for _, character := range signature.Signature {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return errors.New("signature must be 128 lowercase hexadecimal characters")
		}
	}
	return nil
}

func signatureMessage(signature ArchiveSignature) []byte {
	return []byte(fmt.Sprintf(
		"zpr-system-bundle-signature-v1\n%s\n%s\n%s\n%s\n%s",
		signature.ReleaseID,
		signature.BundleKind,
		signature.ArchiveSHA256,
		signature.SignerKeySHA256,
		signature.Algorithm,
	))
}

func ReadArchiveSignature(reader io.Reader) (ArchiveSignature, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxSignatureBytes+1))
	if err != nil || len(data) > MaxSignatureBytes {
		return ArchiveSignature{}, errors.New("signature file cannot be read or exceeds 16 KiB")
	}
	if err := validateJSONShape(data); err != nil {
		return ArchiveSignature{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var signature ArchiveSignature
	if err := decoder.Decode(&signature); err != nil {
		return ArchiveSignature{}, errors.New("signature file is not valid JSON")
	}
	if err := requireJSONEOF(decoder); err != nil {
		return ArchiveSignature{}, err
	}
	if err := signature.Validate(); err != nil {
		return ArchiveSignature{}, err
	}
	return signature, nil
}

func WriteArchiveSignature(writer io.Writer, signature ArchiveSignature) error {
	if err := signature.Validate(); err != nil {
		return err
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(signature); err != nil {
		return errors.New("signature file cannot be written")
	}
	return nil
}

func readPrivateSigningKey(path string) (ed25519.PrivateKey, error) {
	file, err := openKeyFile(path, true)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	block, err := readPEMBlock(file)
	if err != nil || block.Type != "PRIVATE KEY" {
		return nil, errors.New("signing key must be one PKCS#8 Ed25519 PRIVATE KEY PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("signing key is not valid PKCS#8")
	}
	privateKey, ok := key.(ed25519.PrivateKey)
	if !ok || len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("signing key must use Ed25519")
	}
	return privateKey, nil
}

func readPublicSigningKey(path string) (ed25519.PublicKey, error) {
	file, err := openKeyFile(path, false)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	block, err := readPEMBlock(file)
	if err != nil || block.Type != "PUBLIC KEY" {
		return nil, errors.New("trusted key must be one PKIX Ed25519 PUBLIC KEY PEM")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, errors.New("trusted key is not valid PKIX")
	}
	publicKey, ok := key.(ed25519.PublicKey)
	if !ok || len(publicKey) != ed25519.PublicKeySize {
		return nil, errors.New("trusted key must use Ed25519")
	}
	return publicKey, nil
}

func openKeyFile(path string, private bool) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("key file must be a regular non-symlink file")
	}
	if info.Size() <= 0 || info.Size() > MaxSignatureBytes {
		return nil, errors.New("key file must be between 1 byte and 16 KiB")
	}
	if info.Sys() != nil {
		if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Nlink != 1 {
			return nil, errors.New("key file must not have multiple hard links")
		}
	}
	if private && info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("private signing key must not be accessible by group or other")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("key file cannot be opened")
	}
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return nil, errors.New("key file changed while opening")
	}
	return file, nil
}

func readPEMBlock(file *os.File) (*pem.Block, error) {
	data, err := io.ReadAll(io.LimitReader(file, MaxSignatureBytes+1))
	if err != nil || len(data) > MaxSignatureBytes {
		return nil, errors.New("key file cannot be read or exceeds 16 KiB")
	}
	block, rest := pem.Decode(data)
	if block == nil || len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("key file must contain exactly one PEM block")
	}
	return block, nil
}

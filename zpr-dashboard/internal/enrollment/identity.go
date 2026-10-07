//go:build linux || darwin

package enrollment

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

const identityFile = "identity.json"

// LocalEnrollment contains no code, credentials, or cached approval decision.
type LocalEnrollment struct {
	Audience     string `json:"audience"`
	Organization string `json:"organization"`
	InvitationID string `json:"invitation_id"`
}

// SoftwareIdentity is development-only, not a hardware-backed device identity.
type SoftwareIdentity struct {
	key      *rsa.PrivateKey
	metadata LocalEnrollment
}

func (i *SoftwareIdentity) Public() crypto.PublicKey { return i.key.Public() }
func (i *SoftwareIdentity) Sign(random io.Reader, digest []byte, options crypto.SignerOpts) ([]byte, error) {
	return i.key.Sign(random, digest, options)
}
func (i *SoftwareIdentity) Metadata() LocalEnrollment { return i.metadata }

type identityRecord struct {
	Version    int             `json:"version"`
	Protection string          `json:"protection"`
	Enrollment LocalEnrollment `json:"enrollment"`
	PrivateKey string          `json:"private_key"`
}

func validLocalEnrollment(m LocalEnrollment) bool {
	return validateAudience(m.Audience) == nil && validText(m.Organization) && validText(m.InvitationID)
}

// CreateSoftwareIdentity never overwrites an existing identity. The parent
// directory must be trusted; the caller selects a dedicated local state path.
func CreateSoftwareIdentity(directory string, metadata LocalEnrollment) (*SoftwareIdentity, error) {
	if !validLocalEnrollment(metadata) {
		return nil, ErrInvalid
	}
	root, err := openIdentityDirectory(directory, true)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if _, err := root.Lstat(identityFile); err == nil {
		return nil, fmt.Errorf("enrollment identity already exists: %w", os.ErrExist)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return nil, fmt.Errorf("generate enrollment key: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(identityRecord{Version: 1, Protection: "software-development",
		Enrollment: metadata, PrivateKey: base64.StdEncoding.EncodeToString(der)})
	if err != nil {
		return nil, err
	}
	if err := publishIdentity(root, data); err != nil {
		return nil, err
	}
	return &SoftwareIdentity{key: key, metadata: metadata}, nil
}

func publishIdentity(root *os.Root, data []byte) (err error) {
	name := ".identity-" + rand.Text()
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	temporary := true
	defer func() {
		err = errors.Join(err, file.Close())
		if temporary {
			err = errors.Join(err, root.Remove(name))
		}
	}()
	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	// Link publishes a complete file atomically without replacing another key.
	if err = root.Link(name, identityFile); err != nil {
		return fmt.Errorf("publish enrollment identity: %w", err)
	}
	if err = root.Remove(name); err != nil {
		return err
	}
	temporary = false
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}

// LoadSoftwareIdentity fails closed on missing, unsafe, or corrupt state;
// it never generates a replacement key or sends a claim.
func LoadSoftwareIdentity(directory string) (*SoftwareIdentity, error) {
	root, err := openIdentityDirectory(directory, false)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	entry, err := root.Lstat(identityFile)
	if err != nil {
		return nil, err
	}
	if !entry.Mode().IsRegular() || !privateOwned(entry) {
		return nil, errors.New("enrollment identity must be a private, owner-only regular file")
	}
	file, err := root.OpenFile(identityFile, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open enrollment identity: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || !privateOwned(info) || !os.SameFile(entry, info) {
		return nil, errors.New("enrollment identity must be a private, owner-only regular file")
	}
	// Root resolves internal symlinks itself; verify the directory entry too.
	current, err := root.Lstat(identityFile)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(current, info) {
		return nil, errors.New("enrollment identity changed while opening")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		return nil, errors.New("enrollment identity must not have additional hard links")
	}
	data, err := io.ReadAll(io.LimitReader(file, 16385))
	if err != nil {
		return nil, err
	}
	var record identityRecord
	if len(data) > 16384 || decodeJSON(bytes.NewReader(data), &record) != nil ||
		record.Version != 1 || record.Protection != "software-development" || !validLocalEnrollment(record.Enrollment) {
		return nil, errors.New("invalid local enrollment identity")
	}
	der, err := base64.StdEncoding.Strict().DecodeString(record.PrivateKey)
	if err != nil {
		return nil, errors.New("invalid local enrollment key encoding")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, errors.New("invalid local enrollment private key")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok || key.N == nil || key.N.BitLen() != 3072 || key.E != 65537 || key.Validate() != nil {
		return nil, errors.New("invalid local enrollment RSA key")
	}
	return &SoftwareIdentity{key: key, metadata: record.Enrollment}, nil
}

func openIdentityDirectory(path string, create bool) (*os.Root, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("enrollment state directory must be an absolute clean path")
	}
	if create {
		if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || !privateOwned(info) {
		return nil, errors.New("enrollment state directory must be private, owner-only, and not a symlink")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) || !privateOwned(opened) {
		root.Close()
		return nil, errors.New("enrollment state directory changed while opening")
	}
	return root, nil
}

func privateOwned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid() && info.Mode().Perm()&0077 == 0 &&
		info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) == 0
}

//go:build linux || darwin

package enrollment

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

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
	identity, data, err := newSoftwareIdentityRecord(metadata, "software-development")
	if err != nil {
		return nil, err
	}
	if err := publishIdentity(root, data); err != nil {
		return nil, err
	}
	return identity, nil
}

func publishIdentity(root *os.Root, data []byte) (err error) {
	return publishIdentityFile(root, identityFile, data)
}

func publishIdentityFile(root *os.Root, destination string, data []byte) (err error) {
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
	if err = root.Link(name, destination); err != nil {
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
	data, err := readIdentityFile(root, identityFile)
	if err != nil {
		return nil, err
	}
	defer clear(data)
	return decodeSoftwareIdentity(data, "software-development")
}

func readIdentityFile(root *os.Root, name string) ([]byte, error) {
	entry, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !entry.Mode().IsRegular() || !privateOwned(entry) {
		return nil, errors.New("enrollment identity must be a private, owner-only regular file")
	}
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
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
	current, err := root.Lstat(name)
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
	if len(data) > 16384 {
		return nil, errors.New("local enrollment identity exceeds maximum size")
	}
	return data, nil
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

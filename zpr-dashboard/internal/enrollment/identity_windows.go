package enrollment

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const windowsIdentityFile = "identity.dpapi"

func protectWindowsIdentity(data []byte, decrypt bool) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty Windows identity")
	}
	input := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var output windows.DataBlob
	var err error
	if decrypt {
		err = windows.CryptUnprotectData(&input, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output)
	} else {
		// Omitting LOCAL_MACHINE binds decryption to the current Windows user.
		err = windows.CryptProtectData(&input, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output)
	}
	if err != nil {
		return nil, fmt.Errorf("Windows user-bound DPAPI protection: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data)))
	buffer := unsafe.Slice(output.Data, int(output.Size))
	defer clear(buffer)
	if len(buffer) > 65536 {
		return nil, errors.New("Windows identity exceeds maximum size")
	}
	return bytes.Clone(buffer), nil
}

func windowsIdentityDirectory(path string, create bool) (*os.Root, *os.File, error) {
	if err := checkWindowsParents(path); err != nil {
		return nil, nil, err
	}
	if create {
		if err := createWindowsPrivateDirectory(path); err != nil {
			return nil, nil, err
		}
	}
	lock, err := openWindowsFile(path, true, true)
	if err != nil {
		return nil, nil, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		lock.Close()
		return nil, nil, err
	}
	return root, lock, nil
}

func CreateSoftwareIdentity(directory string, metadata LocalEnrollment) (*SoftwareIdentity, error) {
	if !validLocalEnrollment(metadata) {
		return nil, ErrInvalid
	}
	root, lock, err := windowsIdentityDirectory(directory, true)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	defer root.Close()
	for _, name := range []string{windowsIdentityFile, identityFile} {
		if _, err := root.Lstat(name); err == nil {
			return nil, fmt.Errorf("enrollment identity already exists: %w", os.ErrExist)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	identity, plaintext, err := newSoftwareIdentityRecord(metadata, "windows-user-dpapi-development")
	if err != nil {
		return nil, err
	}
	defer clear(plaintext)
	data, err := protectWindowsIdentity(plaintext, false)
	if err != nil {
		return nil, err
	}
	if err := publishWindowsIdentity(root, data); err != nil {
		return nil, err
	}
	return identity, nil
}

func publishWindowsIdentity(root *os.Root, data []byte) (err error) {
	name := ".identity-" + rand.Text()
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Remove(name)) }()
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	// A hard link publishes complete ciphertext without replacing another key.
	return root.Link(name, windowsIdentityFile)
}

func LoadSoftwareIdentity(directory string) (*SoftwareIdentity, error) {
	root, lock, err := windowsIdentityDirectory(directory, false)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	defer root.Close()
	if _, err := root.Lstat(identityFile); err == nil {
		return nil, errors.New("plaintext identity is not supported on Windows; administrator recovery is required")
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	file, err := openWindowsFile(filepath.Join(directory, windowsIdentityFile), false, true)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 65537))
	if err != nil {
		return nil, err
	}
	if len(data) > 65536 {
		return nil, errors.New("invalid Windows enrollment identity size")
	}
	plaintext, err := protectWindowsIdentity(data, true)
	if err != nil {
		return nil, err
	}
	defer clear(plaintext)
	return decodeSoftwareIdentity(plaintext, "windows-user-dpapi-development")
}

//go:build darwin && !cgo

package enrollment

import "errors"

type macKeychain struct{}

func keychainAvailable() bool { return false }

func (macKeychain) add(string, []byte) ([]byte, error) {
	return nil, errors.New("macOS Keychain support requires a native CGO-enabled build")
}
func (macKeychain) read([]byte) ([]byte, error) {
	return nil, errors.New("macOS Keychain support requires a native CGO-enabled build")
}
func (macKeychain) remove([]byte) error {
	return errors.New("macOS Keychain support requires a native CGO-enabled build")
}

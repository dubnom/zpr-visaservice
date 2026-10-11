//go:build linux || darwin

package enrollment

import (
	"errors"
	"path/filepath"
)

func readPrivateServiceFile(path string) ([]byte, error) {
	root, err := openIdentityDirectory(filepath.Dir(path), false)
	if err != nil {
		return nil, err
	}
	data, readErr := readIdentityFile(root, filepath.Base(path))
	if err := errors.Join(readErr, root.Close()); err != nil {
		clear(data)
		return nil, err
	}
	return data, nil
}

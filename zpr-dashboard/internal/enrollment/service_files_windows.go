package enrollment

import (
	"errors"
	"io"
	"path/filepath"
)

func readPrivateServiceFile(path string) ([]byte, error) {
	if err := checkWindowsParents(path); err != nil {
		return nil, err
	}
	directory, err := openWindowsFile(filepath.Dir(path), true, true)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	file, err := openWindowsFile(path, false, true)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 16385))
	if err := errors.Join(readErr, file.Close()); err != nil {
		clear(data)
		return nil, err
	}
	if len(data) > 16384 {
		clear(data)
		return nil, errors.New("private service file exceeds maximum size")
	}
	return data, nil
}
